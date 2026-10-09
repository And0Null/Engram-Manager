# Feature: Web Dashboard y ampliación de Store con Relaciones

<!-- nodd:slug web-dashboard-and-store-relations -->

## Objective

Añadir soporte de relaciones y actividad al store en Go y construir el Web Dashboard embebido con diseño impecable para engram-manager web.

## Problem

Engram-Manager lacks relation graph queries, timeline tracking, and a web dashboard counterpart to the TUI.

## Scope

Store queries for relations/timelines/activity, embedded Go HTTP web server, zero-build web dashboard UI, and web CLI command.

## Constraints

No external npm dependencies; zero-build vanilla frontend embedded in Go binary; no breaking changes to existing TUI or CLI.

## Route

- intent: change
- route: tracked

## Verification

- runner: go test ./...
- tdd: off
- source: nodd_declare
- files: internal/store/model.go, internal/store/query.go, internal/store/relations.go, internal/store/relations_test.go, internal/web/server.go, internal/web/server_test.go, internal/web/static/index.html, internal/web/static/app.css, internal/web/static/app.js, cmd/engram-manager/main.go, README.md

## Delivery

- strategy: ask-on-risk
- chain: stacked-to-main
- forecast: 0
- running: unknown

## Tasks

- [x] T1. Ampliación del store con relaciones, timelines y actividad
  - observed: `none` → none (gate disabled)
  - candidate: pending-commit
- [x] T2. Tests unitarios para relaciones y timelines en store
  - observed: `none` → none (gate disabled)
  - candidate: pending-commit
- [x] T3. Backend web en Go con endpoints API y go:embed
  - observed: `none` → none (gate disabled)
  - candidate: pending-commit
- [x] T4. Frontend Web Dashboard con diseño impecable (Pi Codec)
  - observed: `none` → none (gate disabled)
  - candidate: pending-commit
- [x] T5. Integración del comando engram-manager web en main.go
  - observed: `none` → none (gate disabled)
  - candidate: pending-commit
- [x] T6. Validación completa y documentación
  - observed: `none` → none (gate disabled)
  - candidate: pending-commit
- [x] T7. Consultas y modelo de EmbeddingStats en store
  - observed: `none` → none (gate disabled)
  - candidate: pending-commit
- [x] T8. Integración de embeddings en CLI status y modal TUI
  - observed: `none` → none (gate disabled)
  - candidate: pending-commit
- [x] T9. Integración de embeddings en Web Dashboard
  - observed: `none` → none (gate disabled)
  - candidate: pending-commit
- [x] T10. Validación completa de embeddings
  - observed: `none` → none (gate disabled)
  - candidate: pending-commit

## Outcome

- verified: 10 of 10 task(s)
- [x] T1: `none` → none (gate disabled)
- [x] T2: `none` → none (gate disabled)
- [x] T3: `none` → none (gate disabled)
- [x] T4: `none` → none (gate disabled)
- [x] T5: `none` → none (gate disabled)
- [x] T6: `none` → none (gate disabled)
- [x] T7: `none` → none (gate disabled)
- [x] T8: `none` → none (gate disabled)
- [x] T9: `none` → none (gate disabled)
- [x] T10: `none` → none (gate disabled)

## Progress


