# Engram Manager design system

Reusable visual and interaction guidance for Overview and future Memories, Relations, Sessions, Review, and System & Health screens.

## Product direction

Use a quiet, high-contrast developer-console style: near-black neutral canvas, subtly lifted charcoal surfaces, crisp borders, restrained blue interaction accent, and compact but breathable information density. Treat data as the visual material. Avoid decoration, gradients, glows, and ornamental charting (the only ring-shaped chart allowed is the functional type donut described under Components). The single accent is for interaction and the known activity peak, never for memory-type categories.

The canonical reference is `index.html`; `components.html` demonstrates reusable controls and states. Both are standalone HTML files. The Overview's range control changes every range-sensitive widget, and the template runs on clearly labeled sample data (see the data rules and contract below).

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
- At 1024px, reduce the sidebar to about 204px, retain the four KPI columns when each remains legible, and stack the chart below type distribution if the pair becomes cramped.
- Below tablet width, make navigation horizontally scrollable, then stack the toolbar and cards in reading order. Preserve access to all navigation labels and actions; never hide the selected range, search, or refresh affordance.
- Overview order: KPI row, then the full-width Daily activity chart as the hero, then Memory types (left: donut beside the ranked list) beside System health (right). Connection and WAL status live only in the sidebar.
- Use a consistent 20–24px gap between major content regions and 12–16px between related cards. Maintain a 4px spacing grid.
- Page title and subtitle sit at the upper left; range, search, and refresh controls align to the right on desktop. Card headers always place the title at left and filters/actions/meta at right.
- Sidebar connection status stays pinned to the bottom on desktop. Compact widths may move it into System & Health, but must retain the same connection and DB/WAL facts.
- Use normal flex/grid flow for content. Avoid overlays and absolute positioning for sibling content. Reserve fixed positioning for the desktop sidebar and sticky top bar only.
- Chart labels and hover/focus tooltip must stay within the viewport. Ensure data-table-like rows wrap or reflow without clipping.

## Data, range, and sample-data rules

- The template ships with one `DATA` object in `index.html` and `DATA.source = "sample"`. While `source` is `"sample"` the top bar shows a **Sample data** badge. Real data is never invented: set `source` to `"api"` and the badge disappears.
- Sample invariants (asserted in code): 30 daily counts sum to 1,080 (avg 36.0), peak 110 on Oct 05, last 7 days sum to 289, last 24 hours sum to 47. Every type breakdown sums to its range total (24h 47, 7d 289, 30d 1,080, all-time 1,417).
- One series feeds every range: `7d` = last 7 daily bars, `30d` = all 30, `24h` = 24 hourly bars. No range may render an empty dashboard.
- "New in range" shows a delta against the prior period when one exists (7d vs previous 7 days, 24h vs previous day). With no prior period (30d) the delta is neutral and says so. Never show a delta without both values.
- Shares are whole percents; values below 1% display `<1%`. Shares use the visible total (re-normalized when `session_summary` is excluded).
- Total memories, relations, and pending review are current totals and do not vary by range. The "All time" override on type distribution resets to "In range" when a dashboard range is selected.
- Vector coverage: compact success pill at 100%; a progress bar only below 100%. WAL close to DB size shows a labeled warning pill in the sidebar; do not invent a threshold.
- Search and refresh are presentational unless connected to a real store.

### Data contract

```json
{
  "source": "sample | api",
  "start": "YYYY-MM-DD",
  "daily": [0],
  "hourly": [0],
  "topTypes": ["session_summary"],
  "types": { "all | 24h | 7d | 30d": { "<type>": 0 } },
  "totals": { "memories": 0, "types": 0, "relations": 0, "review": 0 }
}
```

`daily` has 30 counts, oldest to newest, starting at `start`; `hourly` has 24. `topTypes` is optional (top type per day, used in tooltips). Each type maps to its fixed color token; unknown types render with `--type-other`.

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
| Type donut | Functional proportion summary beside the ranked list in the Memory types card. Maximum 5 segments: the top 4 individual types plus one neutral `--type-other` segment for everything else. 20px stroke, 3px gaps between segments, current total in the center with the range as caption. It never replaces the list, which carries exact values and every type. Segments expose a title and the SVG has a text alternative. Shares use the visible total, so excluding `session_summary` re-normalizes both donut and list. |
| Activity chart | Hero card, full width. One bar per day (hourly for 24h) in neutral slate, only the peak in `--accent`, dashed average line, Y-axis with gridlines, date ticks computed from data. Every bar is focusable and shows a tooltip with date, count, and top type on hover and focus. |
| Status pill | Success/warning/error each include a glyph and text label as well as semantic color. |
| Progress bar | Accessible label/value, only shown for incomplete vector coverage; full coverage uses the compact status pill. |
| Card header | Title at left, range/filter/actions at right; one divider separates header from content. |
| Skeleton loader | Match final card geometry; use for loading state and respect reduced motion. |
| Empty state | State what is missing and the next useful action or available range; never leave a blank card. |
| Error state | Explain the affected card and recovery action; retain selected filters/range and avoid hiding neighboring cards. |

Every data card supports loading, empty, and error states inside the same card frame. KPI cards use skeleton label/value/helper rows; chart/type cards use concise state copy and a retry where a retry is meaningful. For unavailable source data, label that the data was not provided rather than presenting a service error.

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
- Add decorative charts, glow, gradients, or unexplained ornament, or use a donut with more than 5 segments or without the ranked list beside it.
- Reassign category colors or use the blue interaction accent as a type color.
- Present sample values as real: keep the Sample data badge visible while `DATA.source` is `"sample"`, and never invent values, deltas, top types, or thresholds for real data.
- Use low-contrast gray, color-only state, ultra-wide tracked uppercase section labels, or tiny action targets.
- Hide core controls at responsive breakpoints or use a loading spinner/blank panel where a skeleton or state message is needed.
- Label dimensions as `384d`; use `384 dim`.
