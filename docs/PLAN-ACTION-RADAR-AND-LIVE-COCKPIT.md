# Architecture Plan: Action Radar & Live Cockpit Integration for Engram-Manager

> **Documento de Planificación y Arquitectura de Integración**  
> **Fecha:** 2026-10-23  
> **Origen:** Análisis y auditoría de patrones arquitectónicos de `WiloxDev/center-control-manager`.  
> **Repositorio Destino:** `Engram-Manager` (Go / modernc.org/sqlite / Zero-Build Web / Bubbletea TUI).

---

## 1. Visión y Requisito General

Extender **Engram-Manager** para transformar la experiencia de inspección de memorias en un centro de mando operativo del desarrollador, incorporando 5 capacidades clave sin violar sus principios arquitectónicos:
1. **Action Radar:** Gestor de tareas ("Para Después" / Backlog) desacoplado de Engram pero fuertemente vinculado a las sesiones de IA mediante `originSessionId`.
2. **Parser Semántico de Resúmenes de Sesión ("Expediente"):** Desglose estructurado de `session_summary` en Goal, Key Decisions, Discoveries, Accomplished y Next Steps, con botón para convertir Next Steps en tareas del Radar.
3. **Live SSE Watcher (`/api/stream`):** Actualización en tiempo real del dashboard web observando el archivo WAL de Engram (`~/.engram/engram.db-wal`).
4. **Monitoreo de Terminales y Agentes en Vivo:** Detección de sockets Herdr (`~/.config/herdr/herdr.sock`) y procesos activos de `gentle-pi` / `pi` con métricas de RAM/PID y estado (working, idle, blocked).
5. **Salud del Portafolio Git:** Estado de ramas, working tree (limpio/sucio) y último commit relativo por proyecto.

---

## 2. Invariantes Arquitectónicos Innegociables

- **Binario único compilado en Go:** Cero dependencias de Node.js, Python o runtime externos.
- **Zero-Build Frontend:** Activos web embebidos en el binario vía `go:embed` en `internal/web/static/`. Cero NPM, cero bundlers, cero CDNs remotas, 100% offline.
- **Aislamiento e Inviolabilidad de `engram.db`:**
  - Lecturas sobre `engram.db` siguen siendo estrictamente `mode=ro` (`busy_timeout(5000)` y `query_only(1)`).
  - Mutaciones de memoria continúan pasando por la API HTTP oficial (`127.0.0.1:7437`).
  - **Las tareas del Action Radar se almacenan en un SQLite propio:** `~/.config/engram-manager/radar.db`. Jamás se agregan tablas o campos dentro de `engram.db`.
- **Seguridad y Loopback:** El servidor web se enlaza exclusivamente en `127.0.0.1` (loopback). Sin interpolación de cadenas shell (`exec.Command` directo con paso de argumentos y límites de timeout estrictos).

---

## 3. Decisiones Arquitectónicas y Modos de Fallo

### Decisión 1: Almacén de Datos del Action Radar
- **Enfoque adoptado:** `modernc.org/sqlite` nativo gestionando `~/.config/engram-manager/radar.db`.
- **Alternativa rechazada:** Guardar tareas como observaciones de tipo `task` dentro de `engram.db`.
- **Razón del rechazo:** Contamina el espacio de recuerdos vectoriales y episódicos de la IA con tareas transaccionales que cambian de estado continuamente (PENDING -> IN_PROGRESS -> COMPLETED).
- **Esquema:**
  ```sql
  CREATE TABLE IF NOT EXISTS radar_tasks (
    id TEXT PRIMARY KEY,
    project TEXT NOT NULL,
    title TEXT NOT NULL,
    description TEXT NOT NULL DEFAULT '',
    category TEXT NOT NULL DEFAULT 'FEATURE', -- FEATURE, BUGFIX, REFACTOR, IDEA
    priority TEXT NOT NULL DEFAULT 'MEDIUM',  -- HIGH, MEDIUM, LOW
    status TEXT NOT NULL DEFAULT 'PENDING',    -- PENDING, IN_PROGRESS, COMPLETED, DEFERRED
    due_date TEXT,
    origin_session_id TEXT,
    created_at TEXT NOT NULL,
    updated_at TEXT NOT NULL,
    completed_at TEXT
  );

  CREATE TABLE IF NOT EXISTS radar_task_notes (
    id TEXT PRIMARY KEY,
    task_id TEXT NOT NULL,
    author TEXT NOT NULL DEFAULT 'user',
    content TEXT NOT NULL,
    created_at TEXT NOT NULL,
    FOREIGN KEY (task_id) REFERENCES radar_tasks(id) ON DELETE CASCADE
  );

  CREATE INDEX IF NOT EXISTS idx_radar_project ON radar_tasks(project);
  CREATE INDEX IF NOT EXISTS idx_radar_status ON radar_tasks(status);
  CREATE INDEX IF NOT EXISTS idx_radar_origin_session ON radar_tasks(origin_session_id);
  ```

### Decisión 2: Parser Semántico de Sesiones
- **Enfoque adoptado:** Scanner determinista línea por línea en `internal/store/summary_parser.go`.
- **Defensa ante fallo:** Si un resumen no sigue el formato markdown canónico, se degrada elegantemente devolviendo el texto crudo en `Goal` o `Instructions`, sin provocar fallos de serialización ni errores HTTP 500.

### Decisión 3: Live SSE Watcher
- **Enfoque adoptado:** Endpoint HTTP `GET /api/stream` utilizando `http.Flusher`. Watcher de sistema de archivos sobre el directorio `~/.engram/` filtrando por eventos sobre `engram.db-wal` con debounce de 300 ms.
- **Defensa ante fallo:** Prevención de fugas de memoria con `select { case <-r.Context().Done(): ... }` para desconectar y limpiar los canales cuando el navegador cierra o recarga la pestaña.

### Decisión 4: Monitoreo Seguro de Herdr y Git
- **Enfoque adoptado:**
  - Git: `exec.CommandContext(ctx, "git", "rev-parse", "--abbrev-ref", "HEAD")` con timeout de 500 ms.
  - Herdr: Chequeo de conexión directa al socket UNIX (`net.DialTimeout("unix", socketPath, 500*time.Millisecond)`) y ejecución de `herdr agent list` con deserialización JSON controlada.
- **Defensa ante fallo:** Si Herdr o Git no están instalados o fallan, se reportan estados nulos (`exists: false`) sin interrumpir las demás vistas.

---

## 4. Fases de Implementación Ordenadas

### Fase 1: Action Radar Core (Store `radar.db` y REST API)
- [ ] 1.1 Crear paquete `internal/radar/` con conexión SQLite aislada en `~/.config/engram-manager/radar.db`.
- [ ] 1.2 Implementar métodos CRUD: `CreateTask`, `GetTasks(filters)`, `GetTaskByID(id)`, `UpdateTask(id, updates)`, `AddNote(taskID, note)`.
- [ ] 1.3 Registrar endpoints en `internal/web/server.go`:
  - `GET /api/radar/tasks` (filtros: `project`, `status`, `priority`, `originSessionId`).
  - `POST /api/radar/tasks`
  - `PATCH /api/radar/tasks/{id}`
  - `POST /api/radar/tasks/{id}/notes`
- [ ] 1.4 Tests unitarios en `internal/radar/store_test.go`.

### Fase 2: Parser Semántico de Resúmenes de Sesión ("Expediente")
- [ ] 2.1 Crear `internal/store/summary_parser.go` con `ParseSessionSummary(raw string) ParsedSummary`.
- [ ] 2.2 Enriquecer `internal/store/sessions.go` para adjuntar `ParsedSummary` al recuperar el detalle de sesión.
- [ ] 2.3 Actualizar el endpoint `GET /api/sessions/{id}/timeline` para incluir el resumen estructurado y la lista de tareas relacionadas (`related_tasks`) que compartan `origin_session_id`.
- [ ] 2.4 Tests unitarios de parsing con fixtures de markdown reales de Engram.

### Fase 3: Live SSE Watcher y Vistas Web (Frontend)
- [ ] 3.1 Implementar `internal/web/sse.go` con `Broker` de clientes SSE y debounce de eventos WAL.
- [ ] 3.2 Añadir endpoint `GET /api/stream` en el servidor web.
- [ ] 3.3 Diseñar componentes visuales en `internal/web/static/`:
  - **Pestaña Action Radar:** Tablero Kanban / lista con filtros rápidos de prioridad y proyecto.
  - **Expediente de Sesión:** Renderizado en acordeones limpios de Goal, Key Decisions y Next Steps.
  - **Botón de acción rápida:** "Convertir Next Step en Tarea del Radar" (pre-llena título y `originSessionId`).
- [ ] 3.4 Conectar `EventSource` en `app.js` para recargar la vista activa cuando se reciba evento `engram-mutation`.

### Fase 4: Monitoreo de Terminales/Herdr y Salud Git
- [ ] 4.1 Crear `internal/system/git.go` para inspeccionar rama y limpieza de carpetas de proyecto.
- [ ] 4.2 Crear `internal/system/terminals.go` para consultar Herdr y procesos locales `gentle-pi`.
- [ ] 4.3 Exponer datos en `GET /api/projects` y nuevo endpoint `GET /api/system/terminals`.
- [ ] 4.4 Mostrar indicador de agentes activos y estado Git en el header y panel general de métricas.

---

## 5. Criterios de Aceptación y Validación

- `go test ./...` pasa al 100% en todas las suites.
- La compilación `go build -o bin/engram-manager ./cmd/engram-manager` genera el binario sin advertencias ni librerías dinámicas requeridas.
- La base `engram.db` permanece en modo solo lectura inalterada; todas las tareas creadas se persisten en `~/.config/engram-manager/radar.db`.
- La interfaz web funciona fluidamente sin conexión a internet y sin recargas de página manuales.
