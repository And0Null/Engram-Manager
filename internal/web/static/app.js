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
    memoryTotal: 0,
    sessionTotal: 0,
    reviewTotal: 0,
    overview: null,
    health: null,
    memories: [],
    filter: { q: "", type: "", project: "", pinned: false, deleted: false, limit: 100, offset: 0 },
    relations: [],
    relationsSummary: null,
    sessions: [],
    timeline: null,
    review: [],
    radar: { tasks: [], total: 0, filter: { project: "", status: "", priority: "", category: "", q: "" } },
    radarTask: null,
    projectsList: [],
    memory: null,
    memoryRelations: [],
    editing: false,
  };

  // Every load bumps this, and a response that is not from the newest load is
  // dropped. Without it a slow endpoint answers after the operator has moved
  // on and repaints its view under the wrong navigation item.
  let viewEpoch = 0;
  // The drawer is not a view: a separate counter so opening a memory cannot
  // cancel a view load, and a slow memory cannot land in a newer drawer.
  let drawerEpoch = 0;

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
  // "100 of 1436" when there is more to page through, just the number when the
  // page is the whole set.
  const shownOf = (shown, total) => (total > shown ? `${num(shown)} of ${num(total)}` : num(shown));
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
    // Daily buckets are UTC calendar days (see store.DailyActivity). Formatting
    // them in local time labels every bucket with the previous day west of
    // UTC, which attributes a count to the wrong date.
    const d = parseTs(iso);
    return d ? d.toLocaleDateString("es-ES", { day: "2-digit", month: "short", timeZone: "UTC" }) : iso;
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
          <div><h2 class="card-title">Daily activity</h2><p class="card-subtitle" id="activity-window">${esc(windowLabel())} · UTC</p></div>
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
              <span class="system-label">Agent processes</span>
              <span class="system-value mono">${(d.terminals && d.terminals.active_count > 0) ? `${d.terminals.active_count} active` : "idle"}</span>
            </div>
            <div class="system-item">
              <span class="system-label">Herdr socket</span>
              <span class="system-value mono" style="color:${d.terminals?.herdr?.available ? 'var(--healthy)' : 'var(--fg-mute)'}">${d.terminals?.herdr?.available ? "connected" : "absent"}</span>
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

  async function loadMemories(append = false) {
    const epoch = ++viewEpoch;
    if (!append) loading("Loading memories…");
    const f = state.filter;
    const q = new URLSearchParams();
    if (f.q) q.set("q", f.q);
    if (f.project) q.set("project", f.project);
    if (f.type) q.set("type", f.type);
    if (f.pinned) q.set("pinned", "1");
    if (f.deleted) q.set("deleted", "1");
    q.set("limit", f.limit);
    q.set("offset", append ? state.memories.length : f.offset);
    try {
      const data = await api(`/api/observations?${q}`);
      if (epoch !== viewEpoch) return;
      const rows = data.observations || [];
      state.memories = append ? state.memories.concat(rows) : rows;
      state.memoryTotal = data.total || state.memories.length;
      renderMemories();
    } catch (err) {
      if (epoch !== viewEpoch) return;
      failed(err, () => loadMemories());
    }
  }

  function renderMemories() {
    const f = state.filter;
    const shown = state.memories.length;
    const total = state.memoryTotal || shown;
    const more = total > shown
      ? `<div class="table-foot"><span class="sheet-note">${num(total - shown)} more in the store</span><button class="btn" type="button" data-more>Load more</button></div>`
      : "";
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
            <p class="card-subtitle">${shownOf(shown, total)} shown${f.q ? ` for “${esc(f.q)}”` : ""}${f.project ? ` · project ${esc(f.project)}` : ""}${f.type ? ` · ${esc(f.type)}` : ""}</p>
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
        </table>${more}` : `<div class="state">
          <h3>Nothing matches</h3>
          <p>No memory matches these filters. Loosen them, or press <code>/</code> to search the full text index.</p>
        </div>`}
      </div>`;

    $$("[data-open]", content).forEach((tr) =>
      tr.addEventListener("click", () => openMemory(tr.dataset.open)));
    $("[data-more]", content)?.addEventListener("click", () => loadMemories(true));

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

  async function loadSessions(append = false) {
    const epoch = ++viewEpoch;
    if (!append) loading("Loading sessions…");
    const offset = append ? state.sessions.length : 0;
    try {
      const data = await api(`/api/sessions?limit=120&offset=${offset}`);
      if (epoch !== viewEpoch) return;
      const rows = data.sessions || [];
      state.sessions = append ? state.sessions.concat(rows) : rows;
      state.sessionTotal = data.total || state.sessions.length;
      renderSessions();
    } catch (err) {
      if (epoch !== viewEpoch) return;
      failed(err, () => loadSessions());
    }
  }

  function renderSessions() {
    const shown = state.sessions.length;
    const total = state.sessionTotal || shown;
    const more = total > shown
      ? `<div class="table-foot"><span class="sheet-note">${num(total - shown)} more sessions</span><button class="btn" type="button" data-more>Load more</button></div>`
      : "";
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
            <p class="card-subtitle">${shownOf(shown, total)} shown · select one to read its timeline</p>
          </div>
        </div>
        ${rows ? `<table class="table">
          <thead><tr>
            <th style="width:190px">ID</th><th style="width:140px">Project</th><th>Directory</th>
            <th style="width:56px;text-align:right">Obs</th><th style="width:80px">State</th>
            <th style="width:132px">Started</th>
          </tr></thead><tbody>${rows}</tbody>
        </table>${more}` : `<div class="state"><h3>No sessions</h3><p>This store has no recorded sessions yet.</p></div>`}
      </div>`;

    $$("[data-timeline]", content).forEach((tr) =>
      tr.addEventListener("click", () => openTimeline(tr.dataset.timeline)));
    $("[data-more]", content)?.addEventListener("click", () => loadSessions(true));
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
    const ps = t.parsed_summary;
    const relTasks = t.related_tasks || [];

    const goal = ps?.goal || "";
    const nextSteps = ps?.next_steps || [];
    const decisions = ps?.decisions || [];
    const discoveries = ps?.discoveries || [];
    const accomplished = ps?.accomplished || [];

    const hasExpediente = goal || nextSteps.length || decisions.length || discoveries.length || accomplished.length;

    const expedienteHtml = hasExpediente ? `
      <section class="expediente-panel">
        <div class="expediente-head">
          <div class="expediente-title">
            <svg viewBox="0 0 24 24" width="18" height="18" fill="none" stroke="currentColor" stroke-width="2"><circle cx="12" cy="12" r="10"/><polyline points="12 6 12 12 16 14"/></svg>
            Expediente de Sesión (Resumen Estructurado)
          </div>
          <span class="panel-note">${esc(t.session?.Project || "")} · ${esc(fmtDate(t.session?.StartedAt || ""))}</span>
        </div>

        ${goal ? `<div class="expediente-goal"><strong>Objetivo:</strong> ${esc(goal)}</div>` : ""}

        ${nextSteps.length ? `
          <div class="expediente-section">
            <div class="expediente-sec-title">Próximos Pasos (Accionables)</div>
            <div class="expediente-list">
              ${nextSteps.map((step, idx) => `
                <div class="expediente-item">
                  <span>${esc(step)}</span>
                  <button class="btn-sm" data-step-to-radar="${idx}">+ A Radar</button>
                </div>
              `).join("")}
            </div>
          </div>
        ` : ""}

        <div style="display:grid;grid-template-columns:repeat(auto-fit, minmax(240px, 1fr));gap:var(--s3)">
          ${decisions.length ? `
            <div class="expediente-section">
              <div class="expediente-sec-title">Decisiones Clave</div>
              <ul style="padding-left:18px;font-size:12.5px;color:var(--text-secondary);line-height:1.5">
                ${decisions.map(d => `<li>${esc(d)}</li>`).join("")}
              </ul>
            </div>
          ` : ""}
          ${discoveries.length ? `
            <div class="expediente-section">
              <div class="expediente-sec-title">Descubrimientos / Aprendizajes</div>
              <ul style="padding-left:18px;font-size:12.5px;color:var(--text-secondary);line-height:1.5">
                ${discoveries.map(d => `<li>${esc(d)}</li>`).join("")}
              </ul>
            </div>
          ` : ""}
          ${accomplished.length ? `
            <div class="expediente-section">
              <div class="expediente-sec-title">Logros / Shipped</div>
              <ul style="padding-left:18px;font-size:12.5px;color:var(--text-secondary);line-height:1.5">
                ${accomplished.map(a => `<li>${esc(a)}</li>`).join("")}
              </ul>
            </div>
          ` : ""}
        </div>
      </section>
    ` : "";

    const relatedTasksHtml = relTasks.length ? `
      <section class="panel" style="margin-bottom:var(--s4)">
        <div class="panel-head">
          <h2 class="panel-title">Tareas de Action Radar Vinculadas (${relTasks.length})</h2>
        </div>
        <div style="display:flex;flex-wrap:wrap;gap:var(--s2);padding:var(--s3)">
          ${relTasks.map(tsk => `
            <div class="radar-card" style="flex:1;min-width:220px;cursor:pointer" data-open-radar="${esc(tsk.id)}">
              <div class="radar-card-head">
                <span class="tag ${tsk.priority === 'HIGH' ? 'tag-conflict' : tsk.priority === 'MEDIUM' ? 'tag-pending' : 'tag-live'}">${esc(tsk.priority)}</span>
                <span class="chip" style="font-size:10.5px">${esc(tsk.category)}</span>
                <span class="tag ${tsk.status === 'COMPLETED' ? 'tag-live' : 'tag-pending'}">${esc(tsk.status)}</span>
              </div>
              <div class="radar-card-title">${esc(tsk.title)}</div>
            </div>
          `).join("")}
        </div>
      </section>
    ` : "";

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
      ${expedienteHtml}
      ${relatedTasksHtml}
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

    $$("[data-step-to-radar]", content).forEach((btn) => {
      btn.addEventListener("click", async () => {
        const idx = Number(btn.dataset.stepToRadar);
        const stepText = nextSteps[idx];
        try {
          await api("/api/radar/tasks", {
            method: "POST",
            body: {
              project: t.session?.Project || "general",
              title: stepText,
              category: "FEATURE",
              priority: "MEDIUM",
              origin_session_id: t.session_id,
            },
          });
          toast("Tarea creada en Action Radar", "ok");
          openTimeline(t.session_id);
        } catch (err) {
          toast(err.message, "error");
        }
      });
    });

    $$("[data-open-radar]", content).forEach((card) => {
      card.addEventListener("click", () => {
        openRadarTaskDrawer(card.dataset.openRadar);
      });
    });
  }

  // ---------------------------------------------------------------- radar

  async function loadRadar() {
    const epoch = ++viewEpoch;
    loading("Cargando tareas de Action Radar…");
    try {
      const rf = state.radar.filter;
      const params = new URLSearchParams();
      if (rf.project) params.set("project", rf.project);
      if (rf.status) params.set("status", rf.status);
      if (rf.priority) params.set("priority", rf.priority);
      if (rf.category) params.set("category", rf.category);
      if (rf.q) params.set("q", rf.q);
      params.set("limit", "150");

      const [data, projData] = await Promise.all([
        api(`/api/radar/tasks?${params.toString()}`),
        state.projectsList.length ? Promise.resolve(null) : api("/api/projects").catch(() => null),
      ]);

      if (epoch !== viewEpoch) return;
      state.radar.tasks = data.tasks || [];
      state.radar.total = data.total || 0;
      if (projData?.projects) {
        state.projectsList = projData.projects;
      }
      renderRadar();
    } catch (err) {
      if (epoch !== viewEpoch) return;
      failed(err, loadRadar);
    }
  }

  function renderRadar() {
    const tasks = state.radar.tasks;
    const rf = state.radar.filter;

    const inProgress = tasks.filter((t) => t.status === "IN_PROGRESS");
    const pending = tasks.filter((t) => t.status === "PENDING" || t.status === "DEFERRED");
    const completed = tasks.filter((t) => t.status === "COMPLETED");

    const projects = state.projectsList || state.overview?.projects || [];

    const renderCard = (t) => `
      <div class="radar-card" data-task-id="${esc(t.id)}">
        <div class="radar-card-head">
          <div style="display:flex;align-items:center;gap:6px">
            <span class="tag ${t.priority === 'HIGH' ? 'tag-conflict' : t.priority === 'MEDIUM' ? 'tag-pending' : 'tag-live'}">${esc(t.priority)}</span>
            <span class="chip" style="font-size:10.5px">${esc(t.category)}</span>
          </div>
          <span class="mono" style="font-size:10.5px;color:var(--text-muted)">${esc(t.project)}</span>
        </div>
        <div class="radar-card-title">${esc(t.title)}</div>
        ${t.description ? `<div class="radar-card-desc">${esc(t.description)}</div>` : ""}
        <div class="radar-card-meta">
          <div>
            ${t.origin_session_id ? `<button class="link mono" style="font-size:10.5px" data-goto-session="${esc(t.origin_session_id)}">⎇ session</button>` : ""}
            ${t.notes && t.notes.length ? `<span style="margin-left:6px">${t.notes.length} nota(s)</span>` : ""}
          </div>
          <div style="display:flex;gap:4px">
            ${t.status === "PENDING" ? `<button class="btn-sm" data-move-status="${esc(t.id)}" data-to="IN_PROGRESS" title="Mover a En Curso">▶ Iniciar</button>` : ""}
            ${t.status === "IN_PROGRESS" ? `<button class="btn-sm" data-move-status="${esc(t.id)}" data-to="COMPLETED" title="Marcar como Completada">✓ Listo</button>` : ""}
            ${t.status === "COMPLETED" ? `<button class="btn-sm" data-move-status="${esc(t.id)}" data-to="PENDING" title="Reabrir">↺ Reabrir</button>` : ""}
          </div>
        </div>
      </div>
    `;

    content.innerHTML = `
      <div class="radar-toolbar">
        <div class="radar-filters">
          <select data-filter-proj style="background:var(--surface);color:var(--text);border:1px solid var(--border);padding:6px 10px;border-radius:var(--r-sm);font-size:12px">
            <option value="">Todos los Proyectos</option>
            ${projects.map((p) => `<option value="${esc(p.Name)}" ${rf.project === p.Name ? "selected" : ""}>${esc(p.Name)}</option>`).join("")}
          </select>
          <select data-filter-prio style="background:var(--surface);color:var(--text);border:1px solid var(--border);padding:6px 10px;border-radius:var(--r-sm);font-size:12px">
            <option value="">Todas las Prioridades</option>
            <option value="HIGH" ${rf.priority === "HIGH" ? "selected" : ""}>HIGH</option>
            <option value="MEDIUM" ${rf.priority === "MEDIUM" ? "selected" : ""}>MEDIUM</option>
            <option value="LOW" ${rf.priority === "LOW" ? "selected" : ""}>LOW</option>
          </select>
          <select data-filter-cat style="background:var(--surface);color:var(--text);border:1px solid var(--border);padding:6px 10px;border-radius:var(--r-sm);font-size:12px">
            <option value="">Todas las Categorías</option>
            <option value="FEATURE" ${rf.category === "FEATURE" ? "selected" : ""}>FEATURE</option>
            <option value="BUGFIX" ${rf.category === "BUGFIX" ? "selected" : ""}>BUGFIX</option>
            <option value="REFACTOR" ${rf.category === "REFACTOR" ? "selected" : ""}>REFACTOR</option>
            <option value="IDEA" ${rf.category === "IDEA" ? "selected" : ""}>IDEA</option>
          </select>
        </div>
        <button class="btn" data-new-task style="background:var(--accent);color:var(--app);font-weight:600">
          + Nueva Tarea
        </button>
      </div>

      <div class="radar-board">
        <div class="radar-col">
          <div class="radar-col-head">
            <span class="radar-col-title" style="color:var(--warning)">⚡ En Curso</span>
            <span class="radar-col-count">${inProgress.length}</span>
          </div>
          ${inProgress.map(renderCard).join("") || `<p class="sheet-note" style="text-align:center;padding:24px 0">Sin tareas en curso</p>`}
        </div>

        <div class="radar-col">
          <div class="radar-col-head">
            <span class="radar-col-title" style="color:var(--text-secondary)">📋 Pendientes / Backlog</span>
            <span class="radar-col-count">${pending.length}</span>
          </div>
          ${pending.map(renderCard).join("") || `<p class="sheet-note" style="text-align:center;padding:24px 0">Sin tareas pendientes</p>`}
        </div>

        <div class="radar-col">
          <div class="radar-col-head">
            <span class="radar-col-title" style="color:var(--success)">✓ Completadas</span>
            <span class="radar-col-count">${completed.length}</span>
          </div>
          ${completed.map(renderCard).join("") || `<p class="sheet-note" style="text-align:center;padding:24px 0">Sin tareas completadas</p>`}
        </div>
      </div>
    `;

    $("[data-filter-proj]", content).addEventListener("change", (e) => {
      rf.project = e.target.value;
      loadRadar();
    });
    $("[data-filter-prio]", content).addEventListener("change", (e) => {
      rf.priority = e.target.value;
      loadRadar();
    });
    $("[data-filter-cat]", content).addEventListener("change", (e) => {
      rf.category = e.target.value;
      loadRadar();
    });
    $("[data-new-task]", content).addEventListener("click", () => openNewTaskModal());

    $$("[data-move-status]", content).forEach((btn) => {
      btn.addEventListener("click", async (e) => {
        e.stopPropagation();
        const id = btn.dataset.moveStatus;
        const nextStatus = btn.dataset.to;
        try {
          await api(`/api/radar/tasks/${encodeURIComponent(id)}`, {
            method: "PATCH",
            body: { status: nextStatus },
          });
          toast(`Tarea actualizada a ${nextStatus}`, "ok");
          loadRadar();
        } catch (err) {
          toast(err.message, "error");
        }
      });
    });

    $$("[data-task-id]", content).forEach((card) => {
      card.addEventListener("click", (e) => {
        if (e.target.closest("button")) return;
        openRadarTaskDrawer(card.dataset.taskId);
      });
    });

    $$("[data-goto-session]", content).forEach((btn) => {
      btn.addEventListener("click", (e) => {
        e.stopPropagation();
        setView("sessions");
        openTimeline(btn.dataset.gotoSession);
      });
    });
  }

  async function openRadarTaskDrawer(id) {
    try {
      const task = await api(`/api/radar/tasks/${encodeURIComponent(id)}`);
      state.radarTask = task;

      $("[data-drawer-ref]").textContent = `#${task.id} · ${task.project}`;
      $("[data-drawer-title]").textContent = task.title;

      const notesHtml = (task.notes || []).map((n) => `
        <div style="background:var(--elevated);padding:8px 12px;border-radius:var(--r-sm);border:1px solid var(--border);margin-bottom:8px">
          <div style="display:flex;justify-content:space-between;font-size:11px;color:var(--text-muted);margin-bottom:4px">
            <strong style="color:var(--text)">${esc(n.author)}</strong>
            <span class="mono">${esc(fmtDate(n.created_at))}</span>
          </div>
          <p style="font-size:12.5px;color:var(--text-secondary);line-height:1.4">${esc(n.content)}</p>
        </div>
      `).join("") || `<p class="sheet-note">Sin notas aún.</p>`;

      $("[data-drawer-body]").innerHTML = `
        <div class="edit-form">
          <div class="form-row">
            <label for="rt-title">Título</label>
            <input id="rt-title" type="text" value="${esc(task.title)}">
          </div>
          <div style="display:grid;grid-template-columns:1fr 1fr;gap:var(--s3)">
            <div class="form-row">
              <label for="rt-status">Estado</label>
              <select id="rt-status">
                <option value="PENDING" ${task.status === "PENDING" ? "selected" : ""}>PENDING</option>
                <option value="IN_PROGRESS" ${task.status === "IN_PROGRESS" ? "selected" : ""}>IN_PROGRESS</option>
                <option value="COMPLETED" ${task.status === "COMPLETED" ? "selected" : ""}>COMPLETED</option>
                <option value="DEFERRED" ${task.status === "DEFERRED" ? "selected" : ""}>DEFERRED</option>
              </select>
            </div>
            <div class="form-row">
              <label for="rt-priority">Prioridad</label>
              <select id="rt-priority">
                <option value="HIGH" ${task.priority === "HIGH" ? "selected" : ""}>HIGH</option>
                <option value="MEDIUM" ${task.priority === "MEDIUM" ? "selected" : ""}>MEDIUM</option>
                <option value="LOW" ${task.priority === "LOW" ? "selected" : ""}>LOW</option>
              </select>
            </div>
          </div>
          <div class="form-row">
            <label for="rt-desc">Descripción</label>
            <textarea id="rt-desc" rows="4">${esc(task.description || "")}</textarea>
          </div>
          ${task.origin_session_id ? `
            <div class="form-row">
              <label>Sesión de Origen</label>
              <div><button class="link mono" id="rt-goto-origin">${esc(task.origin_session_id)}</button></div>
            </div>
          ` : ""}
          <div style="margin-top:var(--s4);border-top:1px solid var(--border);padding-top:var(--s3)">
            <h3 class="panel-title" style="margin-bottom:var(--s2)">Notas y Comentarios</h3>
            <div id="rt-notes-list">${notesHtml}</div>
            <div style="display:flex;gap:var(--s2);margin-top:var(--s3)">
              <input id="rt-new-note" type="text" placeholder="Escribir una nota rápida…" style="flex:1;background:var(--elevated);border:1px solid var(--border);border-radius:var(--r-sm);color:var(--text);padding:6px 10px;font-size:12.5px">
              <button class="btn" id="rt-add-note">Añadir</button>
            </div>
          </div>
        </div>
      `;

      $("[data-drawer-foot]").innerHTML = `
        <button class="btn" id="rt-save" style="background:var(--accent);color:var(--app);font-weight:600">Guardar Cambios</button>
        <button class="btn" id="rt-delete" style="color:var(--danger)">Eliminar Tarea</button>
      `;

      drawer.hidden = false;
      backdrop.hidden = false;

      $("#rt-goto-origin")?.addEventListener("click", () => {
        closeDrawer();
        setView("sessions");
        openTimeline(task.origin_session_id);
      });

      $("#rt-save").addEventListener("click", async () => {
        try {
          await api(`/api/radar/tasks/${encodeURIComponent(task.id)}`, {
            method: "PATCH",
            body: {
              title: $("#rt-title").value.trim(),
              status: $("#rt-status").value,
              priority: $("#rt-priority").value,
              description: $("#rt-desc").value.trim(),
            },
          });
          toast("Tarea guardada", "ok");
          closeDrawer();
          loadRadar();
        } catch (err) {
          toast(err.message, "error");
        }
      });

      $("#rt-delete").addEventListener("click", async () => {
        if (!confirm("¿Eliminar esta tarea de forma permanente?")) return;
        try {
          await api(`/api/radar/tasks/${encodeURIComponent(task.id)}`, { method: "DELETE" });
          toast("Tarea eliminada", "ok");
          closeDrawer();
          loadRadar();
        } catch (err) {
          toast(err.message, "error");
        }
      });

      $("#rt-add-note").addEventListener("click", async () => {
        const input = $("#rt-new-note");
        const val = input.value.trim();
        if (!val) return;
        try {
          await api(`/api/radar/tasks/${encodeURIComponent(task.id)}/notes`, {
            method: "POST",
            body: { author: "user", content: val },
          });
          toast("Nota agregada", "ok");
          openRadarTaskDrawer(task.id);
        } catch (err) {
          toast(err.message, "error");
        }
      });
    } catch (err) {
      toast(`No se pudo abrir la tarea: ${err.message}`, "error");
    }
  }

  function openNewTaskModal() {
    $("[data-drawer-ref]").textContent = "Action Radar";
    $("[data-drawer-title]").textContent = "Nueva Tarea";

    const projects = state.projectsList || state.overview?.projects || [];

    $("[data-drawer-body]").innerHTML = `
      <div class="edit-form">
        <div class="form-row">
          <label for="new-t-title">Título *</label>
          <input id="new-t-title" type="text" placeholder="Ej. Refactorizar store..." autofocus>
        </div>
        <div class="form-row">
          <label for="new-t-project">Proyecto</label>
          <input id="new-t-project" type="text" list="proj-datalist" placeholder="Engram-Manager" value="${esc(state.radar.filter.project || "general")}">
          <datalist id="proj-datalist">
            ${projects.map((p) => `<option value="${esc(p.Name)}">`).join("")}
          </datalist>
        </div>
        <div style="display:grid;grid-template-columns:1fr 1fr;gap:var(--s3)">
          <div class="form-row">
            <label for="new-t-cat">Categoría</label>
            <select id="new-t-cat">
              <option value="FEATURE">FEATURE</option>
              <option value="BUGFIX">BUGFIX</option>
              <option value="REFACTOR">REFACTOR</option>
              <option value="IDEA">IDEA</option>
            </select>
          </div>
          <div class="form-row">
            <label for="new-t-prio">Prioridad</label>
            <select id="new-t-prio">
              <option value="HIGH">HIGH</option>
              <option value="MEDIUM" selected>MEDIUM</option>
              <option value="LOW">LOW</option>
            </select>
          </div>
        </div>
        <div class="form-row">
          <label for="new-t-desc">Descripción</label>
          <textarea id="new-t-desc" rows="4" placeholder="Detalles o contexto de la tarea..."></textarea>
        </div>
      </div>
    `;

    $("[data-drawer-foot]").innerHTML = `
      <button class="btn" id="new-t-submit" style="background:var(--accent);color:var(--app);font-weight:600">+ Crear Tarea</button>
      <button class="btn" id="new-t-cancel">Cancelar</button>
    `;

    drawer.hidden = false;
    backdrop.hidden = false;

    $("#new-t-cancel").addEventListener("click", closeDrawer);
    $("#new-t-submit").addEventListener("click", async () => {
      const title = $("#new-t-title").value.trim();
      if (!title) {
        toast("El título es obligatorio", "error");
        return;
      }
      try {
        await api("/api/radar/tasks", {
          method: "POST",
          body: {
            title,
            project: $("#new-t-project").value.trim() || "general",
            category: $("#new-t-cat").value,
            priority: $("#new-t-prio").value,
            description: $("#new-t-desc").value.trim(),
          },
        });
        toast("Tarea creada con éxito", "ok");
        closeDrawer();
        loadRadar();
      } catch (err) {
        toast(err.message, "error");
      }
    });
  }

  // ---------------------------------------------------------------- review

  async function loadReview() {
    const epoch = ++viewEpoch;
    loading("Loading the review queue…");
    try {
      const data = await api("/api/review-queue");
      if (epoch !== viewEpoch) return;
      state.review = data.queue || [];
      state.reviewTotal = data.total || 0;
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
            <p class="card-subtitle">${shownOf(state.review.length, state.reviewTotal || state.review.length)} memories carry a review_after date</p>
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
    loading("Inspecting the store and host system…");
    try {
      const [data, terminals, projectsData] = await Promise.all([
        api("/api/health"),
        api("/api/system/terminals").catch(() => null),
        api("/api/projects").catch(() => null),
      ]);
      if (epoch !== viewEpoch) return;
      state.health = data;
      state.terminals = terminals;
      state.projectsWithGit = projectsData?.projects || [];
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
    const terms = state.terminals || { herdr: { available: false, agents: [] }, processes: [] };
    const projects = state.projectsWithGit || [];

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
              ["Engram DB", bytes(data.db_size), null],
              ["Engram WAL", bytes(data.wal_size), null],
              ["Read path", "sqlite mode=ro", null],
              ["Radar DB", "radar.db mode=rw", null],
              ["Write path", data.api_status === "connected" ? "engram serve" : `engram serve (${data.api_status})`, data.api_status !== "connected"],
              ["Auto-embedder", "engram-embed-watch", null],
            ].map(([label, value, bad]) => `
              <div class="typebar typebar-kv" aria-disabled="true">
                <span class="typebar-label"><b>${esc(label)}</b></span>
                <span class="typebar-num" style="color:${bad ? "var(--pending)" : "var(--fg)"}">${esc(value)}</span>
              </div>`).join("")}
          </div>
        </section>

        <section class="panel">
          <div class="panel-head">
            <h2 class="panel-title">Agentes & Terminales</h2>
            <span class="panel-note">${terms.processes ? terms.processes.length : 0} procesos activos</span>
          </div>
          <div class="typebars">
            <div class="typebar typebar-kv" aria-disabled="true">
              <span class="typebar-label"><b>Herdr socket</b></span>
              <span class="typebar-num" style="color:${terms.herdr?.available ? 'var(--healthy)' : 'var(--fg-mute)'}">${terms.herdr?.available ? "conectado" : "ausente"}</span>
            </div>
            ${(terms.processes || []).map((p) => `
              <div class="typebar typebar-kv" aria-disabled="true">
                <span class="typebar-label"><strong style="color:var(--accent)">PID ${p.pid}</strong> · ${esc(p.name)}</span>
                <span class="typebar-num mono" style="font-size:11px;color:var(--text-secondary);max-width:160px;overflow:hidden;text-overflow:ellipsis">${esc(p.command)}</span>
              </div>
            `).join("") || `<p class="sheet-note" style="padding:8px 0">Ningún proceso de agente en ejecución</p>`}
          </div>
        </section>

        <section class="panel" style="grid-column:1 / -1">
          <div class="panel-head">
            <h2 class="panel-title">Salud del Portafolio Git</h2>
            <span class="panel-note">${projects.length} proyectos registrados</span>
          </div>
          <div style="overflow-x:auto">
            <table class="table">
              <thead>
                <tr>
                  <th>Proyecto</th>
                  <th>Rama</th>
                  <th>Estado Working Tree</th>
                  <th>Último Commit</th>
                  <th style="text-align:right">Memorias</th>
                </tr>
              </thead>
              <tbody>
                ${projects.map((p) => {
                  const g = p.git;
                  return `
                    <tr>
                      <td class="strong">${esc(p.Name)}</td>
                      <td class="mono" style="font-size:11.5px">${g && g.is_repo ? `<span class="chip">${esc(g.branch || '—')}</span>` : '—'}</td>
                      <td>${g && g.is_repo ? (g.is_clean ? `<span class="tag tag-live">✓ limpio</span>` : `<span class="tag tag-conflict">⚡ dirty (${g.dirty_count})</span>`) : '<span class="tag tag-closed">no git</span>'}</td>
                      <td class="mono" style="font-size:11.5px;color:var(--text-secondary)">${g && g.last_commit ? `${esc(g.last_commit)} · ${esc(g.commit_time || '')}` : '—'}</td>
                      <td style="text-align:right" class="mono">${num(p.Observations)}</td>
                    </tr>
                  `;
                }).join("")}
              </tbody>
            </table>
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
    const epoch = ++drawerEpoch;
    try {
      const data = await api(`/api/observations/${encodeURIComponent(ref)}`);
      if (epoch !== drawerEpoch) return;
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
    radar: { title: "Action Radar", sub: "Backlog y tareas de seguimiento vinculadas a sesiones de IA", load: loadRadar },
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
      label.textContent = up ? "engram serve" : "engram offline";
      label.className = "engine-text";
      label.style.color = up ? "var(--healthy)" : "var(--conflict)";
      const dbEl = $("[data-db-size]");
      if (dbEl) dbEl.textContent = bytes(data.db_size);
      const walEl = $("[data-wal-size]");
      if (walEl) walEl.textContent = bytes(data.wal_size);
      const walMetric = $("[data-wal-metric]");
      const walRatio = data.db_size > 0 ? data.wal_size / data.db_size : 0;
      if (walMetric) {
        walMetric.dataset.warn = walRatio >= 0.5 ? "true" : "false";
        walMetric.title = data.db_size > 0
          ? `WAL is ${Math.round(walRatio * 100)}% of the database size`
          : "";
      }
      const h = data.health || {};
      $("[data-count='memories']").textContent = num(h.LiveObservations);
      $("[data-count='sessions']").textContent = num(h.TotalSessions);
      $("[data-count='relations']").textContent = num(data.relations);
      $("[data-count='review']").textContent = num(data.review_pending);
      pollRadarCount();
    } catch {
      dot.className = "dot dot-wait";
      label.textContent = "unreachable";
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

  async function pollRadarCount() {
    try {
      const data = await api("/api/radar/tasks?status=PENDING&limit=1");
      const badge = $("[data-count='radar']");
      if (badge) badge.textContent = num(data.total || 0);
    } catch {
      // ignore
    }
  }

  function initSSE() {
    const liveBadge = $("[data-live-status]");
    let es;
    try {
      es = new EventSource("/api/stream");
    } catch {
      if (liveBadge) liveBadge.dataset.active = "false";
      return;
    }

    es.onopen = () => {
      if (liveBadge) {
        liveBadge.dataset.active = "true";
        liveBadge.title = "Real-time SSE active";
      }
    };

    es.onerror = () => {
      if (liveBadge) {
        liveBadge.dataset.active = "false";
        liveBadge.title = "SSE reconnecting…";
      }
    };

    es.onmessage = (e) => {
      try {
        const msg = JSON.parse(e.data);
        if (msg.type === "engram-mutation") {
          pollHealth();
          if (state.view === "overview" || state.view === "memories" || state.view === "sessions") {
            VIEWS[state.view].load();
          }
        } else if (msg.type === "radar-mutation") {
          pollRadarCount();
          if (state.view === "radar") {
            loadRadar();
          } else if (state.view === "sessions" && state.timeline) {
            openTimeline(state.timeline.session_id);
          }
        }
      } catch {
        // ignore
      }
    };
  }

  pollHealth();
  initSSE();
  setView("overview");
  setInterval(pollHealth, 20000);
})();