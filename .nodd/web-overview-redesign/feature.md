# Feature: Land the combined Overview design (Sonnet structure + Luna chart)

<!-- nodd:slug web-overview-redesign -->

## Objective

Replace the Overview view's radar/instrument layout with the approved reference design: KPI row, full-width activity chart with a date-proportional engine, memory types with a functional donut, and a system-health card, on the reference token system.

## Problem

The live Overview renders a radar chart and an instrument-panel rail that the approved design explicitly forbids, and it has no hourly bucket for the 24h range the topbar already offers.

## Scope

Overview only. Tokens are added as the canonical system with the old names aliased, so the other five views inherit the new ground and hues without being rewritten. Adds an hourly activity series because the 24h range has no bucketing today. Excludes: rewriting Memories/Relations/Sessions/Review/System screens, per-day top-type data, and a web CDN font dependency.

## Constraints

No new dependency; no schema change; keep the other views working; keep the page self-contained (no external font CDN).

## Route

- intent: change
- route: tracked

## Verification

- runner: go test ./...
- tdd: off
- source: nodd_declare
- files: internal/store/model.go, internal/store/stats.go, internal/store/relations_test.go, internal/web/server.go, internal/web/static/app.js, internal/web/static/app.css

## Delivery

- strategy: ask-on-risk
- chain: stacked-to-main
- forecast: 0
- running: unknown

## Tasks



## Outcome

No tasks declared yet.

## Progress


