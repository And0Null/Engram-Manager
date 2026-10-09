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

  const $ = (sel, root = document) => root.querySelector(sel);
  const $$ = (sel, root = document) => Array.from(root.querySelectorAll(sel));

  const content = $("#content");
  const pageTitle = $("[data-page-title]");
  const pageSub = $("[data-page-sub]");
  const searchInput = $("[data-search]");
  const windowSwitch = $("[data-window-switch]");
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
    content.innerHTML = `<div class="state"><div class="spinner"></div><p>${esc(label)}</p></div>`;
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

  // ---------------------------------------------------------------- radar

    // Polar multi-ring arc fan: all type arcs start together at top-left (280°)
  // and sweep clockwise on their own concentric ring track by rank.
  // Match the reference chart's iconic radar sweep fan appearance.
  function polarToXY(cx, cy, r, deg) {
    const rad = ((deg - 90) * Math.PI) / 180;
    return [cx + r * Math.cos(rad), cy + r * Math.sin(rad)];
  }

  function arcPath(cx, cy, r, a0, a1) {
    if (a1 - a0 >= 359.99) {
      const mid = a0 + 180;
      return arcPath(cx, cy, r, a0, mid) + " " + arcPath(cx, cy, r, mid, a0 + 360);
    }
    const large = a1 - a0 > 180 ? 1 : 0;
    const [x0, y0] = polarToXY(cx, cy, r, a0);
    const [x1, y1] = polarToXY(cx, cy, r, a1);
    return `M ${x0.toFixed(1)} ${y0.toFixed(1)} A ${r.toFixed(1)} ${r.toFixed(1)} 0 ${large} 1 ${x1.toFixed(1)} ${y1.toFixed(1)}`;
  }

  function sectorPath(cx, cy, rIn, rOut, a0, a1) {
    const large = a1 - a0 > 180 ? 1 : 0;
    const [x0, y0] = polarToXY(cx, cy, rOut, a0);
    const [x1, y1] = polarToXY(cx, cy, rOut, a1);
    const [x2, y2] = polarToXY(cx, cy, rIn, a1);
    const [x3, y3] = polarToXY(cx, cy, rIn, a0);
    return `M ${x0.toFixed(1)} ${y0.toFixed(1)} A ${rOut} ${rOut} 0 ${large} 1 ${x1.toFixed(1)} ${y1.toFixed(1)}
            L ${x2.toFixed(1)} ${y2.toFixed(1)} A ${rIn} ${rIn} 0 ${large} 0 ${x3.toFixed(1)} ${y3.toFixed(1)} Z`;
  }

  const RADAR_MAX_RINGS = 8;

  function buildRadar(types, total) {
    const cx = 150, cy = 150;
    const rIn = 50, rOut = 138;
    const step = (rOut - rIn) / (RADAR_MAX_RINGS - 1);
    const startAngle = 280; // Start top-left baseline

    const ranked = types.slice(0, RADAR_MAX_RINGS - 1);
    const tail = types.slice(RADAR_MAX_RINGS - 1);
    if (tail.length) {
      const n = tail.reduce((a, t) => a + t.count, 0);
      const share = tail.reduce((a, t) => a + t.pct, 0);
      ranked.push({ type: `${tail.length} more types`, count: n, pct: share, tail: true });
    }

    // Concentric grid circles (radar target)
    let gridRings = "";
    for (let r = 25; r <= 145; r += 10) {
      gridRings += `<circle cx="${cx}" cy="${cy}" r="${r}" fill="none" stroke="#121e2c" stroke-width="0.8"/>`;
    }

    // Translucent radar sweep cone behind max arc
    const maxSweep = Math.max(10, (ranked[0]?.pct || 30) * 3.6);
    const sweepCone = `<path d="${sectorPath(cx, cy, 20, 142, startAngle, startAngle + maxSweep)}"
      fill="rgba(45, 212, 191, 0.035)" stroke="rgba(45, 212, 191, 0.12)" stroke-width="0.8"/>`;

    const needlePos = polarToXY(cx, cy, 142, startAngle + maxSweep);
    const needleLine = `<line x1="${cx}" y1="${cy}" x2="${needlePos[0].toFixed(1)}" y2="${needlePos[1].toFixed(1)}"
      stroke="rgba(45, 212, 191, 0.35)" stroke-width="1.2" stroke-dasharray="3,3"/>`;

    let tracks = "";
    let arcs = "";

    ranked.forEach((t, i) => {
      const r = rOut - i * step; // Rank 0 is outermost ring
      const sweep = Math.max(3.5, (t.count / total) * 360);
      const colour = t.tail ? FALLBACK_HUE : hueOf(t.type);

      // Dark background ring track
      tracks += `<circle class="radar-ring" cx="${cx}" cy="${cy}" r="${r.toFixed(1)}" stroke="#15212f" stroke-width="7" fill="none" opacity="0.6"/>`;

      // Vibrant rounded arc segment starting at baseline
      const pathD = arcPath(cx, cy, r, startAngle, startAngle + sweep);
      arcs += `<path class="radar-arc" data-type="${esc(t.type)}"
        d="${pathD}" stroke="${colour}" stroke-width="7.5" stroke-linecap="round" fill="none" opacity="0.95">
        <title>${esc(t.type)} — ${num(t.count)} memories · ${pct(t.pct)}</title>
      </path>`;
    });

    return `<svg class="radar" viewBox="0 0 300 300" role="img"
        aria-label="Distribution of ${ranked.length} memory types over ${num(total)} memories">
      ${gridRings}
      ${sweepCone}
      ${needleLine}
      ${tracks}
      ${arcs}
      <circle cx="${cx}" cy="${cy}" r="${rIn - 6}" fill="var(--ink-850)" stroke="var(--line)" stroke-width="1.5"/>
      <text class="radar-hub-value" x="${cx}" y="${cy + 5}" text-anchor="middle">${num(total)}</text>
      <text class="radar-hub-label" x="${cx}" y="${cy + 21}" text-anchor="middle">OBSERVACIONES</text>
      <text class="radar-hub-sub" x="${cx}" y="${cy + 33}" text-anchor="middle">${num(total)} activas en total</text>
    </svg>`;
  }

  // Hovering an arc, a bar or a legend chip lights the same type everywhere.
  // One delegated handler, because the three views of the same data are
  // rendered in the same pass.
  function wireCrossHighlight() {
    const marks = $$("[data-type]", content);
    const paint = (type) => {
      for (const el of marks) {
        const hit = type !== null && el.dataset.type === type;
        el.dataset.hot = hit ? "true" : "false";
        if (el.classList.contains("radar-arc")) el.dataset.dim = hit ? "false" : "true";
      }
    };
    for (const el of marks) {
      el.addEventListener("mouseenter", () => paint(el.dataset.type));
      el.addEventListener("focus", () => paint(el.dataset.type));
      el.addEventListener("mouseleave", () => paint(null));
      el.addEventListener("blur", () => paint(null));
    }
  }

  // ---------------------------------------------------------------- pieces

  function typeBars(types, limit = 10) {
    if (!types.length) return `<p class="sheet-note">No types recorded.</p>`;
    const shown = types.slice(0, limit);
    const rest = types.slice(limit);
    let rows = shown.map((t) => {
      const w = Math.max(1.5, t.pct);
      return `<button class="typebar" data-type="${esc(t.type)}">
        <span class="typebar-label"><i class="legend-swatch" style="background:${hueOf(t.type)}"></i><b>${esc(t.type)}</b></span>
        <span class="typebar-track"><span class="typebar-fill" style="width:${w}%;background:${hueOf(t.type)}"></span></span>
        <span class="typebar-num">${num(t.count)} <span>· ${pct(t.pct)}</span></span>
      </button>`;
    }).join("");
    if (rest.length) {
      const count = rest.reduce((a, t) => a + t.count, 0);
      const share = rest.reduce((a, t) => a + t.pct, 0);
      rows += `<div class="typebar" aria-disabled="true">
        <span class="typebar-label"><i class="legend-swatch" style="background:${FALLBACK_HUE}"></i><b>${rest.length} other types</b></span>
        <span class="typebar-track"><span class="typebar-fill" style="width:${Math.max(1.5, share)}%;background:${FALLBACK_HUE}"></span></span>
        <span class="typebar-num">${num(count)} <span>· ${pct(share)}</span></span>
      </div>`;
    }
    return `<div class="typebars">${rows}</div>`;
  }

  function legendChips(types, limit = 9) {
    const shown = types.slice(0, limit);
    const tail = types.slice(limit);
    const chips = shown.map((t) => `<button class="legend-item" data-type="${esc(t.type)}">
      <i class="legend-swatch" style="background:${hueOf(t.type)}"></i>
      <span>${esc(t.type)}</span>
      <span class="legend-count">${num(t.count)}</span>
      <span class="legend-pct">${pct(t.pct)}</span>
    </button>`).join("");
    if (!tail.length) return chips;
    // The tail is on its own ring inside the radar; naming it here keeps the
    // legend honest without listing eleven one-count types.
    const n = tail.reduce((a, t) => a + t.count, 0);
    const share = tail.reduce((a, t) => a + t.pct, 0);
    return chips + `<span class="legend-item" style="cursor:default">
      <i class="legend-swatch" style="background:${FALLBACK_HUE}"></i>
      <span>${tail.length} smaller types</span>
      <span class="legend-count">${num(n)}</span>
      <span class="legend-pct">${pct(share)}</span>
    </span>`;
  }

  // Real daily buckets, newest at the right. Every bar is a day that exists.
  function activityStrip(days) {
    if (!days || !days.length) return "";
    const max = Math.max(1, ...days.map((d) => d.count));
    return days.map((d) => {
      const isPeak = d.count === max && d.count > 0;
      return `<i class="strip-bar" style="height:${Math.max(2, (d.count / max) * 100)}%"
        data-peak="${isPeak}" title="${esc(fmtDay(d.date))} — ${num(d.count)} memories"></i>`;
    }).join("");
  }

  function miniSpark(days, n = 14) {
    const slice = (days || []).slice(-n);
    if (!slice.length) return "";
    const max = Math.max(1, ...slice.map((d) => d.count));
    return slice.map((d) => `<i style="height:${Math.max(1, (d.count / max) * 100)}%"
      title="${esc(fmtDay(d.date))} — ${num(d.count)}"></i>`).join("");
  }

  function shareBars(values, hue) {
    const max = Math.max(1, ...values.map((v) => v.n));
    return values.map((v) => `<i style="height:${Math.max(1, (v.n / max) * 100)}%;background:${hue}"
      title="${esc(v.label)} — ${num(v.n)}"></i>`).join("");
  }

  // ---------------------------------------------------------------- overview

  async function loadOverview() {
    loading("Reading the store…");
    try {
      state.overview = await api("/api/overview");
      renderOverview();
    } catch (err) {
      failed(err, loadOverview);
    }
  }

  function renderOverview() {
    const d = state.overview;
    const h = d.health || {};
    const act = d.activity || {};
    const rel = d.relations || {};
    const emb = d.embeddings || {};
    const daily = d.daily || { days: [], avg_per_day: 0, peak_count: 0, peak_date: "", active_days: 0, total: 0 };
    const projects = d.projects || [];

    // The type mix shown in the radar is the whole store; the bars answer the
    // sharper question, "what are we writing right now", over the selected
    // window. Both windows ship with the payload so switching is instant and
    // the number on screen always states the window it belongs to.
    const WINDOW_LABEL = { "24h": "last 24 hours", "7d": "last seven days", "30d": "last thirty days" };
    const radarTypes = d.types_all || [];
    const barTypes = (d.types || {})[state.window] || [];
    const windowTotal = barTypes.reduce((a, t) => a + t.count, 0);
    const windowLabel = WINDOW_LABEL[state.window] || state.window;

    const peakCount = Math.max(0, ...daily.days.map((x) => x.count));
    const strip = activityStrip(daily.days);
    const last14 = miniSpark(daily.days);
    const topProjects = projects.slice(0, 8).map((p) => ({ label: p.Name, n: p.Observations }));

    const vectorPct = emb.available ? emb.coverage_pct : 0;

    content.innerHTML = `
      <div class="instrument">
        <section class="panel radar-wrap">
          <div class="panel-head">
            <h2 class="panel-title">DISTRIBUCIÓN POR TIPO</h2>
            <span class="panel-note">${radarTypes.length} TIPOS · ${num(h.LiveObservations || 0)} CARGADAS</span>
          </div>
          <div class="radar-stage">${buildRadar(radarTypes, h.LiveObservations || 0)}</div>
          <div class="legend">${legendChips(radarTypes)}</div>
        </section>

        <section class="panel">
          <div class="readout-block">
            <div class="readout-cap"><span>RITMO · ÚLTIMOS ${state.window.toUpperCase()}</span><span>${barTypes.length} TIPOS</span></div>
            <div class="readout-headline">${num(windowTotal)}<span class="readout-unit">/ ${state.window}</span></div>
            <div class="readout-label">${num(windowTotal)} de ${num(h.LiveObservations || 0)} memorias se crearon en los ${windowLabel}</div>
            ${typeBars(barTypes)}
          </div>

          <div class="readout-block">
            <div class="readout-cap">
              <span>Daily activity · 30d</span>
              <span>avg ${daily.avg_per_day.toFixed(1)} · peak ${num(peakCount)}${daily.peak_date ? " on " + esc(fmtDay(daily.peak_date)) : ""}</span>
            </div>
            <div class="strip">${strip}</div>
            <div class="strip-axis"><span>${esc(fmtDay(daily.days[0]?.date || ""))}</span><span>${esc(fmtDay(daily.days[daily.days.length - 1]?.date || ""))}</span></div>
          </div>

          <div class="readout-block">
            <div class="readout-cap">
              <span>Vector store coverage</span>
              <span>${emb.available ? `${num(emb.total_embedded)} / ${num(emb.live_count)} · ${emb.dimensions}d` : "not installed"}</span>
            </div>
            <div class="meter"><div class="meter-fill" data-warn="${emb.pending_count > 0}" style="width:${Math.min(100, vectorPct)}%"></div></div>
            <div class="strip-axis" style="margin-top:6px">
              <span>${emb.available ? esc(emb.model.split("/").pop()) : "run engram-embed to create the index"}</span>
              <span>${emb.pending_count > 0 ? `${num(emb.pending_count)} pending` : "all covered"}</span>
            </div>
          </div>
        </section>
      </div>

      <div class="rail">
        <div class="metric">
          <div class="metric-value">${num(h.LiveObservations)}</div>
          <div class="metric-label">Memories live</div>
          <div class="metric-note">${num(h.DeletedCount)} soft-deleted kept</div>
          <div class="metric-spark">${last14}</div>
        </div>
        <div class="metric">
          <div class="metric-value">${num(h.TotalSessions)}</div>
          <div class="metric-label">Sessions</div>
          <div class="metric-note">${num(h.ActiveSessions)} still active</div>
          <div class="metric-spark">${shareBars(projects.slice(0, 6).map((p) => ({ label: p.Name, n: p.Sessions })), "var(--t-manual)")}</div>
        </div>
        <div class="metric">
          <div class="metric-value">${num(h.Projects)}</div>
          <div class="metric-label">Projects</div>
          <div class="metric-note">${num(h.ProjectsWithoutSession)} unowned names</div>
          <div class="metric-spark">${shareBars(topProjects, "var(--t-config)")}</div>
        </div>
        <div class="metric">
          <div class="metric-value">${num(act.last_24h)}</div>
          <div class="metric-label">Last 24 hours</div>
          <div class="metric-note">${act.last_24h > daily.avg_per_day ? "above" : "below"} the ${daily.avg_per_day.toFixed(0)}/day average</div>
          <div class="metric-spark">${last14}</div>
        </div>
        <div class="metric">
          <div class="metric-value">${num(act.last_7d)}</div>
          <div class="metric-label">Last 7 days</div>
          <div class="metric-note">${num(act.last_30d)} in 30 days</div>
          <div class="metric-spark">${last14}</div>
        </div>
        <div class="metric">
          <div class="metric-value">${num(rel.total)}</div>
          <div class="metric-label">Relations</div>
          <div class="metric-note">${num(rel.conflicts)} conflicting · ${num(rel.pending)} unjudged</div>
          <div class="metric-spark">${shareBars(Object.entries(rel.by_kind || {}).sort((a, b) => b[1] - a[1]).slice(0, 6).map(([label, n]) => ({ label, n })), "var(--t-decision)")}</div>
        </div>
      </div>`;

    wireCrossHighlight();
  }

  // ---------------------------------------------------------------- memories

  async function loadMemories() {
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
      state.memories = data.observations || [];
      renderMemories();
    } catch (err) {
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
          <div class="sheet-filters">
            <input class="filter" type="text" placeholder="Filter by project…" value="${esc(f.project)}"
                   data-filter="project" style="width:180px">
            <select class="filter" data-filter="type">
              <option value="">All types</option>
              ${[...new Set((state.overview?.types_all || []).map((t) => t.type))]
                .map((t) => `<option value="${esc(t)}" ${f.type === t ? "selected" : ""}>${esc(t)}</option>`).join("")}
            </select>
            <label class="check"><input type="checkbox" data-filter="pinned" ${f.pinned ? "checked" : ""}> Pinned</label>
            <label class="check"><input type="checkbox" data-filter="deleted" ${f.deleted ? "checked" : ""}> Show deleted</label>
          </div>
          <span class="sheet-note">${num(state.memories.length)} shown${f.q ? ` for “${esc(f.q)}”` : ""}</span>
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
    loading("Loading the relation graph…");
    try {
      const data = await api("/api/relations?limit=200");
      state.relations = data.relations || [];
      state.relationsSummary = data.summary || {};
      renderRelations();
    } catch (err) {
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
                <span class="typebar-label"><i class="legend-swatch" style="background:${kind === "conflicts_with" ? "var(--conflict)" : kind === "supersedes" ? "var(--accent)" : "var(--t-decision)"}"></i><b>${esc(kind)}</b></span>
                <span class="typebar-track"><span class="typebar-fill" style="width:${(n / maxKind) * 100}%;background:${kind === "conflicts_with" ? "var(--conflict)" : kind === "supersedes" ? "var(--accent)" : "var(--t-decision)"}"></span></span>
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
                <span class="typebar-track"><span class="typebar-fill" style="width:${(n / Math.max(1, sum.total)) * 100}%;background:${st === "pending" ? "var(--pending)" : "var(--healthy)"}"></span></span>
                <span class="typebar-num">${num(n)}</span>
              </div>`).join("") || `<p class="sheet-note">Nothing to judge.</p>`}
          </div>
        </section>
      </div>

      <div class="sheet">
        <div class="sheet-head">
          <span class="sheet-title">Most recent relations</span>
          <span class="sheet-note">${num(state.relations.length)} of ${num(sum.total)} shown</span>
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
    loading("Loading sessions…");
    try {
      const data = await api("/api/sessions?limit=120");
      state.sessions = data.sessions || [];
      renderSessions();
    } catch (err) {
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
          <span class="sheet-title">Sessions</span>
          <span class="sheet-note">Select one to read its timeline</span>
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
    loading("Reading the session timeline…");
    try {
      state.timeline = await api(`/api/sessions/${encodeURIComponent(id)}/timeline`);
      renderTimeline();
    } catch (err) {
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
            <button class="btn" data-back>← Sessions</button>
            <span class="panel-note">${obs.length} observations · ${prompts.length} prompts</span>
          </div>
          <div class="mono" style="font-size:13px;color:var(--fg);margin-bottom:var(--s4)">${esc(t.session_id)}</div>
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
    loading("Loading the review queue…");
    try {
      const data = await api("/api/review-queue");
      state.review = data.queue || [];
      renderReview();
    } catch (err) {
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
          <span class="sheet-title">Scheduled for review</span>
          <span class="sheet-note">${num(state.review.length)} memories carry a review_after date</span>
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
    loading("Inspecting the store…");
    try {
      const data = await api("/api/health");
      state.health = data;
      renderSystem();
    } catch (err) {
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
    windowSwitch.hidden = !v.tools;
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
      $("[data-wal-size]").textContent = `wal ${bytes(data.wal_size)}`;
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

  $$("[data-window]").forEach((b) => b.addEventListener("click", () => {
    state.window = b.dataset.window;
    $$("[data-window]").forEach((x) => x.setAttribute("aria-pressed", String(x === b)));
    if (state.view === "overview" && state.overview) renderOverview();
  }));

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