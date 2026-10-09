# Engram Manager

TUI, CLI, and Web Dashboard manager for [Engram](https://github.com/Gentle-AI/engram) memories, active sessions, and store health.

Designed with a dual-pane **Lazygit-inspired layout** in the terminal and a modern **Zero-Build Web Dashboard** in the browser, styled with the **Pi Codec theme palette** (cyan, magenta, and dark teal accents).

---

## Architecture: Hybrid Model

- **Reads (Direct SQLite mode=ro)**:
  Direct access to `~/.engram/engram.db` via `modernc.org/sqlite` in read-only mode (`busy_timeout(5000)` and `query_only(1)`). Instant search, relations graphs, and metric aggregations without process lock contention or server requirements.
- **Writes (HTTP API)**:
  Mutations (edits, soft-deletes, hard-deletes, pins, reviews) go through `engram serve` (`127.0.0.1:7437`). This preserves Engram's own optimistic concurrency rules (`expected_project`), conflict relations, and revision tracking.

---

## Directory Structure

```
Engram-Manager/
├── cmd/
│   ├── engram-manager/     # Main command & orchestrator (TUI, status, web)
│   └── engram-tui/         # Standalone TUI entrypoint
├── internal/
│   ├── api/                # HTTP API client for write operations
│   ├── cli/                # Non-interactive stdout commands (status)
│   ├── store/              # Read-only SQLite queries, FTS5, relations & metrics
│   ├── tui/                # Bubbletea terminal interface (2-pane lazygit layout)
│   └── web/                # Zero-build embedded web server & dashboard SPA
│       └── static/         # HTML, CSS & vanilla JS (embedded via go:embed)
├── bin/                    # Compiled binaries
├── go.mod
└── README.md
```

---

## Installation & Build

Build both binaries:

```bash
go build -o bin/engram-manager ./cmd/engram-manager
go build -o bin/engram-tui ./cmd/engram-tui
```

Link them to your local PATH:

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

- **Dual-Pane Split (columns ≥ 90)**:
  - **Left Pane**: Memory list with badges, project context, and types.
  - **Right Pane**: Live instant preview of the selected memory (no need to press Enter to read).
- **Keybindings**:
  - `↑/k`, `↓/j`: Navigate memories / sessions
  - `enter`: Expand memory into full detail view (history, relations, session timeline)
  - `/`: Search via FTS5 trigram index (`esc` to clear)
  - `m`: Health & Metrics dashboard (DB/WAL size, drift warnings, top projects)
  - `s`: Active sessions & drift panel
  - `P`: Toggle soft-deleted memory visibility
  - `e`: Edit memory title & content (in detail view)
  - `d` / `D`: Soft-delete (recoverable) / Permanent hard-delete (in detail view)
  - `p`: Toggle pin / unpin
  - `r`: Mark as reviewed
  - `esc`: Back / reset filters
  - `q`: Quit

### 2. Web Dashboard (Zero-Build Embedded SPA)
Launch the local web dashboard:

```bash
engram-manager web
# or custom port / address:
engram-manager web -port 7438
# or via environment variable:
ENGRAM_DASH_PORT=7438 engram-manager web
```

Open `http://127.0.0.1:7438` in your browser.

- **Embedded & Zero-Dependencies**: Served directly from the Go binary via `go:embed`. No Python, no Node.js, no build step.
- **Key Views**:
  - **Overview**: Memory totals, 24h / 7d / 30d activity windows, memory type distribution bars, top projects.
  - **Memories**: Instant FTS trigram search, filter by project / type / pinned / deleted, and a sliding detail drawer (`#/memories/obs/<id>`) with Markdown view, copy ID, edit, and soft-delete.
  - **Relaciones & Conflictos**: Knowledge graph relationship explorer (`conflicts_with`, `supersedes`, `related`, `compatible`), confidence scores, and origin/target links.
  - **Sesiones & Timeline**: Chronological observation timeline and prompt flow per agent session.
  - **Revisión**: Queue of memories scheduled for verification (`review_after`).
  - **Sistema & Salud**: DB/WAL storage gauges, store invariant checks, and connection heartbeat with `engram serve`.

### 3. Fast CLI Status (Headless)
Inspect database health and top projects in stdout without opening the interactive TUI:

```bash
engram-manager status
```

Example output:
```
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
