# Feature: Overview con radar y series temporales reales

<!-- nodd:slug overview-constellation-and-timeseries -->

## Objective

Rediseñar el Overview del web dashboard con un radar radial de distribución por tipo, barras de tipo con porcentaje y series temporales reales (actividad diaria, cobertura de vectorización), y extender ese lenguaje visual al resto de vistas.

## Problem

El Overview actual son cuatro tarjetas de texto y dos paneles planos: no comunica forma, tendencia ni volumen. La referencia resuelve esa lectura con un radar radial, barras tipadas con porcentaje, y una tira de actividad diaria. Además el store no expone series temporales, así que la composición no podría ser real.

## Scope

Overview rediseñado con radar radial, barras tipadas, tira de actividad diaria y fila de métricas; el mismo lenguaje visual se extiende a Memories, Relations, Sessions, Review y System.

## Constraints

No npm/bundler/CDN; no third-party chart library; real data only from the store, never synthetic; go:embed must keep the binary self-contained; one spacing rhythm; color never the sole channel for meaning.

## Route

- intent: change
- route: tracked

## Verification

- runner: go test ./...
- tdd: off
- source: nodd_declare
- files: internal/store/model.go, internal/store/stats.go, internal/store/relations_test.go, internal/web/server.go, internal/web/static/index.html, internal/web/static/app.css, internal/web/static/app.js, PRODUCT.md

## Delivery

- strategy: ask-on-risk
- chain: stacked-to-main
- forecast: 0
- running: unknown

## Tasks

- [x] T1. Series temporales reales en el store (actividad diaria, conteo por tipo en ventana)
  - observed: `none` → none (gate disabled)
  - candidate: pending-commit
- [x] T2. Tests del store para series temporales
  - observed: `none` → none (gate disabled)
  - candidate: pending-commit
- [x] T3. Exponer series y radar en la API web
  - observed: `none` → none (gate disabled)
  - candidate: pending-commit
- [x] T4. Rediseñar Overview: radar radial, barras tipadas, tira diaria, fila de métricas
  - observed: `none` → none (gate disabled)
  - candidate: pending-commit
- [x] T5. Extender el lenguaje visual al resto de vistas
  - observed: `none` → none (gate disabled)
  - candidate: pending-commit
- [x] T6. Detector mecánico y verificación en navegador
  - observed: `none` → none (gate disabled)
  - candidate: pending-commit

## Outcome

- verified: 6 of 6 task(s)
- [x] T1: `none` → none (gate disabled)
- [x] T2: `none` → none (gate disabled)
- [x] T3: `none` → none (gate disabled)
- [x] T4: `none` → none (gate disabled)
- [x] T5: `none` → none (gate disabled)
- [x] T6: `none` → none (gate disabled)

## Progress


