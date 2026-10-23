# Feature: Action Radar and Live Cockpit Integration

<!-- nodd:slug action-radar-live-cockpit -->

## Objective

Implement Action Radar and Live Cockpit integration following the approved architecture plan.

## Problem

Engram-Manager lacks task backlog management (Action Radar), structured session analysis, live real-time updates via SSE, and terminal/agent monitoring.

## Scope

Implement Phase 1 through Phase 4 from docs/PLAN-ACTION-RADAR-AND-LIVE-COCKPIT.md while strictly preserving engram.db read-only and zero-build web invariants.

## Constraints



## Route

- intent: change
- route: tracked

## Verification

- runner: go test ./...
- tdd: off
- source: nodd_declare
- files: internal/radar/store.go, internal/radar/model.go, internal/radar/store_test.go, internal/web/server.go, internal/web/server_test.go, cmd/engram-manager/main.go, internal/store/summary_parser.go, internal/store/summary_parser_test.go, internal/store/sessions.go, internal/web/sse.go, internal/system/git.go, internal/system/terminals.go, internal/web/static/app.js, internal/web/static/index.html, internal/web/static/style.css

## Delivery

- strategy: ask-on-risk
- chain: stacked-to-main
- forecast: 0
- running: unknown

## Tasks

- [x] T1. Create internal/radar SQLite store for radar.db
  - observed: `none` → none (gate disabled)
  - candidate: pending-commit
- [x] T2. Implement Radar CRUD and filters
  - observed: `none` → none (gate disabled)
  - candidate: pending-commit
- [x] T3. Add store tests in internal/radar/store_test.go
  - observed: `none` → none (gate disabled)
  - candidate: pending-commit
- [x] T4. Mount Radar REST endpoints and tests in internal/web
  - observed: `none` → none (gate disabled)
  - candidate: pending-commit
- [x] T5. Semantic Session Summary Parser and Expediente API
  - observed: `none` → none (gate disabled)
  - candidate: pending-commit
- [x] T6. Live SSE Watcher and Radar/Expediente Web Views
  - observed: `none` → none (gate disabled)
  - candidate: pending-commit
- [x] T7. Terminal and Git health monitoring
  - observed: `none` → none (gate disabled)
  - candidate: pending-commit

## Outcome

- verified: 7 of 7 task(s)
- [x] T1: `none` → none (gate disabled)
- [x] T2: `none` → none (gate disabled)
- [x] T3: `none` → none (gate disabled)
- [x] T4: `none` → none (gate disabled)
- [x] T5: `none` → none (gate disabled)
- [x] T6: `none` → none (gate disabled)
- [x] T7: `none` → none (gate disabled)

## Progress


