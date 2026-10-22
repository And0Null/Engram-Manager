/* Engram Manager — dashboard client.
   No framework, no build step: one namespace, explicit render functions. */
"use strict";

(function () {
  // ---------------------------------------------------------------- types

  // Each memory type owns a hue for good. Read once from the stylesheet so the
  // chart and the stylesheet can never drift apart.
  const TYPE_HUE = {};
  const CSS_VARS = ["session_summary", "discovery", "config", "bugfix", "decision",
    "architecture", "preference", "pattern", "learning", "manual"];
  const FALLBACK_HUE = "#7d8ea0";
  CSS_VARS.forEach((t) => {
    TYPE_HUE[t] = getComputedStyle(document.documentElement).getPropertyValue(`--t-${t}`).trim() || FALLBACK_HUE;
  });
  const hueOf = (t) => TYPE_HUE[t] || FALLBACK_HUE;

  // ---------------------------------------------------------------- state

  const state = {
    view: "overview",
    window: "7d",
    typePeriod: "range",
    excludeSessions: false,
    overview: null,
    health: null,
    memories: [],
    filter: { q: "", type: "", project: "", pinned: false, deleted: false, limit: 100, offset: 0 },
    relations: [],
    relationsSummary: null,
    sessions: [],
    timeline: null,
    review: [],
    memory: null,
    memoryRelations: [],
    editing: false,
  };

  // Every load bumps this, and a response that is not from the newest load is
  // dropped. Without it a slow endpoint answers after the operator has moved
  // on and repaints its view under the wrong navigation item.
  let viewEpoch = 0;

  const $ = (sel, root = document) => root.querySelector(sel);
  const $$ = (sel, root = document) => Array.from(root.querySelectorAll(sel));

  const content = $("#content");
  const pageTitle = $("[data-page-title]");
  const pageSub = $("[data-page-sub]");
  const searchInput = $("[data-search]");
  const drawer = $("[data-drawer]");
  const backdrop = $("[data-drawer-backdrop]");

  // ---------------------------------------------------------------- helpers

  const esc = (s) => String(s ?? "")
    .replace(/&/g, "&amp;").replace(/</g, "&lt;").replace(/>/g, "&gt;")
    .replace(/"/g, "&quot;").replace(/'/g, "&#39;");

  const num = (n) => (n ?? 0).toLocaleString("es-ES");
  const pct = (n) => `${(n ?? 0).toFixed(1)}%`;

  function bytes(n) {
    if (!n) return "0 B";
    const u = ["B", "KiB", "MiB", "GiB"];
    const i = Math.min(u.length - 1, Math.floor(Math.log(n) / Math.log(1024)));
    return `${(n / 1024 ** i).toFixed(i ? 1 : 0)} ${u[i]}`;
  }

  // Engram writes UTC as "YYYY-MM-DD HH:MM:SS".
  function parseTs(s) {
    if (!s) return null;
    const d = new Date(/^\d{4}-\d{2}-\d{2} /.test(s) ? s.replace(" ", "T") + "Z" : s);
    return isNaN(d.getTime()) ? null : d;
  }

  function fmtDate(s) {
    const d = parseTs(s);
    return d ? d.toLocaleString("es-ES", { dateStyle: "short", timeStyle: "short" }) : "—";
  }

  function fmtDay(iso) {
    const d = parseTs(iso);
    return d ? d.toLocaleDateString("es-ES", { day: "2-digit", month: "short" }) : iso;
  }

  function toast(message, kind = "ok") {
    const el = document.createElement("div");
    el.className = "toast";
    el.dataset.kind = kind;
    el.innerHTML = `<span class="toast-dot"></span><span>${esc(message)}</span>`;
    $("#toasts").appendChild(el);
    setTimeout(() => {
      el.classList.add("leaving");
      setTimeout(() => el.remove(), 260);
    }, 3000);
  }

  async function api(path, opts = {}) {
    const res = await fetch(path, {
      method: opts.method || "GET",
      headers: opts.body ? { "Content-Type": "application/json" } : {},
      body: opts.body ? JSON.stringify(opts.body) : undefined,
    });
    const data = await res.json().catch(() => ({}));
    if (!res.ok) throw new Error(data.error || `HTTP ${res.status}`);
    return data;
  }

  function loading(label) {
    // A skeleton keeps the geometry of the view it replaces: no spinner in the
    // middle of an empty page, and no layout jump when the data lands.
    content.innerHTML = `<div class="loading-shell" role="status">
      <span class="visually-hidden">${esc(label)}</span>
      <div class="kpi-grid">
        ${Array.from({ length: 4 }, () => `
          <div class="card kpi">
            <div class="kpi-top"><span class="skeleton" style="width:38%;height:12px"></span></div>
            <span class="skeleton" style="width:46%;height:26px"></span>
            <span class="skeleton" style="width:62%;height:11px"></span>
          </div>`).join("")}
      </div>
      <div class="card">
        <div class="card-header"><span class="skeleton" style="width:150px;height:15px"></span></div>
        <div class="chart-wrap"><span class="skeleton" style="height:200px"></span></div>
      </div>
    </div>`;
  }

  function failed(err, retry) {
    content.innerHTML = `<div class="state">
      <h3>Could not load this view</h3>
      <p>${esc(err.message)}</p>
      <button class="btn" data-retry>Try again</button>
    </div>`;
    const b = $("[data-retry]", content);
    if (b) b.addEventListener("click", retry);
  }


  // ---------------------------------------------------------------- overview

  const WINDOW_LABEL = { "24h": "Last 24 hours", "7d": "Last 7 days", "30d": "Last 30 days" };
  const windowLabel = () => WINDOW_LABEL[state.window] || state.window;

  // Fixed type→token map. Unlisted types resolve to the neutral, never to a
  // ninth hue, and the map is the single source the donut and the rows share.
  const TYPE_VARS = {
    session_summary: "--t-session_summary", discovery: "--t-discovery", decision: "--t-decision",
    config: "--t-config", bugfix: "--t-bugfix", pattern: "--t-pattern",
    architecture: "--t-architecture", feature: "--t-feature",
  };
  const typeToken = (t) => TYPE_VARS[t] || "--t-other";
  const shareLabel = (p) => (p > 0 && p < 1 ? "<1%" : `${Math.round(p)}%`);

  // One series feeds every range: the 24 hourly buckets, the last 7 daily
  // buckets, or all 30. The chart and the "new in range" KPI read this same
  // array, so the headline number always belongs to the bars under it.
  function chartSeries() {
    const d = state.overview || {};
    if (state.window === "24h") {
      return {
        unit: "hour",
        points: ((d.hourly || {}).hours || []).map((h) => ({
          label: hourLabel(h.hour),
          value: h.count,
        })),
      };
    }
    const n = state.window === "7d" ? 7 : 30;
    return {
      unit: "day",
      points: ((d.daily || {}).days || []).slice(-n).map((day) => ({
        label: fmtDay(day.date),
        value: day.count,
      })),
    };
  }

  // "YYYY-MM-DD HH:00" → "HH:00".
  function hourLabel(hour) {
    const m = /(\d{2}):00$/.exec(hour || "");
    return m ? `${m[1]}:00` : String(hour || "—");
  }

  // Prior periods are computed from the series the store already returns,
  // never from a constant. The 30-day window ships no prior month, so it says
  // so instead of inventing a comparison.
  function priorForRange() {
    const days = ((state.overview || {}).daily || {}).days || [];
    if (state.window === "7d" && days.length >= 14) {
      return { label: "prior 7 days", value: days.slice(-14, -7).reduce((a, d) => a + d.count, 0) };
    }
    if (state.window === "24h" && days.length >= 2) {
      return { label: "prior day", value: days[days.length - 2].count };
    }
    return null;
  }

  function deltaHtml(total) {
    const prior = priorForRange();
    if (!prior || prior.value <= 0) return `<span class="delta neutral">— No prior period</span>`;
    const change = Math.round(((total - prior.value) / prior.value) * 100);
    const cls = change > 0 ? "up" : change < 0 ? "down" : "neutral";
    const arrow = change > 0 ? "↑" : change < 0 ? "↓" : "→";
    return `<span class="delta ${cls}">${arrow} ${Math.abs(change)}%<small>vs ${prior.label}</small></span>`;
  }

  function chartTooltip() {
    let tip = $("#chart-tooltip");
    if (!tip) {
      tip = document.createElement("div");
      tip.id = "chart-tooltip";
      tip.className = "tooltip";
      tip.setAttribute("role", "status");
      tip.setAttribute("aria-live", "polite");
      document.body.appendChild(tip);
    }
    return tip;
  }

  function renderChart(series) {
    const svg = $("#activity-chart");
    const note = $("#chart-note");
    const selected = $("#chart-selected");
    if (!svg || !note || !selected) return;

    const points = series.points;
    if (!points.length) {
      svg.innerHTML = "";
      note.innerHTML = `<span>No activity buckets in this range.</span>`;
      return;
    }

    const values = points.map((p) => p.value);
    const max = Math.max(...values);
    const peak = values.indexOf(max);
    const avg = values.reduce((a, b) => a + b, 0) / points.length;

    const W = 1000, H = 240, L = 44, R = 14, T = 14, B = 30;
    const plotW = W - L - R, plotH = H - T - B, slot = plotW / points.length;
    // Seven days gets wide bars, a month gets narrow ones; the cap keeps a
    // sparse window from turning into seven lonely slivers.
    const maxBar = points.length <= 7 ? 52 : points.length === 24 ? 26 : 20;
    const barWidth = Math.min(Math.max(slot * 0.62, 3), maxBar);

    const step = [1, 2, 5, 10, 20, 25, 30, 50, 100].find((s) => max / s <= 4) || 250;
    const top = Math.max(step, Math.ceil(max / step) * step);

    const y = (v) => T + plotH - (v / top) * plotH;
    const x = (i) => L + slot * (i + 0.5);

    let grid = "";
    for (let v = 0; v <= top; v += step) {
      grid += `<line class="chart-grid" x1="${L}" x2="${W - R}" y1="${y(v)}" y2="${y(v)}"/>`
        + `<text class="chart-axis" x="${L - 10}" y="${y(v) + 3}" text-anchor="end">${v}</text>`;
    }

    // Every fourth hour, five days, or every bar when the window is a week.
    const every = points.length <= 7 ? 1 : points.length === 24 ? 4 : 5;
    const ticks = points.map((p, i) => {
      const last = i === points.length - 1;
      if (i % every !== 0 && !last) return "";
      return `<text class="chart-axis" x="${x(i)}" y="${H - 8}" text-anchor="middle">${esc(p.label)}</text>`;
    }).join("");

    const bars = points.map((p, i) => {
      const h = p.value ? Math.max((p.value / top) * plotH, 2) : 0;
      const isPeak = i === peak && max > 0;
      return `<g class="bar" tabindex="0" role="img" data-index="${i}"
          aria-label="${esc(p.label)}, ${num(p.value)} memories">`
        + `<rect class="chart-hit" x="${L + slot * i}" y="${T}" width="${slot}" height="${plotH}"/>`
        + `<rect class="chart-bar${isPeak ? " peak" : ""}" x="${(x(i) - barWidth / 2).toFixed(1)}" y="${(T + plotH - h).toFixed(1)}" width="${barWidth.toFixed(1)}" height="${h.toFixed(1)}" rx="3"/>`
        + `</g>`;
    }).join("");

    svg.innerHTML = `<title>Activity · ${esc(windowLabel())}</title>`
      + `<desc>${points.length} buckets, average ${avg.toFixed(1)} per ${series.unit}, peak ${max}.</desc>`
      + grid + bars
      + `<line class="chart-average" x1="${L}" x2="${W - R}" y1="${y(avg)}" y2="${y(avg)}"/>`
      + `<text class="chart-axis chart-avg-label" x="${W - R}" y="${y(avg) - 5}" text-anchor="end">avg ${avg.toFixed(1)}</text>`
      + (max > 0 ? `<text class="chart-axis" x="${x(peak)}" y="${Math.max(10, y(max) - 6)}" text-anchor="middle">${max}</text>` : "")
      + ticks;

    note.innerHTML = max > 0
      ? `<span><strong>${avg.toFixed(1)}</strong> avg / ${series.unit}</span><span>Peak <strong>${num(max)}</strong> · ${esc(points[peak].label)}</span>`
      : `<span>No activity in this range.</span><span>${esc(windowLabel())}</span>`;

    selected.textContent = "Select a bar to read its exact count.";

    const tip = chartTooltip();
    const show = (node, event) => {
      const p = points[Number(node.dataset.index)];
      tip.innerHTML = `<strong>${esc(p.label)} · ${num(p.value)}</strong><span>${series.unit === "hour" ? "memories in this hour" : "memories this day"}</span>`;
      tip.classList.add("visible");
      const box = node.getBoundingClientRect();
      const px = event && event.clientX != null ? event.clientX : box.left + box.width / 2;
      const py = event && event.clientY != null ? event.clientY : box.top;
      tip.style.left = `${Math.max(8, Math.min(px + 12, window.innerWidth - tip.offsetWidth - 8))}px`;
      tip.style.top = `${Math.max(8, Math.min(py - tip.offsetHeight - 12, window.innerHeight - tip.offsetHeight - 8))}px`;
      selected.textContent = `${p.label} · ${num(p.value)} memories`;
    };
    const hide = () => tip.classList.remove("visible");

    for (const node of $$(".bar", svg)) {
      node.addEventListener("pointerenter", (e) => show(node, e));
      node.addEventListener("pointermove", (e) => show(node, e));
      node.addEventListener("pointerleave", hide);
      node.addEventListener("focus", () => show(node));
      node.addEventListener("blur", hide);
      node.addEventListener("click", (e) => show(node, e));
      node.addEventListener("keydown", (e) => {
        if (e.key === "Enter" || e.key === " ") { e.preventDefault(); show(node); }
      });
    }
  }

  function typeRows() {
    const d = state.overview || {};
    const allTime = state.typePeriod === "all";
    const source = allTime ? (d.types_all || []) : ((d.types || {})[state.window] || []);
    const rows = source
      .filter((t) => !(state.excludeSessions && t.type === "session_summary"))
      .slice()
      .sort((a, b) => b.count - a.count);
    return { rows, total: rows.reduce((a, t) => a + t.count, 0), allTime };
  }

  function typeRowHtml(name, count, total, max, aggregate = false) {
    const token = aggregate ? "--t-other" : typeToken(name);
    const share = total > 0 ? (count / total) * 100 : 0;
    return `<div class="type-row">
      <span class="type-row-name"><span class="type-dot" aria-hidden="true" style="--type-color:var(${token})"></span>${esc(name)}</span>
      <span class="type-track" aria-hidden="true"><span class="type-fill" style="--type-color:var(${token});--bar-width:${Math.max(1.5, (count / max) * 100)}%"></span></span>
      <span class="type-num">${num(count)}</span>
      <span class="type-share">${shareLabel(share)}</span>
    </div>`;
  }

  // The donut carries the shape, the rows carry the exact values. Five
  // segments at most: the top four types plus one neutral tail, so the chart
  // never becomes a rainbow and never replaces the list.
  function renderDonut(rows, total) {
    const donut = $("#donut");
    if (!donut) return;
    const top = rows.slice(0, 4).map((r) => ({ name: r.type, value: r.count, token: typeToken(r.type) }));
    const rest = total - top.reduce((a, s) => a + s.value, 0);
    const segs = rest > 0 ? [...top, { name: "Other types", value: rest, token: "--t-other" }] : top;

    const R = 76, C = 2 * Math.PI * R;
    let cum = 0;
    const arcs = segs.map((s) => {
      const frac = s.value / total;
      const len = Math.max(frac * C - 3, 0.5);
      const el = `<circle class="donut-seg" cx="100" cy="100" r="${R}" style="stroke:var(${s.token})"
          stroke-dasharray="${len.toFixed(2)} ${(C - len).toFixed(2)}"
          stroke-dashoffset="${(-cum).toFixed(2)}" transform="rotate(-90 100 100)">`
        + `<title>${esc(s.name)}: ${num(s.value)} (${shareLabel(frac * 100)})</title></circle>`;
      cum += frac * C;
      return el;
    }).join("");

    donut.setAttribute("aria-label", `Type distribution of ${num(total)} memories: `
      + segs.map((s) => `${s.name} ${shareLabel((s.value / total) * 100)}`).join(", "));
    donut.innerHTML = `<circle class="donut-track" cx="100" cy="100" r="${R}"/>` + arcs
      + `<text class="donut-value" x="100" y="104" text-anchor="middle">${num(total)}</text>`
      + `<text class="donut-caption" x="100" y="126" text-anchor="middle">${state.typePeriod === "all" ? "all time" : "in " + state.window}</text>`;
  }

  function renderTypes() {
    const grid = $("#type-grid");
    const donut = $("#donut");
    if (!grid || !donut) return;
    const { rows, total, allTime } = typeRows();

    const scope = allTime ? "All-time counts" : `Counts in the ${windowLabel().toLowerCase()}`;
    const windowEl = $("#type-window");
    if (windowEl) {
      windowEl.textContent = total > 0
        ? `${scope} · ${num(total)} total${state.excludeSessions ? " · excl. session_summary" : ""}`
        : scope;
    }

    if (!rows.length || total === 0) {
      grid.innerHTML = `<div class="type-empty">No type counts in this range.</div>`;
      donut.innerHTML = "";
      donut.removeAttribute("aria-label");
      return;
    }

    const max = rows[0].count;
    const shown = rows.slice(0, 9);
    const tail = rows.slice(9);
    let html = shown.map((t) => typeRowHtml(t.type, t.count, total, max)).join("");
    if (tail.length) {
      // The tail is aggregated honestly into one neutral row: nine one-count
      // types are not nine identities.
      const count = tail.reduce((a, t) => a + t.count, 0);
      html += typeRowHtml(`${tail.length} other types`, count, total, max, true);
    }
    grid.innerHTML = html;
    renderDonut(rows, total);
  }

  function vectorLabel(emb, coverage) {
    if (!emb.available) return "not installed";
    const dims = emb.dimensions ? `${emb.dimensions} dim` : "vector index";
    return `${dims} · ${coverage >= 100 ? "100% covered" : `${coverage.toFixed(1)}% covered`}`;
  }

  async function loadOverview() {
    const epoch = ++viewEpoch;
    loading("Reading the store…");
    try {
      const data = await api("/api/overview");
      if (epoch !== viewEpoch) return;
      state.overview = data;
      renderOverview();
    } catch (err) {
      if (epoch !== viewEpoch) return;
      failed(err, loadOverview);
    }
  }

  function renderOverview() {
    const d = state.overview || {};
    const h = d.health || {};
    const emb = d.embeddings || {};
    const rel = d.relations || {};
    const series = chartSeries();
    const totalInRange = series.points.reduce((a, p) => a + p.value, 0);
    const typeCount = (d.types_all || []).length;
    const coverage = emb.available ? (emb.coverage_pct || 0) : 0;
    const covered = emb.available && coverage >= 100;

    content.innerHTML = `
      <section class="kpi-grid" aria-label="Memory metrics">
        <article class="card kpi">
          <div class="kpi-top"><span class="kpi-label">Memories</span></div>
          <div class="kpi-value">${num(h.LiveObservations)}</div>
          <div class="kpi-foot">Across ${num(typeCount)} types · current total</div>
        </article>
        <article class="card kpi">
          <div class="kpi-top"><span class="kpi-label">New in range</span></div>
          <div class="kpi-value">${num(totalInRange)}</div>
          <div class="kpi-foot">${deltaHtml(totalInRange)}</div>
        </article>
        <article class="card kpi">
          <div class="kpi-top">
            <button class="kpi-action" type="button" data-goto-review aria-label="Open pending review">
              <span class="kpi-label">Pending review</span><span aria-hidden="true">↗</span>
            </button>
            <span class="pending-marker"><span aria-hidden="true">!</span>Action needed</span>
          </div>
          <div class="kpi-value">${num((state.health || {}).review_pending)}</div>
          <div class="kpi-foot">Items awaiting review</div>
        </article>
        <article class="card kpi">
          <div class="kpi-top"><span class="kpi-label">Relations</span></div>
          <div class="kpi-value">${num(rel.total)}</div>
          <div class="kpi-foot">Stored connections</div>
        </article>
      </section>

      <article class="card activity-card">
        <header class="card-header">
          <div><h2 class="card-title">Daily activity</h2><p class="card-subtitle" id="activity-window">${esc(windowLabel())}</p></div>
          <div class="header-actions">
            <div class="seg" role="group" aria-label="Chart range">
              ${["24h", "7d", "30d"].map((w) => `<button type="button" data-window="${w}" aria-pressed="${w === state.window}">${w}</button>`).join("")}
            </div>
          </div>
        </header>
        <div class="chart-wrap">
          <div class="chart-legend">
            <span><i class="swatch peak" aria-hidden="true"></i>Peak</span>
            <span><i class="swatch" aria-hidden="true"></i>Activity</span>
            <span><i class="rule" aria-hidden="true"></i>Average</span>
          </div>
          <svg class="chart" id="activity-chart" viewBox="0 0 1000 240" role="group" aria-label="Memory activity"></svg>
          <div class="chart-selected" id="chart-selected" aria-live="polite"></div>
          <div class="chart-note" id="chart-note"></div>
        </div>
      </article>

      <div class="lower-grid">
        <article class="card">
          <header class="card-header">
            <div><h2 class="card-title">Memory types</h2><p class="card-subtitle" id="type-window"></p></div>
            <div class="header-actions">
              <label class="check"><input type="checkbox" id="exclude-sessions"${state.excludeSessions ? " checked" : ""}>Exclude session_summary</label>
              <select class="filter" id="type-period" aria-label="Memory type period">
                <option value="range"${state.typePeriod === "range" ? " selected" : ""}>In range</option>
                <option value="all"${state.typePeriod === "all" ? " selected" : ""}>All time</option>
              </select>
            </div>
          </header>
          <div class="type-body">
            <div class="donut-wrap">
              <svg class="donut" id="donut" viewBox="0 0 200 200" role="img"></svg>
              <p class="donut-note">Top 4 types · rest grouped</p>
            </div>
            <div class="type-rows" id="type-grid" aria-live="polite"></div>
          </div>
        </article>

        <section class="card" aria-label="System health">
          <header class="card-header"><div><h2 class="card-title">System health</h2><p class="card-subtitle">Vector index</p></div></header>
          <div class="system-stack">
            <div class="system-item">
              <span class="system-label">Vector coverage</span>
              <span class="health-pill${covered ? "" : " warn"}"><span aria-hidden="true">${covered ? "✓" : "!"}</span>${esc(vectorLabel(emb, coverage))}</span>
              ${covered ? "" : `<div class="progress-track"><div class="progress-fill" style="width:${Math.max(0, Math.min(100, coverage))}%"></div></div>`}
            </div>
            <div class="system-item">
              <span class="system-label">Indexed memories</span>
              <span class="system-value">${num(emb.total_embedded || 0)} / ${num(h.LiveObservations || 0)}</span>
            </div>
            <div class="system-item">
              <span class="system-label">Embedding model</span>
              <span class="model-name">${esc(emb.available && emb.model ? emb.model.split("/").pop() : "not installed")}</span>
            </div>
          </div>
        </section>
      </div>`;

    renderChart(series);
    renderTypes();

    $("#type-period").addEventListener("change", (e) => { state.typePeriod = e.target.value; renderTypes(); });
    $("#exclude-sessions").addEventListener("change", (e) => { state.excludeSessions = e.target.checked; renderTypes(); });
    $("[data-goto-review]", content).addEventListener("click", () => setView("review"));
  }

  // ---------------------------------------------------------------- memories

  async function loadMemories() {
    const epoch = ++viewEpoch;
    loading("Loading memories…");
    const f = state.filter;
    const q = new URLSearchParams();
    if (f.q) q.set("q", f.q);
    if (f.project) q.set("project", f.project);
    if (f.type) q.set("type", f.type);
    if (f.pinned) q.set("pinned", "1");
    if (f.deleted) q.set("deleted", "1");
    q.set("limit", f.limit);
    q.set("offset", f.offset);
    try {
      const data = await api(`/api/observations?${q}`);
      if (epoch !== viewEpoch) return;
      state.memories = data.observations || [];
      renderMemories();
    } catch (err) {
      if (epoch !== viewEpoch) return;
      failed(err, loadMemories);
    }
  }

  function renderMemories() {
    const f = state.filter;
    const rows = state.memories.map((o) => `
      <tr data-clickable="true" data-open="${o.ID}">
        <td class="mono" style="color:var(--fg-mute)">${o.ID}</td>
        <td><span class="chip" style="--chip:${hueOf(o.Type)}">${esc(o.Type)}</span></td>
        <td class="strong truncate">${o.Pinned ? `<span style="color:var(--pending)">★</span> ` : ""}${esc(o.Title)}</td>
        <td class="truncate" style="font-family:var(--font-mono);font-size:11.5px">${esc(o.Project || "—")}</td>
        <td class="mono" style="font-size:11.5px;white-space:nowrap">${esc(fmtDate(o.UpdatedAt || o.CreatedAt))}</td>
        <td>${o.HasVector ? `<span style="color:var(--healthy)" title="${esc(o.VectorDims)}d vector">●</span>` : `<span style="color:var(--fg-mute)" title="no vector">○</span>`}</td>
      </tr>`).join("");

    content.innerHTML = `
      <div class="sheet">
        <div class="sheet-head">
          <div>
            <h2 class="card-title">Memories</h2>
            <p class="card-subtitle">${num(state.memories.length)} shown${f.q ? ` for “${esc(f.q)}”` : ""}${f.project ? ` · project ${esc(f.project)}` : ""}${f.type ? ` · ${esc(f.type)}` : ""}</p>
          </div>
          <div class="sheet-filters">
            <input class="filter" type="text" placeholder="Filter by project…" value="${esc(f.project)}"
                   data-filter="project" aria-label="Filter by project" style="width:180px">
            <select class="filter" data-filter="type" aria-label="Filter by type">
              <option value="">All types</option>
              ${[...new Set((state.overview?.types_all || []).map((t) => t.type))]
                .map((t) => `<option value="${esc(t)}" ${f.type === t ? "selected" : ""}>${esc(t)}</option>`).join("")}
            </select>
            <label class="check"><input type="checkbox" data-filter="pinned" ${f.pinned ? "checked" : ""}> Pinned</label>
            <label class="check"><input type="checkbox" data-filter="deleted" ${f.deleted ? "checked" : ""}> Show deleted</label>
          </div>
        </div>
        ${state.memories.length ? `<table class="table">
          <thead><tr>
            <th style="width:52px">ID</th><th style="width:132px">Type</th><th>Title</th>
            <th style="width:150px">Project</th><th style="width:132px">Updated</th>
            <th style="width:44px" title="Has semantic vector">Vec</th>
          </tr></thead>
          <tbody>${rows}</tbody>
        </table>` : `<div class="state">
          <h3>Nothing matches</h3>
          <p>No memory matches these filters. Loosen them, or press <code>/</code> to search the full text index.</p>
        </div>`}
      </div>`;

    $$("[data-open]", content).forEach((tr) =>
      tr.addEventListener("click", () => openMemory(tr.dataset.open)));

    $$("[data-filter]", content).forEach((el) => {
      const apply = () => {
        const key = el.dataset.filter;
        state.filter[key] = el.type === "checkbox" ? el.checked : el.value.trim();
        state.filter.offset = 0;
        loadMemories();
      };
      el.addEventListener("change", apply);
      if (el.type === "text") el.addEventListener("change", apply);
    });
  }

  // ---------------------------------------------------------------- relations

  async function loadRelations() {
    const epoch = ++viewEpoch;
    loading("Loading the relation graph…");
    try {
      const data = await api("/api/relations?limit=200");
      if (epoch !== viewEpoch) return;
      state.relations = data.relations || [];
      state.relationsSummary = data.summary || {};
      renderRelations();
    } catch (err) {
      if (epoch !== viewEpoch) return;
      failed(err, loadRelations);
    }
  }

  function renderRelations() {
    const sum = state.relationsSummary || {};
    const kinds = Object.entries(sum.by_kind || {}).sort((a, b) => b[1] - a[1]);
    const maxKind = Math.max(1, ...kinds.map(([, n]) => n));

    const rows = state.relations.map((r) => `
      <tr>
        <td><span class="tag ${r.relation === "conflicts_with" ? "tag-conflict" : "tag-judged"}">${esc(r.relation)}</span></td>
        <td><span class="tag tag-${esc(r.judgment_status)}">${esc(r.judgment_status)}</span></td>
        <td class="truncate"><button class="link" data-open="${esc(r.source_obs_id || r.source_id)}">${esc(r.source_title || r.source_id)}</button></td>
        <td class="truncate"><button class="link" data-open="${esc(r.target_obs_id || r.target_id)}">${esc(r.target_title || r.target_id)}</button></td>
        <td class="truncate">${esc(r.reason || "—")}</td>
        <td class="mono" style="text-align:right">${r.confidence ? r.confidence.toFixed(2) : "—"}</td>
      </tr>`).join("");

    content.innerHTML = `
      <div class="instrument">
        <section class="panel">
          <div class="panel-head">
            <h2 class="panel-title">Relation kinds</h2>
            <span class="panel-note">${num(sum.total)} judged · ${num(sum.pending)} pending</span>
          </div>
          <div class="typebars">
            ${kinds.map(([kind, n]) => `
              <div class="typebar" aria-disabled="true">
                <span class="typebar-label"><i class="type-dot" style="background:${kind === "conflicts_with" ? "var(--danger)" : kind === "supersedes" ? "var(--accent)" : "var(--t-decision)"}"></i><b>${esc(kind)}</b></span>
                <span class="typebar-track"><span class="typebar-fill" style="width:${(n / maxKind) * 100}%;background:${kind === "conflicts_with" ? "var(--danger)" : kind === "supersedes" ? "var(--accent)" : "var(--t-decision)"}"></span></span>
                <span class="typebar-num">${num(n)}</span>
              </div>`).join("") || `<p class="sheet-note">No relations recorded.</p>`}
          </div>
        </section>
        <section class="panel">
          <div class="panel-head">
            <h2 class="panel-title">Judgement queue</h2>
            <span class="panel-note">memory_relations.judgment_status</span>
          </div>
          <div class="typebars">
            ${Object.entries(sum.by_status || {}).map(([st, n]) => `
              <div class="typebar" aria-disabled="true">
                <span class="typebar-label"><b>${esc(st)}</b></span>
                <span class="typebar-track"><span class="typebar-fill" style="width:${(n / Math.max(1, sum.total)) * 100}%;background:${st === "pending" ? "var(--warning)" : "var(--success)"}"></span></span>
                <span class="typebar-num">${num(n)}</span>
              </div>`).join("") || `<p class="sheet-note">Nothing to judge.</p>`}
          </div>
        </section>
      </div>

      <div class="sheet">
        <div class="sheet-head">
          <div>
            <h2 class="card-title">Most recent relations</h2>
            <p class="card-subtitle">${num(state.relations.length)} of ${num(sum.total)} shown</p>
          </div>
        </div>
        ${rows ? `<table class="table">
          <thead><tr>
            <th style="width:130px">Relation</th><th style="width:96px">Status</th>
            <th>Source</th><th>Target</th><th style="width:240px">Reason</th>
            <th style="width:70px;text-align:right">Conf.</th>
          </tr></thead><tbody>${rows}</tbody>
        </table>` : `<div class="state"><h3>No relations yet</h3><p>Engram records relations as memories are judged. Nothing has been judged in this store.</p></div>`}
      </div>`;

    $$("[data-open]", content).forEach((b) =>
      b.addEventListener("click", () => openMemory(b.dataset.open)));
  }

  // ---------------------------------------------------------------- sessions

  async function loadSessions() {
    const epoch = ++viewEpoch;
    loading("Loading sessions…");
    try {
      const data = await api("/api/sessions?limit=120");
      if (epoch !== viewEpoch) return;
      state.sessions = data.sessions || [];
      renderSessions();
    } catch (err) {
      if (epoch !== viewEpoch) return;
      failed(err, loadSessions);
    }
  }

  function renderSessions() {
    const rows = state.sessions.map((s) => `
      <tr data-clickable="true" data-timeline="${esc(s.ID)}">
        <td class="mono strong">${esc(s.ID)}</td>
        <td class="truncate">${esc(s.Project || "—")}</td>
        <td class="truncate" style="font-family:var(--font-mono);font-size:11.5px">${esc(s.Directory || "—")}</td>
        <td class="mono" style="text-align:right">${s.ObservationCount}</td>
        <td><span class="tag ${s.EndedAt ? "tag-closed" : "tag-live"}">${s.EndedAt ? "closed" : "active"}</span></td>
        <td class="mono" style="font-size:11.5px;white-space:nowrap">${esc(fmtDate(s.StartedAt))}</td>
      </tr>`).join("");

    content.innerHTML = `
      <div class="sheet">
        <div class="sheet-head">
          <div>
            <h2 class="card-title">Sessions</h2>
            <p class="card-subtitle">${num(state.sessions.length)} shown · select one to read its timeline</p>
          </div>
        </div>
        ${rows ? `<table class="table">
          <thead><tr>
            <th style="width:190px">ID</th><th style="width:140px">Project</th><th>Directory</th>
            <th style="width:56px;text-align:right">Obs</th><th style="width:80px">State</th>
            <th style="width:132px">Started</th>
          </tr></thead><tbody>${rows}</tbody>
        </table>` : `<div class="state"><h3>No sessions</h3><p>This store has no recorded sessions yet.</p></div>`}
      </div>`;

    $$("[data-timeline]", content).forEach((tr) =>
      tr.addEventListener("click", () => openTimeline(tr.dataset.timeline)));
  }

  async function openTimeline(id) {
    const epoch = ++viewEpoch;
    loading("Reading the session timeline…");
    try {
      state.timeline = await api(`/api/sessions/${encodeURIComponent(id)}/timeline`);
      if (epoch !== viewEpoch) return;
      renderTimeline();
    } catch (err) {
      if (epoch !== viewEpoch) return;
      failed(err, () => loadSessions());
    }
  }

  function renderTimeline() {
    const t = state.timeline;
    const obs = t.observations || [];
    const prompts = t.prompts || [];

    const entries = obs.map((o) => `
      <div style="display:grid;grid-template-columns:118px 1fr;gap:var(--s4);padding-bottom:var(--s4)">
        <div>
          <div class="mono" style="font-size:11.5px;color:var(--fg-mute)">${esc(fmtDate(o.CreatedAt))}</div>
          <div class="mono" style="font-size:11px;color:var(--fg-mute)">#${o.ID}</div>
        </div>
        <div>
          <div style="display:flex;align-items:center;gap:var(--s2);margin-bottom:3px">
            <span class="chip" style="--chip:${hueOf(o.Type)}">${esc(o.Type)}</span>
            <strong style="font-size:13px;font-weight:600">${esc(o.Title)}</strong>
          </div>
          <p style="font-size:12.5px;color:var(--fg-dim);line-height:1.5">${esc(o.Content.slice(0, 260))}${o.Content.length > 260 ? "…" : ""}</p>
        </div>
      </div>`).join("");

    content.innerHTML = `
      <div class="instrument" style="grid-template-columns:minmax(0,7fr) minmax(0,3fr)">
        <section class="panel">
          <div class="panel-head">
            <div style="display:flex;align-items:center;gap:12px;min-width:0">
              <button class="btn" data-back>← Sessions</button>
              <div style="min-width:0">
                <h2 class="card-title">Session</h2>
                <p class="card-subtitle" style="overflow:hidden;text-overflow:ellipsis;white-space:nowrap">${esc(t.session_id)}</p>
              </div>
            </div>
            <span class="panel-note">${obs.length} observations · ${prompts.length} prompts</span>
          </div>
          ${entries || `<div class="state"><h3>Empty session</h3><p>This session recorded no observations.</p></div>`}
        </section>
        <section class="panel">
          <div class="panel-head"><h2 class="panel-title">Prompts</h2></div>
          ${prompts.map((p) => `
            <div style="padding:var(--s3) 0;border-bottom:1px solid var(--line-soft)">
              <div class="mono" style="font-size:10.5px;color:var(--fg-mute);margin-bottom:3px">${esc(fmtDate(p.CreatedAt))}</div>
              <p style="font-size:12.5px;color:var(--fg-dim)">${esc(p.Content)}</p>
            </div>`).join("") || `<p class="sheet-note">No prompts recorded for this session.</p>`}
        </section>
      </div>`;

    $("[data-back]", content).addEventListener("click", loadSessions);
  }

  // ---------------------------------------------------------------- review

  async function loadReview() {
    const epoch = ++viewEpoch;
    loading("Loading the review queue…");
    try {
      const data = await api("/api/review-queue");
      if (epoch !== viewEpoch) return;
      state.review = data.queue || [];
      renderReview();
    } catch (err) {
      if (epoch !== viewEpoch) return;
      failed(err, loadReview);
    }
  }

  function renderReview() {
    const rows = state.review.map((o) => `
      <tr>
        <td class="mono" style="color:var(--fg-mute)">${o.ID}</td>
        <td><span class="chip" style="--chip:${hueOf(o.Type)}">${esc(o.Type)}</span></td>
        <td class="strong truncate"><button class="link" data-open="${o.ID}">${esc(o.Title)}</button></td>
        <td class="mono" style="font-size:11.5px;white-space:nowrap">${esc(fmtDate(o.ReviewAfter))}</td>
        <td style="text-align:right"><button class="btn" data-review="${o.ID}">Mark reviewed</button></td>
      </tr>`).join("");

    content.innerHTML = `
      <div class="sheet">
        <div class="sheet-head">
          <div>
            <h2 class="card-title">Scheduled for review</h2>
            <p class="card-subtitle">${num(state.review.length)} memories carry a review_after date</p>
          </div>
        </div>
        ${rows ? `<table class="table">
          <thead><tr>
            <th style="width:52px">ID</th><th style="width:132px">Type</th><th>Title</th>
            <th style="width:150px">Review after</th><th style="width:150px;text-align:right">Action</th>
          </tr></thead><tbody>${rows}</tbody>
        </table>` : `<div class="state">
          <h3>Nothing to review</h3>
          <p>No memory in this store has a scheduled review date. Everything here is considered current.</p>
        </div>`}
      </div>`;

    $$("[data-open]", content).forEach((b) => b.addEventListener("click", () => openMemory(b.dataset.open)));
    $$("[data-review]", content).forEach((b) =>
      b.addEventListener("click", () => markReviewed(b.dataset.review)));
  }

  // ---------------------------------------------------------------- system

  async function loadSystem() {
    const epoch = ++viewEpoch;
    loading("Inspecting the store…");
    try {
      const data = await api("/api/health");
      if (epoch !== viewEpoch) return;
      state.health = data;
      renderSystem();
    } catch (err) {
      if (epoch !== viewEpoch) return;
      failed(err, loadSystem);
    }
  }

  function renderSystem() {
    const data = state.health;
    const h = data.health || {};
    const emb = data.embeddings || {};

    const invariants = [
      ["Live memories", num(h.LiveObservations), null],
      ["Soft-deleted kept", num(h.DeletedCount), null],
      ["Sessions sharing a project", num(h.AmbiguousSessions), h.AmbiguousSessions > 0],
      ["Projects with no session", num(h.ProjectsWithoutSession), h.ProjectsWithoutSession > 0],
      ["Memories on a missing session", num(h.OrphanSessions), h.OrphanSessions > 0],
      ["Duplicate rows collapsed", num(h.DuplicateCount), null],
      ["Pinned", num(h.PinnedCount), null],
      ["Expiring within 30d", num(h.ExpiringSoon), h.ExpiringSoon > 0],
    ];

    content.innerHTML = `
      <div class="instrument" style="grid-template-columns:repeat(auto-fit,minmax(360px,1fr))">
        <section class="panel">
          <div class="panel-head"><h2 class="panel-title">Store invariants</h2></div>
          <div class="typebars">
            ${invariants.map(([label, value, bad]) => `
              <div class="typebar typebar-kv" aria-disabled="true">
                <span class="typebar-label"><b>${esc(label)}</b></span>
                <span class="typebar-num" style="color:${bad ? "var(--conflict)" : "var(--fg)"}">${value}</span>
              </div>`).join("")}
          </div>
        </section>

        <section class="panel">
          <div class="panel-head"><h2 class="panel-title">Vector store</h2><span class="panel-note">observation_embeddings</span></div>
          <div class="typebars">
            ${[
              ["Index state", emb.available ? "active" : "absent", null],
              ["Vectors", num(emb.total_embedded), null],
              ["Pending", num(emb.pending_count), emb.pending_count > 0],
              ["Dimensions", emb.dimensions ? `${emb.dimensions} · float32` : "—", null],
              ["Model", emb.model ? emb.model.split("/").pop() : "—", null],
              ["Last vector", fmtDate(emb.latest_at), null],
            ].map(([label, value, bad]) => `
              <div class="typebar typebar-kv" aria-disabled="true">
                <span class="typebar-label"><b>${esc(label)}</b></span>
                <span class="typebar-num" style="color:${bad ? "var(--pending)" : "var(--fg)"}">${esc(value)}</span>
              </div>`).join("")}
          </div>
        </section>

        <section class="panel">
          <div class="panel-head"><h2 class="panel-title">Storage & wiring</h2></div>
          <div class="typebars">
            ${[
              ["Database", bytes(data.db_size), null],
              ["WAL", bytes(data.wal_size), null],
              ["Read path", "sqlite mode=ro", null],
              ["Write path", data.api_status === "connected" ? "engram serve" : `engram serve (${data.api_status})`, data.api_status !== "connected"],
              ["Auto-embedder", "engram-embed-watch", null],
            ].map(([label, value, bad]) => `
              <div class="typebar typebar-kv" aria-disabled="true">
                <span class="typebar-label"><b>${esc(label)}</b></span>
                <span class="typebar-num" style="color:${bad ? "var(--pending)" : "var(--fg)"}">${esc(value)}</span>
              </div>`).join("")}
          </div>
        </section>
      </div>

      <div class="rail" style="margin-top:var(--s4)">
        <div class="metric"><div class="metric-value">${num(h.TotalObservations)}</div><div class="metric-label">Observations total</div></div>
        <div class="metric"><div class="metric-value">${num(h.TotalSessions)}</div><div class="metric-label">Sessions total</div></div>
        <div class="metric"><div class="metric-value">${num(h.TotalPrompts)}</div><div class="metric-label">Prompts recorded</div></div>
        <div class="metric"><div class="metric-value">${num(h.Projects)}</div><div class="metric-label">Projects</div></div>
        <div class="metric"><div class="metric-value">${emb.available ? pct(emb.coverage_pct) : "—"}</div><div class="metric-label">Vector coverage</div></div>
        <div class="metric"><div class="metric-value">${num(data.relations)}</div><div class="metric-label">Relations</div></div>
      </div>`;
  }

  // ---------------------------------------------------------------- drawer

  async function openMemory(ref) {
    try {
      const data = await api(`/api/observations/${encodeURIComponent(ref)}`);
      state.memory = data.observation;
      state.memoryRelations = data.relations || [];
      state.editing = false;
      renderDrawer();
      drawer.hidden = false;
      backdrop.hidden = false;
      $("[data-drawer-close]").focus();
    } catch (err) {
      toast(`Could not open memory #${ref}: ${err.message}`, "error");
    }
  }

  function closeDrawer() {
    drawer.hidden = true;
    backdrop.hidden = true;
    state.memory = null;
  }

  function renderDrawer() {
    const o = state.memory;
    $("[data-drawer-ref]").textContent = `#${o.ID} · ${o.Project || "unassigned"} · ${o.Scope}`;
    $("[data-drawer-title]").textContent = o.Title;

    const rels = state.memoryRelations.length ? `
      <div>
        <h3 class="panel-title" style="margin-bottom:var(--s3)">Relations (${state.memoryRelations.length})</h3>
        <div class="rels">
          ${state.memoryRelations.map((r) => {
            const other = String(r.source_obs_id || r.source_id) === String(o.ID) ? r.target_obs_id || r.target_id : r.source_obs_id || r.source_id;
            const otherTitle = String(r.source_obs_id || r.source_id) === String(o.ID) ? r.target_title : r.source_title;
            return `<div class="rel">
              <button class="link" data-open="${esc(other)}">${esc(otherTitle || other)}</button>
              <span class="tag ${r.relation === "conflicts_with" ? "tag-conflict" : "tag-judged"}">${esc(r.relation)}</span>
            </div>`;
          }).join("")}
        </div>
      </div>` : "";

    $("[data-drawer-body]").innerHTML = state.editing ? editForm(o) : `
      <div class="kv">
        <div><div class="kv-label">Type</div><div class="kv-value"><span class="chip" style="--chip:${hueOf(o.Type)}">${esc(o.Type)}</span></div></div>
        <div><div class="kv-label">Vector</div><div class="kv-value" style="color:${o.HasVector ? "var(--healthy)" : "var(--fg-mute)"}">${o.HasVector ? `yes · ${o.VectorDims}d` : "not embedded"}</div></div>
        <div><div class="kv-label">Session</div><div class="kv-value">${esc(o.SessionID || "—")}</div></div>
        <div><div class="kv-label">Topic key</div><div class="kv-value">${esc(o.TopicKey || "—")}</div></div>
        <div><div class="kv-label">Created</div><div class="kv-value">${esc(fmtDate(o.CreatedAt))}</div></div>
        <div><div class="kv-label">Revisions</div><div class="kv-value">${o.RevisionCount} · ${o.DuplicateCount} seen</div></div>
      </div>
      <div class="prose">${esc(o.Content)}</div>
      ${rels}`;

    $("[data-drawer-foot]").innerHTML = state.editing ? `
      <button class="btn" data-cancel>Cancel</button>
      <div class="drawer-actions"><button class="btn btn-primary" data-save>Save changes</button></div>`
      : `<div class="drawer-actions">
          <button class="btn" data-copy>Copy id</button>
          <button class="btn" data-pin>${o.Pinned ? "Unpin" : "Pin"}</button>
        </div>
        <div class="drawer-actions">
          <button class="btn" data-edit>Edit</button>
          <button class="btn btn-danger" data-delete>Delete</button>
        </div>`;

    wireDrawer();
  }

  function editForm(o) {
    return `<div class="form">
      <div class="form-row"><label for="e-title">Title</label><input id="e-title" value="${esc(o.Title)}"></div>
      <div class="form-row"><label for="e-type">Type</label><input id="e-type" value="${esc(o.Type)}"></div>
      <div class="form-row"><label for="e-content">Content (Markdown)</label><textarea id="e-content">${esc(o.Content)}</textarea></div>
    </div>`;
  }

  function wireDrawer() {
    const body = $("[data-drawer-body]");
    const foot = $("[data-drawer-foot]");
    const o = state.memory;

    $$("[data-open]", body).forEach((b) => b.addEventListener("click", () => openMemory(b.dataset.open)));

    const on = (sel, fn) => { const el = $(sel, foot); if (el) el.addEventListener("click", fn); };

    on("[data-copy]", () => {
      navigator.clipboard.writeText(String(o.ID));
      toast(`Copied #${o.ID}`);
    });
    on("[data-pin]", async () => {
      try {
        await api(`/api/observations/${o.ID}/pin`, { method: o.Pinned ? "DELETE" : "PUT" });
        toast(o.Pinned ? "Unpinned" : "Pinned");
        await openMemory(o.ID);
        if (state.view === "memories") loadMemories();
      } catch (err) { toast(err.message, "error"); }
    });
    on("[data-edit]", () => { state.editing = true; renderDrawer(); });
    on("[data-cancel]", () => { state.editing = false; renderDrawer(); });
    on("[data-save]", saveMemory);
    on("[data-delete]", deleteMemory);
  }

  async function saveMemory() {
    const o = state.memory;
    try {
      await api(`/api/observations/${o.ID}`, {
        method: "PATCH",
        body: {
          expected_project: o.Project,
          title: $("#e-title", $("[data-drawer-body]")).value.trim(),
          type: $("#e-type", $("[data-drawer-body]")).value.trim(),
          content: $("#e-content", $("[data-drawer-body]")).value,
        },
      });
      toast("Memory updated");
      state.editing = false;
      await openMemory(o.ID);
      if (state.view === "memories") loadMemories();
    } catch (err) {
      toast(err.message, "error");
    }
  }

  async function deleteMemory() {
    const o = state.memory;
    if (!confirm(`Move memory #${o.ID} to the trash? It stays recoverable.`)) return;
    try {
      await api(`/api/observations/${o.ID}?expected_project=${encodeURIComponent(o.Project)}`, { method: "DELETE" });
      toast(`Memory #${o.ID} deleted`);
      closeDrawer();
      if (state.view === "memories") loadMemories();
    } catch (err) {
      toast(err.message, "error");
    }
  }

  async function markReviewed(id) {
    try {
      await api(`/api/observations/${id}/review`, { method: "POST" });
      toast(`Memory #${id} marked reviewed`);
      loadReview();
    } catch (err) {
      toast(err.message, "error");
    }
  }

  // ---------------------------------------------------------------- chrome

  const VIEWS = {
    overview: { title: "Overview", sub: "Shape, mix and momentum of the memory store", load: loadOverview, tools: true },
    memories: { title: "Memories", sub: "Search, filter and manage observations", load: loadMemories },
    relations: { title: "Relations", sub: "Judged links between memories, and what is still unjudged", load: loadRelations },
    sessions: { title: "Sessions", sub: "Agent runs and the timeline each one produced", load: loadSessions },
    review: { title: "Review", sub: "Memories carrying a scheduled review date", load: loadReview },
    system: { title: "System & Health", sub: "Invariants, vector store and how this process is wired", load: loadSystem },
  };

  function setView(name) {
    const v = VIEWS[name];
    if (!v) return;
    state.view = name;
    $$(".nav-item").forEach((b) => {
      if (b.dataset.view === name) b.setAttribute("aria-current", "page");
      else b.removeAttribute("aria-current");
    });
    pageTitle.textContent = v.title;
    pageSub.textContent = v.sub;
    if (name !== "memories") state.filter.q = "";
    searchInput.value = state.filter.q;
    closeDrawer();
    v.load();
  }

  async function pollHealth() {
    const dot = $("[data-engine-dot]");
    const label = $("[data-engine-label]");
    try {
      const data = await api("/api/health");
      state.health = data;
      const up = data.api_status === "connected";
      dot.className = `dot ${up ? "dot-up" : "dot-down"}`;
      label.textContent = up ? "engram serve connected" : "engram serve offline";
      label.className = "engine-text";
      label.style.color = up ? "var(--healthy)" : "var(--conflict)";
      $("[data-db-size]").textContent = `db ${bytes(data.db_size)}`;
      const walChip = $("[data-wal-size]");
      walChip.textContent = `wal ${bytes(data.wal_size)}`;
      // The WAL is where unmerged writes live; when it approaches the database
      // its size stops being trivia. The ratio is stated, not implied.
      const walRatio = data.db_size > 0 ? data.wal_size / data.db_size : 0;
      walChip.dataset.warn = walRatio >= 0.5 ? "true" : "false";
      walChip.title = data.db_size > 0
        ? `WAL is ${Math.round(walRatio * 100)}% of the database size`
        : "";
      const h = data.health || {};
      $("[data-count='memories']").textContent = num(h.LiveObservations);
      $("[data-count='sessions']").textContent = num(h.TotalSessions);
      $("[data-count='relations']").textContent = num(data.relations);
      $("[data-count='review']").textContent = num(data.review_pending);
    } catch {
      dot.className = "dot dot-wait";
      label.textContent = "manager unreachable";
      label.className = "engine-text";
      label.style.color = "var(--pending)";
    }
  }

  // ---------------------------------------------------------------- events

      $$(".nav-item").forEach((b) => b.addEventListener("click", () => setView(b.dataset.view)));
  $("[data-refresh]").addEventListener("click", () => { VIEWS[state.view].load(); pollHealth(); });
  $("[data-drawer-close]").addEventListener("click", closeDrawer);
  backdrop.addEventListener("click", closeDrawer);

  searchInput.addEventListener("keydown", (e) => {
    if (e.key !== "Enter") return;
    state.filter.q = searchInput.value.trim();
    setView("memories");
  });

  // The range control lives in the chart card, next to the data it changes, and
  // is delegated so a re-render never leaves it unwired.
  function setWindow(next) {
    if (!next || next === state.window) return;
    state.window = next;
    // A dashboard range selection resets the type card to the same range.
    state.typePeriod = "range";
    if (state.view === "overview" && state.overview) renderOverview();
  }

  document.addEventListener("click", (e) => {
    const b = e.target.closest("[data-window]");
    if (b) setWindow(b.dataset.window);
  });

  document.addEventListener("keydown", (e) => {
    if (e.key === "Escape") { closeDrawer(); return; }
    const typing = /^(INPUT|TEXTAREA|SELECT)$/.test(document.activeElement.tagName);
    if (typing) return;
    if (e.key === "/") { e.preventDefault(); searchInput.focus(); }
    if (e.key === "r" && !e.metaKey && !e.ctrlKey) { VIEWS[state.view].load(); pollHealth(); }
  });

  pollHealth();
  setView("overview");
  setInterval(pollHealth, 20000);
})();