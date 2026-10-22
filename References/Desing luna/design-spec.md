# Engram Manager design system

Reusable visual and interaction guidance for Overview and future Memories, Relations, Sessions, Review, and System & Health screens.

## Product direction

Use a quiet, high-contrast developer-console style: near-black neutral canvas, subtly lifted charcoal surfaces, crisp borders, restrained blue interaction accent, and compact but breathable information density. Treat data as the visual material. Avoid decoration, gradients, glows, rings, donuts, and ornamental charting. The single accent is for interaction and the known activity peak, never for memory-type categories.

The canonical reference is `index.html`; `components.html` demonstrates reusable controls and states. Both are standalone HTML files. The Overview's range control updates every range-sensitive widget, and the selected range is reflected in the chart title, badge, activity data, KPI, and type list.

## Design tokens

| Token | Value | Use |
|---|---|---|
| `--app` | `#101216` | App canvas |
| `--surface` | `#171A20` | Sidebar and cards |
| `--elevated` | `#20242C` | Hovered controls and raised elements |
| `--border` | `#2B303A` | Subtle surface boundaries and dividers |
| `--border-strong` | `#3A414D` | Control outlines and emphasis |
| `--text` | `#F1F3F6` | Primary text |
| `--text-secondary` | `#B0B7C3` | Supporting labels and body copy |
| `--text-muted` | `#8992A1` | Captions, axes, and secondary metadata; do not reduce contrast further |
| `--accent` | `#78A9FF` | Focus, selected navigation, and known activity peak only |
| `--accent-strong` | `#9BC0FF` | Accent foreground on dark surfaces |
| `--success` | `#56C596` | Healthy status |
| `--warning` | `#F0B55C` | Attention needed, including review and WAL proximity |
| `--danger` | `#F07878` | Failure and destructive/error state |
| `--info` | `#6BB9DF` | Informational status |
| `--type-session-summary` | `#93A4BA` | `session_summary` |
| `--type-discovery` | `#56C5B7` | `discovery` |
| `--type-decision` | `#B99AF7` | `decision` |
| `--type-config` | `#68A6F0` | `config` |
| `--type-bugfix` | `#E47878` | `bugfix` |
| `--type-pattern` | `#D58BC9` | `pattern` |
| `--type-architecture` | `#7BC77F` | `architecture` |
| `--type-feature` | `#D89A59` | `feature` |
| `--type-other` | `#7F8793` | Unspecified and aggregated types only |
| `--font-ui` | Inter, system sans fallback | Interface text |
| `--font-mono` | JetBrains Mono, system monospace fallback | Metrics, IDs, dates, and model names |
| `--display-size` / line | `40px` / `44px` | Display metric scale |
| `--h1-size` | `24px` | Page title |
| `--h2-size` | `16px` | Card and section title |
| `--body-size` | `14px` | Default interface copy |
| `--caption-size` | `12px` | Captions and compact metadata |
| Spacing | `4px` base; use `4, 8, 12, 16, 20, 24, 32px` | Layout rhythm |
| Control radius | `8px` | Inputs, buttons, segmented controls |
| Card radius | `12px` | Content cards |
| Card padding | `20–24px` desktop; `16px` compact | Content card interior |
| Sidebar width | `240px` desktop | Persistent workspace navigation |

Text colors must retain WCAG AA contrast against their actual surface; recheck when changing a token or surface. Keep backgrounds and borders visibly distinct without relying on a shadow. All metric values use tabular numerals and never shift as digits change.

## Memory-type color mapping

| Type | Token | Color | Notes |
|---|---|---|---|
| `session_summary` | `--type-session-summary` | `#93A4BA` | Cool slate |
| `discovery` | `--type-discovery` | `#56C5B7` | Teal |
| `decision` | `--type-decision` | `#B99AF7` | Violet |
| `config` | `--type-config` | `#68A6F0` | Blue |
| `bugfix` | `--type-bugfix` | `#E47878` | Soft red |
| `pattern` | `--type-pattern` | `#D58BC9` | Orchid |
| `architecture` | `--type-architecture` | `#7BC77F` | Green |
| `feature` | `--type-feature` | `#D89A59` | Muted amber |
| Other/aggregated types | `--type-other` | `#7F8793` | Neutral gray only |

Keep this mapping fixed across charts, chips, filters, and future screens. Never recycle the interaction accent for a type. For unlisted types, use neutral gray rather than inventing a ninth hue. Always pair color with the written type name and shape/position; do not use hue as the only identifier.

## Layout and responsive rules

- Desktop reference canvas: 1440px wide. Use a fixed 240px sidebar and a fluid content region; keep the content readable rather than stretching charts edge-to-edge on very wide windows.
- Place the full-width Daily activity card directly after the four KPI cards; keep its minimum height at 260px so it reads as the primary visualization. Below it, put the ranked single-column Memory types card on the left and stacked System health card on the right.
- At 1024px, reduce the sidebar to about 204px, retain the four KPI columns when each remains legible, and stack the lower cards when the pair becomes cramped.
- Below tablet width, make navigation horizontally scrollable, then stack the toolbar and cards in reading order. Preserve access to all navigation labels and actions; never hide the selected range, search, or refresh affordance.
- Use a consistent 20–24px gap between major content regions and 12–16px between related cards. Maintain a 4px spacing grid.
- Page title and subtitle sit at the upper left; range, search, and refresh controls align to the right on desktop. Card headers always place the title at left and filters/actions/meta at right.
- Sidebar connection status stays pinned to the bottom on desktop and owns the connection, database, and WAL facts. Do not duplicate connection or WAL details in System health.
- Memory-type bars are one column, ranked by count, with an 8px bar; normalize bar width to the largest visible row.
- Use normal flex/grid flow for content. Avoid overlays and absolute positioning for sibling content. Reserve fixed positioning for the desktop sidebar and sticky top bar only.
- Chart labels and hover/focus tooltip must stay within the viewport. Ensure data-table-like rows wrap or reflow without clipping.

## Data contract

`index.html` keeps all dashboard values in one `DATA` object at the top of its script. The shape below is the source contract; values shown as placeholders describe types, not display data.

```js
const DATA = {
  source: "sample" | "real",
  daily: [{ date: "YYYY-MM-DD", count: Number, topType: String }],
  hourly: [{ hour: "HH:00", count: Number, topType: String }],
  typeCounts: {
    all: { [type: Number]: Number },
    "24h": { [type: Number]: Number },
    "7d": { [type: Number]: Number },
    "30d": { [type: Number]: Number }
  },
  totals: {
    memories: Number, relations: Number, sessions: Number,
    pendingReview: Number, memoryTypes: Number,
    databaseMiB: Number, walMiB: Number,
    vectorCovered: Number, vectorTotal: Number, dimensions: Number,
    embeddingModel: String,
    previousPeriod: { "24h": Number, "7d": Number, "30d": Number }
  }
};
```

Widget bindings:

| Widget | Data contract |
|---|---|
| Sidebar | Fixed navigation labels; counts from `totals.memories`, `relations`, `sessions`, and `pendingReview`. |
| Connection block | Connection label plus `totals.databaseMiB` and `totals.walMiB`; keep this information in the sidebar only. |
| Memories KPI | `totals.memories` and `totals.memoryTypes`, labeled as current/all-time. |
| New-in-range KPI | Sum selected `hourly` for `24h`, last seven `daily` entries for `7d`, all `daily` entries for `30d`; compare to matching `totals.previousPeriod` only when `source` is `sample` or when a real comparison is provided. |
| Pending review / Relations KPIs | `totals.pendingReview` and `totals.relations`; labels identify current totals. |
| Activity chart | `hourly` for `24h`, last seven `daily` entries for `7d`, all `daily` entries for `30d`; `date`/`hour`, `count`, `topType` drive each bar and tooltip. Average is derived from selected counts. |
| Memory types | `typeCounts[range]`, or `typeCounts.all` under the explicit All time override. Shares are `count / selected total`; display nonzero shares below 1% as `<1%`. |
| System health | `vectorCovered`, `vectorTotal`, `dimensions`, and `embeddingModel`; show a progress bar only when computed coverage is below 100%. |
| Sample badge | Show `Sample data` only when `DATA.source === "sample"`. |

The type-to-color mapping is declared once as a fixed `TYPE_TOKEN` map and reused for every period. Any unknown type resolves to `--type-other`; `manual`, `preference`, `change`, and other unlisted categories use that same neutral mapping in every range.

## Sample vs real data

Real data is never invented; the template ships with clearly labeled sample data so every widget is demonstrable.

The sample daily series covers Sep 22–Oct 21 and deterministically reconciles to 1,080 total entries (36.0 average), a peak of 110 on Oct 05, and 289 entries across Oct 15–21. Weekend bars are near zero. The hourly sample contains one bar per hour and reconciles to the final daily total. Per-range type-count totals reconcile to their corresponding bar sums. Keep `console.assert` checks for series length, totals, average, peak/date, last-seven-day sum, weekend bounds, and type-count reconciliation.

All generated comparison values and daily/hourly details are explicitly sample content and are covered by the `Sample data` badge. When `source` is `real`, connect the object to actual records and remove the invented sample comparisons from the data. Never silently promote sample series, top types, or previous-period values as real telemetry. A real source without comparison history uses the neutral delta state.

For the chart, position X values from parsed dates/index coordinates, never independent hardcoded percentage offsets. The full-month axis uses five-day ticks and also includes the known Oct 05 date tick; the Oct 05 bar center must share that tick's X coordinate. The 7d chart uses seven wider bars; the 24h chart uses 24 hourly bars.

## Component inventory and usage

| Component | Usage and behavior |
|---|---|
| Sidebar item | Persistent navigation, 32px minimum target, clear hover and keyboard focus; active state combines surface fill, border, and `aria-current`. |
| Count badge | Right-aligned tabular count. Review badge adds warning border/background and explicit action cue; color is not the only signal. |
| KPI card | Label, large tabular value, and concise supporting context. New-in-range includes up/down/neutral delta variants; never show delta without real comparison values. Pending review is an actionable link/button. |
| Segmented control | 24h / 7d / 30d; use `aria-pressed`, selected surface and border, visible focus; each selection updates every range-sensitive widget. |
| Search input | Persistent accessible name, search glyph, `/` hint; slash focuses it, Escape clears it. |
| Button | Primary for the main action, secondary for ordinary actions, ghost for low-emphasis actions. Include hover, focus, disabled, and busy treatments. |
| Type chip | Fixed type color plus readable name; neutral gray for other types. |
| Bar row | Type chip, proportional horizontal bar, count, and share. Normalize bar width to the largest visible row, not the total. |
| Activity chart | Full-width hero beneath KPIs, minimum height 260px, visible Y-axis/grid, average line, selected range badge, five-day date ticks, one bar per item, keyboard-focusable/clickable bars, and clamped hover/focus tooltip with date-or-hour, count, and top type. |
| Status pill | Success/warning/error each include a glyph and text label as well as semantic color. |
| Progress bar | Accessible label/value, only shown for incomplete vector coverage; full coverage uses the compact status pill. |
| Card header | Title at left, range/filter/actions at right; one divider separates header from content. |
| Skeleton loader | Match final card geometry; use for loading state and respect reduced motion. |
| Empty state | State what is missing and the next useful action or available range; never leave a blank card. |
| Error state | Explain the affected card and recovery action; retain selected filters/range and avoid hiding neighboring cards. |

Every data card supports loading, empty, and error states inside the same card frame. KPI cards use skeleton label/value/helper rows; chart/type cards use concise state copy and a retry where a retry is meaningful. The sample source renders complete chart and type data so the template never opens as an empty dashboard. For unavailable real-source data, label the missing data rather than presenting a service error.

## Accessibility and interaction

- Target at least 32px for interactive controls; on touch layouts use larger targets and spacing where practical.
- Provide a clearly visible focus ring on all keyboard-operable controls. Keep focus order aligned with visual reading order.
- Use semantic buttons, links, labels, headings, and chart descriptions. Announce updates to type counts and dynamic range states without forcing focus movement.
- Pair all semantic status colors with words and icons. Never encode meaning only with hue.
- Keep text legible at AA contrast, preserve zoom, and honor `prefers-reduced-motion`.
- Hover is supplementary; every hover affordance must also work by keyboard and touch.
- Tooltips must be reachable by focus as well as pointer. Do not make critical data hover-only.

## Do / Don't

**Do**
- Reuse token variables and the same card/header, spacing, and state patterns on Memories, Relations, Sessions, Review, and System & Health.
- Keep counts, IDs, dates, and model names in JetBrains Mono with tabular numerals.
- Keep action-required review and WAL proximity visibly labeled and keyboard reachable.
- Show missing measurements honestly and preserve the selected date range.
- Use consistent sentence case and short English labels; format dates as `Oct 05`.

**Don't**
- Add decorative charts, donuts, rings, glow, gradients, or unexplained ornament.
- Reassign category colors or use the blue interaction accent as a type color.
- Invent daily values, growth deltas, top types, thresholds, or records to fill a visual gap.
- Use low-contrast gray, color-only state, ultra-wide tracked uppercase section labels, or tiny action targets.
- Hide core controls at responsive breakpoints or use a loading spinner/blank panel where a skeleton or state message is needed.
- Label dimensions as `384d`; use `384 dim`.
