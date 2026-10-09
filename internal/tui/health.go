package tui

import (
	"fmt"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"engram-manager/internal/store"
)

// healthMsg carries the metrics snapshot into the model.
type healthMsg struct {
	health   store.Health
	dbSize   int64
	walSize  int64
	projects []store.ProjectStats
	err      error
}

func (m Model) healthCmd() tea.Cmd {
	st := m.store
	return func() tea.Msg {
		h, err := st.Health()
		if err != nil {
			return healthMsg{err: err}
		}
		dbSize, walSize, err := st.DBSize()
		if err != nil {
			return healthMsg{err: err}
		}
		projects, err := st.Projects()
		if err != nil {
			return healthMsg{err: err}
		}
		return healthMsg{health: h, dbSize: dbSize, walSize: walSize, projects: projects}
	}
}

// loadHealthCmd switches to the metrics screen and fetches the snapshot.
func (m Model) loadHealthCmd() (tea.Model, tea.Cmd) {
	m.view = viewHealth
	m.status = ""
	m.healthErr = nil
	return m, m.healthCmd()
}

// concern is one thing worth knowing about the store.
type concern struct {
	when  bool
	label string
	why   string
}

func concerns(h store.Health) []concern {
	return []concern{
		{
			when:  h.AmbiguousSessions > 0,
			label: fmt.Sprintf("%d sessions share a project with another active session", h.AmbiguousSessions),
			why:   "writes without an explicit session_id fail closed — `ambiguous_active_runtime_sessions` in doctor",
		},
		{
			when:  h.ProjectsWithoutSession > 0,
			label: fmt.Sprintf("%d project names have memories but no owning session", h.ProjectsWithoutSession),
			why:   "project names come from sessions; these memories landed under unowned names",
		},
		{
			when:  h.OrphanSessions > 0,
			label: fmt.Sprintf("%d live memories point at a session that does not exist", h.OrphanSessions),
			why:   "the session was deleted or renamed out from under them",
		},
		{
			when:  h.ExpiringSoon > 0,
			label: fmt.Sprintf("%d memories expire within %d days", h.ExpiringSoon, int(store.ExpiringWindow.Hours()/24)),
			why:   "they stop being recalled once expires_at passes",
		},
		{
			when:  h.DeletedCount > 0,
			label: fmt.Sprintf("%d soft-deleted memories are still in the database", h.DeletedCount),
			why:   "press P in the list to see them, or delete permanently",
		},
	}
}

func (m Model) healthView() string {
	if m.health == nil {
		if m.healthErr != nil {
			return "\n  " + styleError.Render("error: "+m.healthErr.Error()) + "\n"
		}
		return "\n  " + styleDim.Render("reading metrics…") + "\n"
	}
	h := m.health
	var b strings.Builder

	// Top Store Card
	var storeCard strings.Builder
	storeCard.WriteString(styleTitle.Render("DATABASE & STORE") + "\n\n")
	rows := [][2]string{
		{"memories", fmt.Sprintf("%d live (%d total)", h.LiveObservations, h.TotalObservations)},
		{"sessions", fmt.Sprintf("%d (%d active)", h.TotalSessions, h.ActiveSessions)},
		{"prompts", fmt.Sprint(h.TotalPrompts)},
		{"projects", fmt.Sprint(h.Projects)},
		{"database", fmt.Sprintf("%s (wal: %s)", humanSize(m.dbSize), humanSize(m.walSize))},
		{"pinned / dupes", fmt.Sprintf("%d pinned · %d duplicate rows", h.PinnedCount, h.DuplicateCount)},
	}
	for _, r := range rows {
		storeCard.WriteString(fmt.Sprintf("  %s %s\n", styleDim.Render(pad(r[0], 18)), styleMuted.Render(r[1])))
	}

	if m.walSize > m.dbSize {
		storeCard.WriteString("\n  " + styleWarning.Render("notice: WAL > DB size; pending unmerged writes exist") + "\n")
	}

	boxStore := styleBoxActive.Width(clamp(m.width-4, 40, 90)).Render(storeCard.String())
	b.WriteString("\n" + boxStore + "\n")

	// Worth Knowing / Warnings Card
	var warnCard strings.Builder
	warnCard.WriteString(styleAccent.Render("HEALTH & WARNINGS") + "\n\n")
	hasConcern := false
	for _, c := range concerns(*h) {
		if !c.when {
			continue
		}
		hasConcern = true
		warnCard.WriteString(fmt.Sprintf("  %s %s\n", styleWarning.Render("⚑"), c.label))
		warnCard.WriteString(fmt.Sprintf("    %s\n", styleDim.Render(c.why)))
	}
	if !hasConcern {
		warnCard.WriteString("  " + styleSuccess.Render("✔ All clean. No operational drifts detected.") + "\n")
	}

	boxWarn := styleBoxInactive.Width(clamp(m.width-4, 40, 90)).Render(warnCard.String())
	b.WriteString(boxWarn + "\n")

	// Projects breakdown
	if len(m.projects) > 0 {
		var projCard strings.Builder
		projCard.WriteString(styleTitle.Render("TOP PROJECTS") + "\n\n")
		shown := m.projects
		if len(shown) > 6 {
			shown = shown[:6]
		}
		for _, p := range shown {
			projCard.WriteString(fmt.Sprintf("  %s  %s  %s\n",
				pad(trunc(p.Name, 26), 26),
				styleBadge.Render(fmt.Sprintf("%4d obs", p.Observations)),
				styleDim.Render(fmt.Sprintf("%3d ses", p.Sessions))))
		}
		if len(m.projects) > len(shown) {
			projCard.WriteString(styleDim.Render(fmt.Sprintf("\n  … and %d more projects", len(m.projects)-len(shown))))
		}
		boxProj := styleBoxInactive.Width(clamp(m.width-4, 40, 90)).Render(projCard.String())
		b.WriteString(boxProj)
	}

	return b.String()
}

// humanSize formats bytes into readable units.
func humanSize(n int64) string {
	const unit = 1024
	if n < unit {
		return fmt.Sprintf("%d B", n)
	}
	div, exp := int64(unit), 0
	for v := n / unit; v >= unit; v /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %ciB", float64(n)/float64(div), "KMGTPE"[exp])
}

var _ = time.Now
