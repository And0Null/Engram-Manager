# Engram Manager

TUI, CLI, and Web Dashboard manager for [Engram](https://github.com/Gentle-AI/engram) memories, active sessions, Action Radar backlog, and store health.

Designed with a dual-pane Lazygit-inspired layout in the terminal and a modern Zero-Build Web Dashboard in the browser, styled with the Pi Codec palette (cyan, magenta, and dark teal accents).

---

## Architecture: Hybrid Model

Engram Manager uses a decoupled architecture designed for high read performance, atomic writes, and isolated task management:

- **Reads (Direct SQLite mode=ro)**:
  Direct access to `~/.engram/engram.db` via `modernc.org/sqlite` in read-only mode (`busy_timeout(5000)` and `query_only(1)`). Instant search, relations graphs, and metric aggregations without server locks or process contention.
- **Writes (HTTP API)**:
  Memory mutations (edits, soft deletes, hard deletes, pins, scheduled reviews) route through `engram serve` (`127.0.0.1:7437`). This maintains Engram's optimistic concurrency (`expected_project`), conflict relations, and revision tracking.
- **Action Radar Backlog (Dedicated SQLite)**:
  Tracked tasks and session follow-up actions live in an isolated SQLite database at `~/.config/engram-manager/radar.db` (WAL mode enabled). Tasks remain decoupled from `engram.db` to avoid mixing episodic AI memories with transactional backlog state.
- **Live SSE Streaming**:
  A lightweight event broker watches SQLite WAL changes with a 300 ms debounce, streaming real-time mutation events across `/api/stream` to keep web clients synchronized without polling.

---

## Directory Structure

```
Engram-Manager/
├── cmd/
│   ├── engram-manager/     # Main command orchestrator (TUI, status, web)
│   └── engram-tui/         # Standalone TUI binary entrypoint
├── internal/
│   ├── api/                # HTTP client for Engram write mutations
│   ├── cli/                # Fast stdout diagnostics (status)
│   ├── radar/              # Action Radar SQLite store, task CRUD, and notes
│   ├── store/              # Read-only SQLite queries, FTS5 index, relations, and session dossiers
│   ├── system/             # Git portfolio tracker and agent process inspector
│   ├── tui/                # Bubbletea terminal interface (dual-pane lazygit layout)
│   └── web/                # Zero-build embedded web server and dashboard SPA
│       └── static/         # HTML, CSS, and vanilla JS (embedded via go:embed)
├── PRODUCT.md              # Product specifications and UI design constraints
├── go.mod
└── README.md
```

---

## Requirements

- **Go**: 1.22 or newer (uses standard library routing patterns).
- **Engram**: Active Engram database at `~/.engram/engram.db`.
- **Operating System**: Linux or macOS.

---

## Installation & Build

Build both binaries:

```bash
go build -o bin/engram-manager ./cmd/engram-manager
go build -o bin/engram-tui ./cmd/engram-tui
```

Link binaries to your user path:

```bash
ln -sf $(pwd)/bin/engram-manager ~/.local/bin/engram-manager
ln -sf $(pwd)/bin/engram-tui ~/.local/bin/engram-tui
```

---

## Usage

### 1. Interactive TUI
Launch the interactive terminal interface:

```bash
engram-manager
# or explicitly:
engram-manager tui
# or directly via standalone binary:
engram-tui
```

- **Dual-Pane Split (columns >= 90)**:
  - **Left Pane**: Memory list with badges, project context, and type indicators.
  - **Right Pane**: Instant preview of the selected memory content.
- **Keybindings**:
  - `↑/k`, `↓/j`: Navigate memories and sessions.
  - `enter`: Expand memory into full detail view (history, relations, session timeline).
  - `/`: Search memories via FTS5 trigram index (`esc` to clear).
  - `m`: Health & Metrics dashboard (database size, WAL size, top projects).
  - `s`: Active sessions and drift panel.
  - `P`: Toggle soft-deleted memory visibility.
  - `e`: Edit memory title and content (detail view).
  - `d` / `D`: Soft delete (recoverable) / Permanent hard delete (detail view).
  - `p`: Toggle pin state.
  - `r`: Mark memory as reviewed.
  - `esc`: Back / reset search filters.
  - `q`: Quit.

### 2. Web Dashboard (Zero-Build Embedded SPA)
Launch the local web dashboard:

```bash
engram-manager web
# or on a custom port:
engram-manager web -port 7438
# or via environment variable:
ENGRAM_DASH_PORT=7438 engram-manager web
```

Open `http://127.0.0.1:7438` in your browser.

- **Embedded & Zero-Dependencies**: Served directly from the Go binary via `go:embed`. No Node.js runtime, build step, or external CDN dependencies.
- **Live SSE Event Broker**: Real-time push notifications over `/api/stream` update the dashboard instantly when memories change or tasks are updated.

#### Key Dashboard Views

- **Overview**: Polar radar of memory type mix, windowed readouts (24h, 7d, 30d), 30-day activity strip with average/peak counters, vector coverage, and database health metrics.
- **Action Radar**: Integrated Kanban board (`Pending`, `In Progress`, `Completed`, `Deferred`) with priority flags (`HIGH`, `MEDIUM`, `LOW`), categories (`FEATURE`, `BUGFIX`, `REFACTOR`, `IDEA`), and task notes. Includes quick filters by project and status.
- **Memories**: Instant FTS trigram search, filtering by project, type, pinned, or deleted state, with a sliding detail drawer for markdown rendering, editing, and deletion.
- **Relations**: Visual relations browser and judgement queue with confidence ratings and cross-link navigation.
- **Sessions & Dossiers**: Timeline of agent runs with automatic semantic extraction of structured session dossiers (Goal, Accomplished, Decisions, Discoveries, and Next Steps). Includes one-click `+ A Radar` button to convert session next steps into tracked backlog tasks.
- **Review Queue**: Scheduled review inbox with one-click review verification.
- **System & Agent Inspector**: Real-time inspection of running coding agent processes (`pi`, `gentle-pi`, `herdr`), Herdr UNIX socket connection status, Git portfolio branch status, and SQLite store invariants.

### 3. Fast CLI Status (Headless)
Inspect database health and top projects in stdout without opening the UI:

```bash
engram-manager status
```

Example output:

```text
Engram Health & Status
Database: /home/and00pium/.engram/engram.db (23.9 MiB, WAL: 22.1 MiB)
Memories: 961 live (961 total)
Sessions: 292 (12 active)
Prompts:  944 across 64 projects

Health: OK (all invariants hold, 0 operational drifts)

Top Projects:
  and0null                  542 obs  (85 ses)
  and0-pi                   105 obs  (29 ses)
  ambxst-mods                61 obs  (34 ses)
```

---

## Limitations & Operational Notes

- **Read-Only Lock Safety**: Reading `engram.db` directly relies on SQLite WAL mode. Do not delete WAL files manually while processes are running.
- **Mutation Port**: Write operations require `engram serve` running on `127.0.0.1:7437`. If the server is offline, mutations in the UI will fail with a connection error while read queries continue working.
- **Process Discovery**: Terminal and agent process inspection (`pgrep`) is designed for local developer environments on Linux/macOS.

---

## License

MIT License. See project repository for details.
