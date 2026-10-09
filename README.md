# Engram Manager

TUI and CLI manager for [Engram](https://github.com/Gentle-AI/engram) memories, active sessions, and store health.

Designed with a dual-pane **Lazygit-inspired layout** and the **Pi Codec theme palette** (cyan, magenta, and dark teal accents).

---

## Architecture: Hybrid Model

- **Reads (Direct SQLite mode=ro)**:
  Direct access to `~/.engram/engram.db` via `modernc.org/sqlite` in read-only mode (`busy_timeout(5000)` and `query_only(1)`). Instant search and metric aggregations without process lock contention or server requirements.
- **Writes (HTTP API)**:
  Mutations (edits, soft-deletes, hard-deletes, pins, reviews) go through `engram serve` (`127.0.0.1:7437`). This preserves Engram's own optimistic concurrency rules (`expected_project`), conflict relations, and revision tracking.

---

## Directory Structure

```
Engram-Manager/
├── cmd/
│   ├── engram-manager/     # Main command & orchestrator
│   └── engram-tui/         # Standalone TUI entrypoint
├── internal/
│   ├── api/                # HTTP API client for write operations
│   ├── cli/                # Non-interactive stdout commands (status)
│   ├── store/              # Read-only SQLite queries, FTS5 & metrics
│   └── tui/                # Bubbletea terminal interface (2-pane lazygit layout)
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

### 2. Fast CLI Status (Headless)
Inspect database health and top projects in stdout without opening the interactive TUI:

```bash
engram-manager status
```

Example output:
```
Engram Health & Status
Database: /home/and00pium/.engram/engram.db (23.9 MiB, WAL: 22.1 MiB)
Memories: 948 live (948 total)
Sessions: 285 (5 active)
Prompts:  918 across 64 projects

Health: OK (all invariants hold, 0 operational drifts)

Top Projects:
  and0null                  540 obs  (85 ses)
  and0-pi                   103 obs  (28 ses)
  ambxst-mods                61 obs  (34 ses)
```
