/* Engram Manager — Client Application (Vanilla SPA) */
"use strict";

(function () {
  // State
  const state = {
    currentView: "overview",
    overview: null,
    health: null,
    memories: [],
    memoriesFilter: {
      q: "",
      project: "",
      type: "",
      scope: "",
      pinned: false,
      deleted: false,
      limit: 100,
      offset: 0,
    },
    relations: [],
    relationsSummary: null,
    sessions: [],
    selectedSession: null,
    sessionTimeline: null,
    selectedMemory: null,
    editingMemory: false,
  };

  // DOM Elements
  const $ = (sel) => document.querySelector(sel);
  const $$ = (sel) => Array.from(document.querySelectorAll(sel));

  const elMainContent = $("#content");
  const elPageTitle = $("#page-title");
  const elPageSubtitle = $("#page-subtitle");
  const elGlobalSearchInput = $("#global-search-input");
  const elBtnRefresh = $("#btn-refresh");

  const elDrawer = $("#drawer");
  const elDrawerBackdrop = $("#drawer-backdrop");
  const elDrawerClose = $("#drawer-close");
  const elDrawerBody = $("#drawer-body");
  const elDrawerFooter = $("#drawer-footer");
  const elDrawerTitle = $("#drawer-title");
  const elDrawerEyebrow = $("#drawer-eyebrow");

  const elEngineDot = $("#engine-dot");
  const elEngineLabel = $("#engine-label");
  const elDbSize = $("#db-size-label");
  const elWalSize = $("#wal-size-label");

  // Formatters
  function formatBytes(bytes) {
    if (!bytes || bytes === 0) return "0 B";
    const k = 1024;
    const sizes = ["B", "KiB", "MiB", "GiB"];
    const i = Math.floor(Math.log(bytes) / Math.log(k));
    return parseFloat((bytes / Math.pow(k, i)).toFixed(1)) + " " + sizes[i];
  }

  function formatDate(str) {
    if (!str) return "—";
    const d = new Date(str.includes("T") ? str : str.replace(" ", "T") + "Z");
    if (isNaN(d.getTime())) return str;
    return d.toLocaleString("es-ES", {
      dateStyle: "short",
      timeStyle: "short",
    });
  }

  function escapeHtml(str) {
    if (!str) return "";
    return String(str)
      .replace(/&/g, "&amp;")
      .replace(/</g, "&lt;")
      .replace(/>/g, "&gt;")
      .replace(/"/g, "&quot;")
      .replace(/'/g, "&#39;");
  }

  function showToast(message, isError = false) {
    const container = $("#toast-container");
    const toast = document.createElement("div");
    toast.className = `toast ${isError ? "toast-error" : ""}`;
    toast.textContent = message;
    container.appendChild(toast);
    setTimeout(() => {
      toast.style.opacity = "0";
      toast.style.transition = "opacity 0.3s";
      setTimeout(() => toast.remove(), 300);
    }, 2800);
  }

  // API calls
  async function api(path, opts = {}) {
    try {
      const res = await fetch(path, {
        headers: opts.body ? { "Content-Type": "application/json" } : {},
        method: opts.method || "GET",
        body: opts.body ? JSON.stringify(opts.body) : undefined,
      });
      const data = await res.json();
      if (!res.ok) {
        throw new Error(data.error || `HTTP ${res.status}`);
      }
      return data;
    } catch (err) {
      console.error(`API Error [${path}]:`, err);
      throw err;
    }
  }

  // Engine Status Check
  async function checkEngineStatus() {
    try {
      const data = await api("/api/health");
      state.health = data.health;

      if (data.api_status === "connected") {
        elEngineDot.className = "status-dot dot-connected";
        elEngineLabel.textContent = "Engram Serve: OK";
      } else {
        elEngineDot.className = "status-dot dot-unreachable";
        elEngineLabel.textContent = "Engram Serve: Inactivo";
      }

      elDbSize.textContent = `DB: ${formatBytes(data.db_size)}`;
      elWalSize.textContent = `WAL: ${formatBytes(data.wal_size)}`;

      if (data.health) {
        $("#nav-count-memories").textContent = data.health.LiveObservations || "0";
        $("#nav-count-sessions").textContent = data.health.TotalSessions || "0";
      }
    } catch (err) {
      elEngineDot.className = "status-dot dot-unreachable";
      elEngineLabel.textContent = "API error";
    }
  }

  // Routing & Views
  function setView(viewName) {
    state.currentView = viewName;
    $$(".nav-item").forEach((btn) => {
      btn.classList.toggle("active", btn.dataset.view === viewName);
    });

    switch (viewName) {
      case "overview":
        elPageTitle.textContent = "Overview";
        elPageSubtitle.textContent = "Panel general de memoria y métricas operativas";
        loadOverview();
        break;
      case "memories":
        elPageTitle.textContent = "Memories";
        elPageSubtitle.textContent = "Explorador de observaciones y base de conocimientos";
        loadMemories();
        break;
      case "relations":
        elPageTitle.textContent = "Relaciones y Conflictos";
        elPageSubtitle.textContent = "Red de relaciones semánticas entre observaciones";
        loadRelations();
        break;
      case "sessions":
        elPageTitle.textContent = "Sesiones de Agente";
        elPageSubtitle.textContent = "Línea de tiempo de sesiones y actividad de contexto";
        loadSessions();
        break;
      case "review":
        elPageTitle.textContent = "Cola de Revisión";
        elPageSubtitle.textContent = "Memorias programadas para verificación o caducidad";
        loadReviewQueue();
        break;
      case "system":
        elPageTitle.textContent = "Sistema & Salud";
        elPageSubtitle.textContent = "Diagnóstico de invariantes, almacén SQLite y WAL";
        loadSystem();
        break;
    }
  }

  // View: Overview
  async function loadOverview() {
    elMainContent.innerHTML = `<div class="loading-state"><div class="spinner"></div><span>Cargando visión general...</span></div>`;
    try {
      const data = await api("/api/overview");
      state.overview = data;
      renderOverview(data);
    } catch (err) {
      elMainContent.innerHTML = `<div class="empty-state">Error cargando overview: ${escapeHtml(err.message)}</div>`;
    }
  }

  function renderOverview(data) {
    const h = data.health || {};
    const act = data.activity || {};
    const rel = data.relations || {};
    const emb = data.embeddings || {};
    const projects = data.projects || [];

    // Type distribution calculations
    const typeEntries = Object.entries(act.by_type || {}).sort((a, b) => b[1] - a[1]);
    const totalTypeCount = typeEntries.reduce((acc, [, val]) => acc + val, 0) || 1;

    let typesHtml = "";
    typeEntries.slice(0, 7).forEach(([t, count]) => {
      const pct = Math.round((count / totalTypeCount) * 100);
      typesHtml += `
        <div class="dist-item">
          <div class="dist-item-header">
            <span class="badge badge-${escapeHtml(t)}">${escapeHtml(t)}</span>
            <span class="mono">${count} (${pct}%)</span>
          </div>
          <div class="dist-track">
            <div class="dist-fill" style="width: ${pct}%; background: var(--teal);"></div>
          </div>
        </div>
      `;
    });

    // Top projects
    let projRows = "";
    projects.slice(0, 6).forEach((p) => {
      projRows += `
        <tr onclick="window.filterByProject('${escapeHtml(p.Name)}')">
          <td style="font-weight: 600; color: var(--text-primary);">${escapeHtml(p.Name)}</td>
          <td class="mono">${p.Observations}</td>
          <td class="mono">${p.Sessions}</td>
          <td class="mono">${p.Prompts}</td>
        </tr>
      `;
    });

    elMainContent.innerHTML = `
      <div class="overview-stats" style="grid-template-columns: repeat(auto-fit, minmax(180px, 1fr));">
        <div class="stat-card">
          <span class="stat-label">Memorias Vivas</span>
          <span class="stat-value" style="color: var(--teal);">${h.LiveObservations || 0}</span>
          <span class="stat-sub">${h.TotalObservations || 0} total (${h.DeletedCount || 0} borradas)</span>
        </div>
        <div class="stat-card">
          <span class="stat-label">Actividad (24h)</span>
          <span class="stat-value" style="color: var(--cyan);">${act.last_24h || 0}</span>
          <span class="stat-sub">${act.last_7d || 0} en 7d · ${act.last_30d || 0} en 30d</span>
        </div>
        <div class="stat-card">
          <span class="stat-label">Vectores Semánticos</span>
          <span class="stat-value" style="color: var(--emerald);">${emb.total_embedded || 0}</span>
          <span class="stat-sub">${emb.available ? `${Math.round(emb.coverage_pct)}% cobertura · ${emb.dimensions}d` : "Inactivo"}</span>
        </div>
        <div class="stat-card">
          <span class="stat-label">Sesiones Totales</span>
          <span class="stat-value" style="color: var(--magenta);">${h.TotalSessions || 0}</span>
          <span class="stat-sub">${h.ActiveSessions || 0} activas ahora mismo</span>
        </div>
        <div class="stat-card">
          <span class="stat-label">Relaciones</span>
          <span class="stat-value" style="color: var(--purple);">${rel.total || 0}</span>
          <span class="stat-sub">${rel.conflicts || 0} conflictos · ${rel.pending || 0} pendientes</span>
        </div>
      </div>

      <div class="overview-sections">
        <div class="section-panel">
          <div class="panel-header">
            <div class="panel-title">
              <svg style="width:16px;height:16px;color:var(--teal);" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><path d="M21.21 15.89A10 10 0 1 1 8 2.83"/><path d="M22 12A10 10 0 0 0 12 2v10z"/></svg>
              <span>Distribución por Tipo</span>
            </div>
          </div>
          <div class="dist-list">
            ${typesHtml || '<span class="text-muted">Sin datos de tipos</span>'}
          </div>
        </div>

        <div class="section-panel">
          <div class="panel-header">
            <div class="panel-title">
              <svg style="width:16px;height:16px;color:var(--cyan);" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><path d="M22 19a2 2 0 0 1-2 2H4a2 2 0 0 1-2-2V5a2 2 0 0 1 2-2h5l2 3h9a2 2 0 0 1 2 2z"/></svg>
              <span>Top Proyectos</span>
            </div>
          </div>
          <table class="data-table">
            <thead>
              <tr>
                <th>Proyecto</th>
                <th>Obs</th>
                <th>Sesiones</th>
                <th>Prompts</th>
              </tr>
            </thead>
            <tbody>
              ${projRows || '<tr><td colspan="4" class="text-muted">Sin proyectos</td></tr>'}
            </tbody>
          </table>
        </div>
      </div>
    `;
  }

  // View: Memories
  async function loadMemories() {
    elMainContent.innerHTML = `<div class="loading-state"><div class="spinner"></div><span>Cargando lista de memorias...</span></div>`;
    const f = state.memoriesFilter;
    const params = new URLSearchParams();
    if (f.q) params.set("q", f.q);
    if (f.project) params.set("project", f.project);
    if (f.type) params.set("type", f.type);
    if (f.scope) params.set("scope", f.scope);
    if (f.pinned) params.set("pinned", "true");
    if (f.deleted) params.set("deleted", "true");
    params.set("limit", f.limit);
    params.set("offset", f.offset);

    try {
      const data = await api(`/api/observations?${params.toString()}`);
      state.memories = data.observations || [];
      renderMemories();
    } catch (err) {
      elMainContent.innerHTML = `<div class="empty-state">Error cargando memorias: ${escapeHtml(err.message)}</div>`;
    }
  }

  function renderMemories() {
    const f = state.memoriesFilter;
    const rows = state.memories.map((o) => {
      const pinIcon = o.Pinned ? `<span class="pinned-icon" title="Fijada">★</span>` : "";
      return `
        <tr onclick="window.openMemory(${o.ID})">
          <td class="mono" style="color: var(--text-muted); font-size: 11px;">#${o.ID}</td>
          <td><span class="badge badge-${escapeHtml(o.Type)}">${escapeHtml(o.Type)}</span></td>
          <td style="font-weight: 500; color: var(--text-primary); max-width: 320px; overflow: hidden; text-overflow: ellipsis; white-space: nowrap;">
            ${pinIcon}${escapeHtml(o.Title)}
          </td>
          <td class="mono" style="font-size: 12px;">${escapeHtml(o.Project || "—")}</td>
          <td style="font-size: 12px; color: var(--text-muted);">${formatDate(o.UpdatedAt || o.CreatedAt)}</td>
        </tr>
      `;
    }).join("");

    elMainContent.innerHTML = `
      <div class="data-table-wrapper">
        <div class="table-toolbar">
          <div class="toolbar-filters">
            <select class="filter-select" id="filter-type" onchange="window.onFilterChange('type', this.value)">
              <option value="">Todos los tipos</option>
              <option value="decision" ${f.type === "decision" ? "selected" : ""}>decision</option>
              <option value="bugfix" ${f.type === "bugfix" ? "selected" : ""}>bugfix</option>
              <option value="architecture" ${f.type === "architecture" ? "selected" : ""}>architecture</option>
              <option value="discovery" ${f.type === "discovery" ? "selected" : ""}>discovery</option>
              <option value="config" ${f.type === "config" ? "selected" : ""}>config</option>
              <option value="learning" ${f.type === "learning" ? "selected" : ""}>learning</option>
              <option value="pattern" ${f.type === "pattern" ? "selected" : ""}>pattern</option>
              <option value="session_summary" ${f.type === "session_summary" ? "selected" : ""}>session_summary</option>
            </select>
            <input type="text" class="filter-select" placeholder="Filtrar por proyecto..." value="${escapeHtml(f.project)}" onchange="window.onFilterChange('project', this.value)" style="width: 160px;" />
            <label style="display:flex;align-items:center;gap:6px;font-size:12px;color:var(--text-secondary);cursor:pointer;">
              <input type="checkbox" ${f.pinned ? "checked" : ""} onchange="window.onFilterChange('pinned', this.checked)" /> Fijadas
            </label>
            <label style="display:flex;align-items:center;gap:6px;font-size:12px;color:var(--text-secondary);cursor:pointer;">
              <input type="checkbox" ${f.deleted ? "checked" : ""} onchange="window.onFilterChange('deleted', this.checked)" /> Ver borradas
            </label>
          </div>
          <div style="font-size: 12px; color: var(--text-muted);">
            Mostrando ${state.memories.length} resultados
          </div>
        </div>
        <table class="data-table">
          <thead>
            <tr>
              <th style="width: 60px;">ID</th>
              <th style="width: 130px;">Tipo</th>
              <th>Título</th>
              <th style="width: 150px;">Proyecto</th>
              <th style="width: 140px;">Actualizado</th>
            </tr>
          </thead>
          <tbody>
            ${rows || '<tr><td colspan="5" class="empty-state">No se encontraron observaciones</td></tr>'}
          </tbody>
        </table>
      </div>
    `;
  }

  // View: Relations
  async function loadRelations() {
    elMainContent.innerHTML = `<div class="loading-state"><div class="spinner"></div><span>Cargando grafo de relaciones...</span></div>`;
    try {
      const data = await api("/api/relations?limit=150");
      state.relations = data.relations || [];
      state.relationsSummary = data.summary || {};
      renderRelations();
    } catch (err) {
      elMainContent.innerHTML = `<div class="empty-state">Error cargando relaciones: ${escapeHtml(err.message)}</div>`;
    }
  }

  function renderRelations() {
    const sum = state.relationsSummary || {};
    const rows = state.relations.map((r) => {
      const isConflict = r.relation === "conflicts_with";
      const srcId = r.source_obs_id || r.source_id;
      const tgtId = r.target_obs_id || r.target_id;
      return `
        <tr>
          <td class="mono" style="font-size: 11px;">#${r.id}</td>
          <td>
            <div style="display:flex;align-items:center;gap:6px;">
              <span class="badge ${isConflict ? "badge-conflict" : "badge-default"}">${escapeHtml(r.relation)}</span>
              <span class="badge badge-${escapeHtml(r.judgment_status)}">${escapeHtml(r.judgment_status)}</span>
            </div>
          </td>
          <td>
            <div style="display:flex;flex-direction:column;gap:2px;">
              <a href="javascript:void(0)" onclick="window.openMemory('${srcId}')" style="color:var(--cyan);text-decoration:none;font-weight:500;">
                #${srcId}: ${escapeHtml(r.source_title || "Memoria origen")}
              </a>
              <span style="font-size:11px;color:var(--text-muted);">↳ Hacia: <a href="javascript:void(0)" onclick="window.openMemory('${tgtId}')" style="color:var(--teal);text-decoration:none;">#${tgtId}: ${escapeHtml(r.target_title || "Memoria destino")}</a></span>
            </div>
          </td>
          <td style="font-size:12px;color:var(--text-secondary);max-width:260px;">
            ${escapeHtml(r.reason || "—")}
          </td>
          <td class="mono" style="font-size:11.5px;">${r.confidence ? r.confidence.toFixed(2) : "—"}</td>
        </tr>
      `;
    }).join("");

    elMainContent.innerHTML = `
      <div class="overview-stats" style="grid-template-columns: repeat(4, 1fr); margin-bottom: 20px;">
        <div class="stat-card">
          <span class="stat-label">Total Relaciones</span>
          <span class="stat-value" style="color:var(--purple);">${sum.total || 0}</span>
        </div>
        <div class="stat-card">
          <span class="stat-label">Conflictos</span>
          <span class="stat-value" style="color:var(--rose);">${sum.conflicts || 0}</span>
        </div>
        <div class="stat-card">
          <span class="stat-label">Pendientes de Juicio</span>
          <span class="stat-value" style="color:var(--amber);">${sum.pending || 0}</span>
        </div>
        <div class="stat-card">
          <span class="stat-label">Compatibles / Conexas</span>
          <span class="stat-value" style="color:var(--teal);">${(sum.by_kind?.compatible || 0) + (sum.by_kind?.related || 0)}</span>
        </div>
      </div>

      <div class="data-table-wrapper">
        <table class="data-table">
          <thead>
            <tr>
              <th style="width:60px;">ID</th>
              <th style="width:200px;">Relación & Estado</th>
              <th>Enlace (Origen → Destino)</th>
              <th>Razón / Evidencia</th>
              <th style="width:90px;">Confianza</th>
            </tr>
          </thead>
          <tbody>
            ${rows || '<tr><td colspan="5" class="empty-state">No hay relaciones registradas</td></tr>'}
          </tbody>
        </table>
      </div>
    `;
  }

  // View: Sessions & Timeline
  async function loadSessions() {
    elMainContent.innerHTML = `<div class="loading-state"><div class="spinner"></div><span>Cargando sesiones...</span></div>`;
    try {
      const data = await api("/api/sessions?limit=100");
      state.sessions = data.sessions || [];
      renderSessions();
    } catch (err) {
      elMainContent.innerHTML = `<div class="empty-state">Error cargando sesiones: ${escapeHtml(err.message)}</div>`;
    }
  }

  function renderSessions() {
    const rows = state.sessions.map((s) => {
      const isActive = !s.EndedAt;
      return `
        <tr onclick="window.openSessionTimeline('${escapeHtml(s.ID)}')">
          <td class="mono" style="color: var(--teal); font-weight: 600;">${escapeHtml(s.ID)}</td>
          <td style="font-weight: 500;">${escapeHtml(s.Project || "—")}</td>
          <td class="mono" style="font-size: 11.5px; max-width: 260px; overflow: hidden; text-overflow: ellipsis;">${escapeHtml(s.Directory || "—")}</td>
          <td class="mono">${s.ObservationCount}</td>
          <td>
            <span class="badge ${isActive ? "badge-discovery" : "badge-default"}">${isActive ? "ACTIVA" : "CERRADA"}</span>
          </td>
          <td style="font-size: 12px; color: var(--text-muted);">${formatDate(s.StartedAt)}</td>
        </tr>
      `;
    }).join("");

    elMainContent.innerHTML = `
      <div class="data-table-wrapper">
        <div class="table-toolbar">
          <div style="font-size: 13px; font-weight: 600;">Sesiones Registradas (${state.sessions.length})</div>
          <div style="font-size: 12px; color: var(--text-muted);">Haz clic en una sesión para ver su línea de tiempo cronológica</div>
        </div>
        <table class="data-table">
          <thead>
            <tr>
              <th style="width: 140px;">ID Sesión</th>
              <th style="width: 140px;">Proyecto</th>
              <th>Directorio de Trabajo</th>
              <th style="width: 90px;">Obs</th>
              <th style="width: 100px;">Estado</th>
              <th style="width: 140px;">Iniciada</th>
            </tr>
          </thead>
          <tbody>
            ${rows || '<tr><td colspan="6" class="empty-state">No hay sesiones registradas</td></tr>'}
          </tbody>
        </table>
      </div>
    `;
  }

  // Session Timeline View
  async function openSessionTimeline(sessionId) {
    elMainContent.innerHTML = `<div class="loading-state"><div class="spinner"></div><span>Cargando línea de tiempo de la sesión...</span></div>`;
    try {
      const data = await api(`/api/sessions/${encodeURIComponent(sessionId)}/timeline`);
      state.sessionTimeline = data;
      renderSessionTimeline(data);
    } catch (err) {
      elMainContent.innerHTML = `<div class="empty-state">Error cargando timeline: ${escapeHtml(err.message)}</div>`;
    }
  }

  function renderSessionTimeline(data) {
    const obs = data.observations || [];
    const prompts = data.prompts || [];

    const obsEntries = obs.map((o) => `
      <div class="timeline-entry">
        <div class="timeline-dot"></div>
        <div class="timeline-card" onclick="window.openMemory(${o.ID})" style="cursor: pointer;">
          <div class="timeline-meta">
            <span class="badge badge-${escapeHtml(o.Type)}">${escapeHtml(o.Type)}</span>
            <span class="mono">#${o.ID}</span>
            <span>·</span>
            <span>${formatDate(o.CreatedAt)}</span>
          </div>
          <div style="font-weight: 600; color: var(--text-primary); margin-bottom: 4px;">${escapeHtml(o.Title)}</div>
          <div style="font-size: 12.5px; color: var(--text-secondary); line-height: 1.4;">${escapeHtml(o.Content.slice(0, 180))}${o.Content.length > 180 ? "…" : ""}</div>
        </div>
      </div>
    `).join("");

    elMainContent.innerHTML = `
      <div style="margin-bottom: 20px; display: flex; align-items: center; justify-content: space-between;">
        <div>
          <button class="btn btn-secondary" onclick="window.setView('sessions')" style="margin-bottom: 8px;">
            ← Volver a Sesiones
          </button>
          <h2 style="font-size: 18px; font-weight: 700;">Línea de Tiempo: <span class="mono" style="color:var(--teal);">${escapeHtml(data.session_id)}</span></h2>
          <span style="font-size: 12px; color: var(--text-muted);">${obs.length} observaciones generadas</span>
        </div>
      </div>

      <div style="display: grid; grid-template-columns: 2fr 1fr; gap: 24px;">
        <div class="timeline-flow">
          ${obsEntries || '<div class="empty-state">Esta sesión no produjo observaciones</div>'}
        </div>
        <div>
          <div class="section-panel">
            <div class="panel-header">
              <span class="panel-title">Prompts de la Sesión (${prompts.length})</span>
            </div>
            <div style="display: flex; flex-direction: column; gap: 10px; max-height: 500px; overflow-y: auto;">
              ${prompts.map(p => `
                <div style="background: var(--bg-surface); border: 1px solid var(--border); border-radius: var(--radius-sm); padding: 10px 12px; font-size: 12px;">
                  <div style="color: var(--text-muted); font-size: 11px; margin-bottom: 4px;">${formatDate(p.CreatedAt)}</div>
                  <div style="color: var(--text-primary);">${escapeHtml(p.Content)}</div>
                </div>
              `).join("") || '<span class="text-muted" style="font-size: 12px;">Sin prompts registrados</span>'}
            </div>
          </div>
        </div>
      </div>
    `;
  }

  // View: Review Queue
  async function loadReviewQueue() {
    elMainContent.innerHTML = `<div class="loading-state"><div class="spinner"></div><span>Cargando cola de revisión...</span></div>`;
    try {
      const data = await api("/api/review-queue");
      renderReviewQueue(data.queue || []);
    } catch (err) {
      elMainContent.innerHTML = `<div class="empty-state">Error cargando revisión: ${escapeHtml(err.message)}</div>`;
    }
  }

  function renderReviewQueue(queue) {
    const rows = queue.map((o) => `
      <tr>
        <td class="mono" style="font-size: 11px;">#${o.ID}</td>
        <td><span class="badge badge-${escapeHtml(o.Type)}">${escapeHtml(o.Type)}</span></td>
        <td style="font-weight: 500; color: var(--text-primary);" onclick="window.openMemory(${o.ID})">${escapeHtml(o.Title)}</td>
        <td class="mono" style="color: var(--amber); font-size: 12px;">${formatDate(o.ReviewAfter)}</td>
        <td>
          <button class="btn btn-secondary" onclick="window.markReviewed(${o.ID})" style="padding: 4px 10px; font-size: 12px;">
            ✓ Revisada
          </button>
        </td>
      </tr>
    `).join("");

    elMainContent.innerHTML = `
      <div class="data-table-wrapper">
        <div class="table-toolbar">
          <div style="font-size: 13px; font-weight: 600;">Memorias con Revisión Programada (${queue.length})</div>
          <div style="font-size: 12px; color: var(--text-muted);">Verifica la vigencia de memorias y actualiza su estado</div>
        </div>
        <table class="data-table">
          <thead>
            <tr>
              <th style="width: 60px;">ID</th>
              <th style="width: 130px;">Tipo</th>
              <th>Título</th>
              <th style="width: 150px;">Revisar Después De</th>
              <th style="width: 120px;">Acción</th>
            </tr>
          </thead>
          <tbody>
            ${rows || '<tr><td colspan="5" class="empty-state">No hay memorias pendientes de revisión</td></tr>'}
          </tbody>
        </table>
      </div>
    `;
  }

  // View: System & Health
  async function loadSystem() {
    elMainContent.innerHTML = `<div class="loading-state"><div class="spinner"></div><span>Inspeccionando invariantes del sistema...</span></div>`;
    try {
      const data = await api("/api/health");
      const h = data.health || {};
      const emb = data.embeddings || {};

      const shortModel = emb.model ? (emb.model.includes('/') ? emb.model.split('/').pop() : emb.model) : "—";

      elMainContent.innerHTML = `
        <div style="display: grid; grid-template-columns: repeat(auto-fit, minmax(320px, 1fr)); gap: 20px;">
          <div class="section-panel">
            <div class="panel-header">
              <span class="panel-title">Estado de Invariantes de Almacén</span>
            </div>
            <table class="data-table">
              <tbody>
                <tr><td>Total de Observaciones</td><td class="mono">${h.TotalObservations}</td></tr>
                <tr><td>Observaciones Vivas</td><td class="mono" style="color:var(--teal);">${h.LiveObservations}</td></tr>
                <tr><td>Observaciones Borradas (Soft)</td><td class="mono">${h.DeletedCount}</td></tr>
                <tr><td>Sesiones Huérfanas</td><td class="mono ${h.OrphanSessions > 0 ? 'badge-conflict' : ''}">${h.OrphanSessions}</td></tr>
                <tr><td>Sesiones Ambiguas</td><td class="mono ${h.AmbiguousSessions > 0 ? 'badge-conflict' : ''}">${h.AmbiguousSessions}</td></tr>
                <tr><td>Proyectos sin Sesión</td><td class="mono ${h.ProjectsWithoutSession > 0 ? 'badge-conflict' : ''}">${h.ProjectsWithoutSession}</td></tr>
                <tr><td>Duplicados Detectados</td><td class="mono">${h.DuplicateCount}</td></tr>
                <tr><td>Memorias Fijadas</td><td class="mono" style="color:var(--amber);">${h.PinnedCount}</td></tr>
              </tbody>
            </table>
          </div>

          <div class="section-panel">
            <div class="panel-header">
              <span class="panel-title">Vector Store & Embeddings Semánticos</span>
            </div>
            <table class="data-table">
              <tbody>
                <tr><td>Estado del Índice</td><td class="mono" style="color:${emb.available ? 'var(--emerald)' : 'var(--rose)'};">${emb.available ? 'Activo (SQLite)' : 'Inactivo'}</td></tr>
                <tr><td>Total Vectores</td><td class="mono">${emb.total_embedded || 0}</td></tr>
                <tr><td>Cobertura Semántica</td><td class="mono" style="color:var(--emerald);">${emb.coverage_pct ? emb.coverage_pct.toFixed(1) : 0}%</td></tr>
                <tr><td>Vectores Pendientes</td><td class="mono ${emb.pending_count > 0 ? 'badge-pending' : ''}">${emb.pending_count || 0}</td></tr>
                <tr><td>Modelo de Embeddings</td><td class="mono" title="${escapeHtml(emb.model || '')}">${escapeHtml(shortModel)}</td></tr>
                <tr><td>Dimensiones</td><td class="mono">${emb.dimensions ? emb.dimensions + 'd (float32)' : '—'}</td></tr>
                <tr><td>Watcher Automático</td><td class="mono">engram-embed-watch (systemd)</td></tr>
                <tr><td>Última Actualización</td><td class="mono" style="font-size:11px;">${formatDate(emb.latest_at)}</td></tr>
              </tbody>
            </table>
          </div>

          <div class="section-panel">
            <div class="panel-header">
              <span class="panel-title">Almacenamiento SQLite & WAL</span>
            </div>
            <table class="data-table">
              <tbody>
                <tr><td>Tamaño de Base de Datos</td><td class="mono">${formatBytes(data.db_size)}</td></tr>
                <tr><td>Tamaño de WAL (Write-Ahead Log)</td><td class="mono">${formatBytes(data.wal_size)}</td></tr>
                <tr><td>Modo de Conexión Go</td><td class="mono">mode=ro (Direct SQLite)</td></tr>
                <tr><td>API de Mutación Engram</td><td class="mono">${data.api_status}</td></tr>
              </tbody>
            </table>
            <div style="margin-top: 14px; padding: 10px; background: var(--bg-surface); border-radius: var(--radius-sm); font-size: 11.5px; color: var(--text-secondary);">
              <strong>Arquitectura Híbrida:</strong> Todas las consultas analíticas, métricas y búsquedas por trigrama leen directamente del SQLite en modo de solo lectura (sin contención de bloqueo). Todas las modificaciones pasan por el servidor HTTP de Engram para preservar el control de concurrencia y relaciones.
            </div>
          </div>
        </div>
      `;
    } catch (err) {
      elMainContent.innerHTML = `<div class="empty-state">Error cargando salud: ${escapeHtml(err.message)}</div>`;
    }
  }

  // Drawer & Memory Actions
  async function openMemory(id) {
    try {
      const data = await api(`/api/observations/${id}`);
      state.selectedMemory = data.observation;
      state.editingMemory = false;
      renderDrawer(data.observation, data.relations || []);
      elDrawer.classList.remove("hidden");
      elDrawerBackdrop.classList.remove("hidden");
    } catch (err) {
      showToast(`Error abriendo memoria: ${err.message}`, true);
    }
  }

  function closeDrawer() {
    elDrawer.classList.add("hidden");
    elDrawerBackdrop.classList.add("hidden");
    state.selectedMemory = null;
    state.editingMemory = false;
  }

  function renderDrawer(o, rels) {
    elDrawerEyebrow.textContent = `MEMORIA #${o.ID} · ${o.Scope.toUpperCase()}`;
    elDrawerTitle.textContent = o.Title;

    let relsHtml = "";
    if (rels && rels.length > 0) {
      relsHtml = `
        <div style="margin-top: 14px;">
          <span style="font-size: 11px; text-transform: uppercase; color: var(--text-muted); font-weight: 600;">Relaciones Vinculadas (${rels.length})</span>
          <div style="display:flex;flex-direction:column;gap:6px;margin-top:6px;">
            ${rels.map(r => `
              <div style="padding:6px 10px;background:var(--bg-base);border:1px solid var(--border);border-radius:var(--radius-sm);font-size:12px;display:flex;align-items:center;justify-content:space-between;">
                <div style="display:flex;align-items:center;gap:6px;">
                  <span class="badge ${r.relation === 'conflicts_with' ? 'badge-conflict' : 'badge-default'}">${escapeHtml(r.relation)}</span>
                  <span>#${r.target_id === String(o.ID) ? r.source_id : r.target_id}</span>
                </div>
                <span class="mono" style="font-size:11px;color:var(--text-muted);">${escapeHtml(r.judgment_status)}</span>
              </div>
            `).join("")}
          </div>
        </div>
      `;
    }

    const vectorLabel = o.HasVector
      ? `<span style="color:var(--emerald);">✓ ${o.VectorDims}d (${o.VectorModel ? (o.VectorModel.split('/').pop() || o.VectorModel) : ''})</span>`
      : `<span style="color:var(--text-muted);">Sin vector</span>`;

    elDrawerBody.innerHTML = `
      <div class="drawer-meta-grid">
        <div class="meta-field">
          <span class="meta-field-label">Tipo</span>
          <span class="meta-field-value"><span class="badge badge-${escapeHtml(o.Type)}">${escapeHtml(o.Type)}</span></span>
        </div>
        <div class="meta-field">
          <span class="meta-field-label">Proyecto</span>
          <span class="meta-field-value">${escapeHtml(o.Project || "—")}</span>
        </div>
        <div class="meta-field">
          <span class="meta-field-label">Vector Semántico</span>
          <span class="meta-field-value mono" style="font-size:11.5px;">${vectorLabel}</span>
        </div>
        <div class="meta-field">
          <span class="meta-field-label">Topic Key</span>
          <span class="meta-field-value mono">${escapeHtml(o.TopicKey || "—")}</span>
        </div>
      </div>

      <div class="drawer-content-box">${escapeHtml(o.Content)}</div>

      ${relsHtml}
    `;

    const pinLabel = o.Pinned ? "★ Desfijar" : "☆ Fijar";
    elDrawerFooter.innerHTML = `
      <div style="display: flex; gap: 8px;">
        <button class="btn btn-secondary" onclick="window.copyMemory(${o.ID})">Copiar ID</button>
        <button class="btn btn-secondary" onclick="window.togglePin(${o.ID}, ${o.Pinned})">${pinLabel}</button>
      </div>
      <div style="display: flex; gap: 8px;">
        <button class="btn btn-secondary" onclick="window.startEditMemory()">Editar</button>
        <button class="btn btn-danger" onclick="window.deleteMemory(${o.ID}, '${escapeHtml(o.Project)}')">Borrar</button>
      </div>
    `;
  }

  function startEditMemory() {
    const o = state.selectedMemory;
    if (!o) return;
    state.editingMemory = true;

    elDrawerBody.innerHTML = `
      <form class="edit-form" onsubmit="window.saveMemory(event)">
        <div class="form-group">
          <label class="form-label">Título</label>
          <input type="text" id="edit-title" class="form-input" value="${escapeHtml(o.Title)}" required />
        </div>
        <div class="form-group">
          <label class="form-label">Tipo de Memoria</label>
          <input type="text" id="edit-type" class="form-input" value="${escapeHtml(o.Type)}" required />
        </div>
        <div class="form-group">
          <label class="form-label">Contenido (Markdown)</label>
          <textarea id="edit-content" class="form-textarea" required>${escapeHtml(o.Content)}</textarea>
        </div>
      </form>
    `;

    elDrawerFooter.innerHTML = `
      <button class="btn btn-secondary" onclick="window.cancelEditMemory()">Cancelar</button>
      <button class="btn btn-primary" onclick="window.saveMemory(event)">Guardar Cambios</button>
    `;
  }

  async function saveMemory(e) {
    if (e) e.preventDefault();
    const o = state.selectedMemory;
    if (!o) return;

    const title = $("#edit-title").value.trim();
    const type = $("#edit-type").value.trim();
    const content = $("#edit-content").value.trim();

    try {
      await api(`/api/observations/${o.ID}`, {
        method: "PATCH",
        body: {
          expected_project: o.Project,
          title,
          type,
          content,
        },
      });
      showToast("Memoria actualizada correctamente");
      openMemory(o.ID);
      if (state.currentView === "memories") loadMemories();
    } catch (err) {
      showToast(`Error al guardar: ${err.message}`, true);
    }
  }

  async function togglePin(id, currentPinned) {
    try {
      await api(`/api/observations/${id}/pin`, {
        method: currentPinned ? "DELETE" : "PUT",
      });
      showToast(currentPinned ? "Memoria desfijada" : "Memoria fijada");
      openMemory(id);
      if (state.currentView === "memories") loadMemories();
    } catch (err) {
      showToast(`Error fijando memoria: ${err.message}`, true);
    }
  }

  async function deleteMemory(id, project) {
    if (!confirm(`¿Deseas enviar la memoria #${id} a la papelera (soft-delete)?`)) {
      return;
    }
    try {
      await api(`/api/observations/${id}?expected_project=${encodeURIComponent(project)}`, {
        method: "DELETE",
      });
      showToast(`Memoria #${id} eliminada`);
      closeDrawer();
      if (state.currentView === "memories") loadMemories();
    } catch (err) {
      showToast(`Error al eliminar: ${err.message}`, true);
    }
  }

  async function markReviewed(id) {
    try {
      await api(`/api/observations/${id}/review`, { method: "POST" });
      showToast(`Memoria #${id} marcada como revisada`);
      if (state.currentView === "review") loadReviewQueue();
    } catch (err) {
      showToast(`Error al revisar: ${err.message}`, true);
    }
  }

  function copyMemory(id) {
    navigator.clipboard.writeText(String(id));
    showToast(`ID #${id} copiado al portapapeles`);
  }

  // Global Attachments
  window.setView = setView;
  window.openMemory = openMemory;
  window.openSessionTimeline = openSessionTimeline;
  window.copyMemory = copyMemory;
  window.togglePin = togglePin;
  window.deleteMemory = deleteMemory;
  window.startEditMemory = startEditMemory;
  window.cancelEditMemory = () => openMemory(state.selectedMemory.ID);
  window.saveMemory = saveMemory;
  window.markReviewed = markReviewed;

  window.onFilterChange = (key, val) => {
    state.memoriesFilter[key] = val;
    state.memoriesFilter.offset = 0;
    loadMemories();
  };

  window.filterByProject = (proj) => {
    state.memoriesFilter.project = proj;
    setView("memories");
  };

  // Event Listeners
  $$(".nav-item").forEach((btn) => {
    btn.addEventListener("click", () => setView(btn.dataset.view));
  });

  elDrawerClose.addEventListener("click", closeDrawer);
  elDrawerBackdrop.addEventListener("click", closeDrawer);

  elBtnRefresh.addEventListener("click", () => {
    checkEngineStatus();
    setView(state.currentView);
    showToast("Datos actualizados");
  });

  elGlobalSearchInput.addEventListener("keydown", (e) => {
    if (e.key === "Enter") {
      state.memoriesFilter.q = elGlobalSearchInput.value.trim();
      setView("memories");
    }
  });

  window.addEventListener("keydown", (e) => {
    if (e.key === "Escape") {
      closeDrawer();
    } else if (e.key === "/" && document.activeElement !== elGlobalSearchInput) {
      e.preventDefault();
      elGlobalSearchInput.focus();
    }
  });

  // Initialization
  checkEngineStatus();
  setView("overview");
  setInterval(checkEngineStatus, 15000);
})();
