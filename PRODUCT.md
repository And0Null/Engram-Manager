# Product

<!-- impeccable:product-schema 1 -->

## Platform

web

## Users

One person: the repository owner, working alone on a personal Linux workstation
(Omarchy/Hyprland desktop, 8-thread laptop, no GPU). They open the dashboard to
understand what their agent fleet is remembering — what is growing, what is
duplicated, what is drifting — and to inspect individual memories when a search
or a conflict needs explaining. There is no login, no team, and no deployment:
the dashboard binds to loopback and exists for one operator at a glance.

## Product Purpose

Engram Manager is the inspection and management surface for an Engram memory
store. It answers three questions quickly: what the store currently holds, how
it is changing over time, and what a specific memory means in context (its
relations, its vector, the session that produced it).

It serves those questions through three surfaces over one engine:

- `engram-manager tui` — keyboard-driven, dense, for working in a terminal.
- `engram-manager web` — a browser dashboard for reading shapes and trends.
- `engram-manager status` — one-shot stdout for scripts and health checks.

Success means the operator reads an accurate picture in seconds and never has to
open a SQLite client or remember an Engram CLI incantation.

## Positioning

A hybrid read/write manager over someone else's memory engine: reads go straight
to Engram's SQLite in `mode=ro` for instant FTS5 and aggregation, while every
mutation is delegated to Engram's own HTTP API so its optimistic concurrency,
dedupe, and relation rules stay authoritative. A dashboard that reimplemented
those rules would drift on every Engram upgrade; this one cannot.

Engram Manager also reports on the vector layer that Engram itself is unaware of —
embeddings are written by local tooling, and coverage, model, and per-memory
vector presence are first-class operational facts, not an implementation detail.

## Operating Context

The store lives at `~/.engram/engram.db` and is almost always being written to by
other agents while it is being read. Timestamps are SQLite UTC strings. Projects
are derived from sessions, not a projects table. Memories embed locally
(`paraphrase-multilingual-MiniLM-L12-v2`, 384 dims) via a debounced watcher, so
vector coverage is a moving target and pending counts are normal, not an error.

The operator's environment is a dark desktop; the dashboard is read in a browser
window that is usually not full screen and not maximized.

## Capabilities and Constraints

Confirmed:

- Read-only SQLite access; all writes through `engram serve` with
  `expected_project`.
- Views: Overview, Memories (FTS trigram + filters), Relations and conflicts,
  Session timelines with prompts, Review queue, System health.
- Memory actions: read, search, edit title/type/content, pin, soft-delete, hard
  delete, mark reviewed, copy id, export context.
- Vector store reporting: coverage, model, dimensions, per-memory vector badge.
- Zero-build: frontend assets are embedded in the binary with `go:embed`; no npm,
  no bundler, no CDN, no web fonts.
- Go 1.26 `http.ServeMux` patterns; no third-party router or chart dependency.
- Loopback only; no authentication, by design.

Explicitly undecided: whether hard delete is ever exposed in the browser; whether
the dashboard ever binds beyond loopback.

## Evidence on Hand

- Live store: ~962 observations, ~293 sessions, ~1475 relations, 962 vectors at
  100% coverage. Real data, always present, never synthetic.
- `References/` screenshot of `eSagraAI/engram-dashboard-local`, the reference
  whose Overview composition this dashboard is being aligned with.
- No testimonials, pricing, or benchmark claims exist, and none may be invented.

## Product Principles

1. **Never lie by omission.** If a count is partial, capped, or stale, the surface
   says so. Silence is the one failure mode this product cannot have.
2. **Reads are free, writes are negotiated.** Analytics never wait on a lock;
   mutations never bypass Engram's own rules.
3. **One engine, three surfaces.** A fact shown in the TUI, the CLI, and the web
   is the same fact from the same query, never a re-derivation.
4. **Dense is a feature, clutter is not.** The operator wants density they can
   scan, not decoration; every pixel of chrome must carry information.
5. **Degrade, never crash.** A missing table, an absent vector store, or an
   unreachable engine must render as a stated fact, not an error page.

## Accessibility & Inclusion

Single-operator local tool; the operator is the author and sole reader, so no
formal accessibility standard was established. Known constraint to preserve:
color must never be the only channel carrying meaning (counts and labels are
always printed next to the color), and text must stay legible at the density the
operator chose.