package tui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
)

const previewWidth = 70

// View renders the current state.
func (m Model) View() string {
	if m.width == 0 || m.height == 0 {
		return "loading…"
	}
	var b strings.Builder

	b.WriteString(m.header())
	b.WriteString("\n")

	switch m.view {
	case viewError:
		b.WriteString(m.errorView())
	case viewDetail:
		b.WriteString(m.detailView())
	case viewEdit:
		b.WriteString(m.editView())
	case viewConfirm:
		b.WriteString(m.confirmView())
	case viewHealth:
		b.WriteString(m.healthView())
	case viewSessions:
		b.WriteString(m.sessionsView())
	default:
		b.WriteString(m.listView())
	}

	b.WriteString("\n")
	b.WriteString(m.footer())
	return b.String()
}

func (m Model) header() string {
	brand := styleBadge.Render("ENGRAM") + " " + styleTitle.Render("Manager")
	dbPath := styleBadgeMuted.Render(trunc(m.store.Path(), 40))

	parts := []string{brand, dbPath}

	if m.filter.Project != "" {
		parts = append(parts, styleBadge.Render("project:"+m.filter.Project))
	}
	if m.filter.Type != "" {
		parts = append(parts, styleBadge.Render("type:"+m.filter.Type))
	}
	if m.filter.IncludeDeleted {
		parts = append(parts, styleBadgeWarn.Render("deleted:shown"))
	}
	if m.filter.Query != "" {
		parts = append(parts, styleBadge.Render("search: "+m.filter.Query))
	}

	return strings.Join(parts, " ")
}

func (m Model) filterSummary() string {
	parts := []string{}
	if m.filter.Project != "" {
		parts = append(parts, "project="+m.filter.Project)
	}
	if m.filter.Type != "" {
		parts = append(parts, "type="+m.filter.Type)
	}
	if m.filter.IncludeDeleted {
		parts = append(parts, "deleted=shown")
	}
	return strings.Join(parts, " ")
}

// isWide returns true if the terminal has enough space for a 2-panel lazygit view.
func (m Model) isWide() bool {
	return m.width >= 90
}

func (m Model) listView() string {
	if len(m.items) == 0 {
		emptyBox := styleBoxInactive.
			Width(m.width - 4).
			Height(clamp(m.height-6, 5, 20)).
			Render("\n  no memories match the current filter.\n  Press / to search or esc to reset.\n")
		return "\n" + emptyBox
	}

	if m.isWide() {
		return m.splitView()
	}

	return m.singleListView()
}

func (m Model) splitView() string {
	leftWidth := clamp(m.width*38/100, 32, 55)
	rightWidth := m.width - leftWidth - 5
	paneHeight := clamp(m.height-6, 5, 45)

	// Left pane: memory list
	leftContent := m.renderRows(leftWidth-4, paneHeight)
	leftPane := styleBoxActive.
		Width(leftWidth).
		Height(paneHeight).
		Render(leftContent)

	// Right pane: live preview of selected memory
	rightContent := m.renderSelectedPreview(rightWidth - 4)
	rightPane := styleBoxInactive.
		Width(rightWidth).
		Height(paneHeight).
		Render(rightContent)

	return lipgloss.JoinHorizontal(lipgloss.Top, leftPane, " ", rightPane)
}

func (m Model) renderRows(width, maxRows int) string {
	visible := maxRows
	if visible > len(m.items) {
		visible = len(m.items)
	}
	start := m.cursor - visible/2
	if start < 0 {
		start = 0
	}
	if start > len(m.items)-visible {
		start = len(m.items) - visible
	}
	if start < 0 {
		start = 0
	}
	end := min(start+visible, len(m.items))

	var b strings.Builder
	for i := start; i < end; i++ {
		b.WriteString(m.rowSplit(i, width))
		if i < end-1 {
			b.WriteString("\n")
		}
	}
	return b.String()
}

func (m Model) rowSplit(i, width int) string {
	o := m.items[i]
	prefix := "  "
	if i == m.cursor {
		prefix = "▸ "
	}
	if o.Pinned {
		prefix = "★ "
	}

	titleWidth := clamp(width-len(prefix)-12, 10, 40)
	title := trunc(oneLine(o.Title), titleWidth)
	typeStr := trunc(o.Type, 10)

	line := fmt.Sprintf("%s%s  %s",
		prefix,
		pad(title, titleWidth),
		styleDim.Render(typeStr),
	)

	if o.Deleted() {
		line = styleDeleted.Render(line)
	}

	if i == m.cursor {
		return styleRowSelected.Width(width).Render(line)
	}
	return styleRowNormal.Render(line)
}

func (m Model) renderSelectedPreview(width int) string {
	if len(m.items) == 0 || m.cursor < 0 || m.cursor >= len(m.items) {
		return styleDim.Render("No memory selected")
	}
	o := m.items[m.cursor]

	var b strings.Builder
	idBadge := styleBadge.Render(fmt.Sprintf("#%d", o.ID))
	title := styleTitle.Render(oneLine(o.Title))
	b.WriteString(fmt.Sprintf("%s  %s\n", idBadge, title))

	meta := fmt.Sprintf("%s · %s · scope=%s · project=%s",
		styleAccent.Render(o.Type),
		styleDim.Render(o.CreatedAt),
		styleMuted.Render(o.Scope),
		styleBadgeMuted.Render(o.Project))
	b.WriteString(meta + "\n")

	if o.Deleted() {
		b.WriteString(styleError.Render(fmt.Sprintf("soft-deleted at %s\n", o.DeletedAt)))
	}
	if o.RevisionCount > 1 || o.DuplicateCount > 1 {
		b.WriteString(styleDim.Render(fmt.Sprintf("%d revisions · %d duplicates · session %s\n",
			o.RevisionCount, o.DuplicateCount, trunc(o.SessionID, 12))))
	}

	b.WriteString("\n" + styleDim.Render(strings.Repeat("─", width)) + "\n")

	// Content with wrapping
	contentLines := wrap(o.Content, width)
	maxContent := clamp(m.height-14, 5, 25)
	for idx, line := range contentLines {
		if idx >= maxContent {
			b.WriteString(styleDim.Render(fmt.Sprintf("… and %d more lines (press Enter to view full)", len(contentLines)-maxContent)))
			break
		}
		b.WriteString(line + "\n")
	}

	return b.String()
}

func (m Model) singleListView() string {
	visible := m.height - 5
	if visible < 3 {
		visible = 3
	}
	if visible > len(m.items) {
		visible = len(m.items)
	}
	start := m.cursor - visible/2
	if start < 0 {
		start = 0
	}
	if start > len(m.items)-visible {
		start = len(m.items) - visible
	}
	if start < 0 {
		start = 0
	}
	end := min(start+visible, len(m.items))

	var b strings.Builder
	for i := start; i < end; i++ {
		b.WriteString(m.row(i))
		b.WriteString("\n")
	}
	return b.String()
}

func (m Model) row(i int) string {
	o := m.items[i]
	marker := "  "
	if i == m.cursor {
		marker = "> "
	}
	pin := "  "
	if o.Pinned {
		pin = "* "
	}

	titleWidth := clamp(m.width/4, 16, 52)
	projectWidth := clamp(m.width/8, 8, 20)
	typeWidth := clamp(m.width/10, 6, 14)
	fixed := 2 + 2 + titleWidth + 1 + projectWidth + 1 + typeWidth + 2
	preview := o.Preview(clamp(m.width-fixed, 10, previewWidth))

	row := fmt.Sprintf("%s%s%s  %s  %s  %s",
		marker, pin,
		pad(trunc(oneLine(o.Title), titleWidth), titleWidth),
		pad(trunc(o.Project, projectWidth), projectWidth),
		pad(trunc(o.Type, typeWidth), typeWidth),
		styleDim.Render(preview),
	)
	if o.Deleted() {
		row = styleDeleted.Render(row)
	}
	if i == m.cursor {
		row = styleRowSelected.Render(row)
	}
	return row
}

func (m Model) detailView() string {
	o := m.detail
	var b strings.Builder

	b.WriteString(fmt.Sprintf("\n%s  %s\n", styleBadge.Render("#"+fmt.Sprint(o.ID)), styleTitle.Render(oneLine(o.Title))))
	b.WriteString(styleDim.Render(fmt.Sprintf(
		"%s · %s · scope=%s · project=%s\n", o.Type, o.CreatedAt, o.Scope, o.Project)))
	if o.Deleted() {
		b.WriteString(styleError.Render(fmt.Sprintf("  soft-deleted at %s\n", o.DeletedAt)))
	}
	if o.RevisionCount > 1 || o.DuplicateCount > 1 {
		b.WriteString(styleDim.Render(fmt.Sprintf(
			"  %d revisions · %d duplicates · session %s\n", o.RevisionCount, o.DuplicateCount, o.SessionID)))
	}

	b.WriteString("\n")
	for _, line := range wrap(o.Content, m.width-4) {
		b.WriteString("  " + line + "\n")
	}

	if len(m.relations) > 0 {
		b.WriteString("\n  " + styleAccent.Render("relations") + "\n")
		for _, r := range m.relations {
			other := r.TargetID
			if other == o.ID {
				other = r.SourceID
			}
			b.WriteString(fmt.Sprintf("    %s %d → %d  %s\n",
				styleTitle.Render(r.Relation), o.ID, other, styleDim.Render(r.JudgmentStatus)))
		}
	}
	if len(m.versions) > 0 {
		b.WriteString("\n  " + styleAccent.Render("revisions") + "\n")
		for _, v := range m.versions {
			b.WriteString(styleDim.Render(fmt.Sprintf("    v%d  %s  %s\n",
				v.Version, v.CreatedAt, oneLine(trunc(v.Title, 50)))))
		}
	}
	if len(m.timeline) > 0 {
		b.WriteString("\n  " + styleAccent.Render("same session, around it") + "\n")
		for _, t := range m.timeline {
			b.WriteString(styleDim.Render(fmt.Sprintf("    #%-5d %s  %s\n",
				t.ID, t.CreatedAt, oneLine(trunc(t.Title, 46)))))
		}
	}
	return b.String()
}

func (m Model) errorView() string {
	return "\n  " + styleError.Render("error: "+m.err.Error()) + "\n\n  press esc to go back\n"
}

func (m Model) editView() string {
	var b strings.Builder
	boxContent := fmt.Sprintf("%s  %s\n\n  %s\n\n  %s",
		styleTitle.Render("EDITING MEMORY"),
		styleDim.Render(fmt.Sprintf("#%d · %s", m.editing.ID, m.editing.Project)),
		m.title.View(),
		m.body.View(),
	)
	box := styleBoxModal.Width(clamp(m.width-4, 40, 90)).Render(boxContent)
	b.WriteString("\n" + box + "\n")
	return b.String()
}

func (m Model) confirmView() string {
	if m.pending == nil {
		return "\n  nothing to confirm\n"
	}
	verb := "SOFT-DELETE"
	warning := "The row stays and can be restored; press P to see it."
	if m.pendingHard {
		verb = "PERMANENTLY DELETE"
		warning = "This cannot be undone."
	}

	var inner strings.Builder
	inner.WriteString(styleError.Render(verb) + " " + styleTitle.Render(fmt.Sprintf("#%d", m.pending.ID)) + "\n\n")
	for _, line := range wrap(m.pending.Title, clamp(m.width-12, 30, 80)) {
		inner.WriteString("  " + line + "\n")
	}
	inner.WriteString("\n  " + styleWarning.Render(warning) + "\n\n")
	inner.WriteString("  " + styleSuccess.Render("y") + " confirm    " + styleDim.Render("n cancel") + "\n")

	modal := styleBoxModal.Width(clamp(m.width-4, 40, 80)).Render(inner.String())
	return "\n" + modal
}

func (m Model) footer() string {
	if m.searching {
		return "\n" + styleTitle.Render("search: "+m.search+"▌") +
			styleDim.Render("   enter apply · esc clear")
	}
	if m.view == viewEdit {
		return "\n" + renderKeyHelp("ctrl+s", "save") +
			renderKeyHelp("esc", "discard") +
			renderKeyHelp("tab", "switch field") +
			statusTail(m)
	}
	if m.view == viewConfirm {
		return "\n" + statusTail(m)
	}
	if m.view == viewHealth {
		return "\n" + renderKeyHelp("r", "refresh") +
			renderKeyHelp("esc", "back") +
			statusTail(m)
	}
	if m.view == viewSessions {
		return "\n" + renderKeyHelp("↑/↓", "move") +
			renderKeyHelp("r", "refresh") +
			renderKeyHelp("esc", "back") +
			statusTail(m)
	}

	left := styleBadgeMuted.Render(fmt.Sprintf("%d/%d", min(m.cursor+1, len(m.items)), m.total)) + " "
	switch {
	case m.loading:
		left += styleDim.Render("loading… ")
	case m.status != "":
		left += styleDim.Render(m.status + " ")
	}

	if m.view == viewDetail {
		return "\n" + left +
			renderKeyHelp("e", "edit") +
			renderKeyHelp("d", "delete") +
			renderKeyHelp("D", "hard-del") +
			renderKeyHelp("p", "pin") +
			renderKeyHelp("r", "reviewed") +
			renderKeyHelp("esc", "back")
	}

	// Lazygit-style command pills in list view
	pills := []string{
		renderKeyHelp("↑/↓", "nav"),
		renderKeyHelp("enter", "expand"),
		renderKeyHelp("/", "search"),
		renderKeyHelp("m", "health"),
		renderKeyHelp("s", "sessions"),
		renderKeyHelp("P", "deleted"),
		renderKeyHelp("q", "quit"),
	}

	return "\n" + left + strings.Join(pills, "")
}

func statusTail(m Model) string {
	if m.status == "" {
		return ""
	}
	return " " + styleDim.Render(m.status)
}

func pad(s string, n int) string {
	r := []rune(s)
	if len(r) >= n {
		return s
	}
	return s + strings.Repeat(" ", n-len(r))
}

func trunc(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n-1]) + "…"
}

func oneLine(s string) string {
	out := make([]rune, 0, len(s))
	var lastSpace bool
	for _, r := range s {
		if r == '\n' || r == '\r' || r == '\t' {
			r = ' '
		}
		if r == ' ' && lastSpace {
			continue
		}
		lastSpace = r == ' '
		out = append(out, r)
	}
	return strings.TrimSpace(string(out))
}

func wrap(s string, width int) []string {
	if width < 20 {
		width = 20
	}
	var lines []string
	for _, para := range strings.Split(strings.ReplaceAll(s, "\r\n", "\n"), "\n") {
		words := strings.Fields(para)
		if len(words) == 0 {
			lines = append(lines, "")
			continue
		}
		cur := ""
		for _, w := range words {
			if cur == "" {
				cur = w
				continue
			}
			if len([]rune(cur))+1+len([]rune(w)) > width {
				lines = append(lines, cur)
				cur = w
				continue
			}
			cur += " " + w
		}
		lines = append(lines, cur)
	}
	return lines
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
