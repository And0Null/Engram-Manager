package tui

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"

	"engram-manager/internal/store"
)

// sessionsMsg carries the session list into the model.
type sessionsMsg struct {
	sessions []store.Session
	err      error
}

func (m Model) sessionsCmd() tea.Cmd {
	st, project := m.store, m.sessionProject
	return func() tea.Msg {
		s, err := st.Sessions(project, true, 300)
		if err != nil {
			return sessionsMsg{err: err}
		}
		return sessionsMsg{sessions: s}
	}
}

// openSessions switches to the session panel and loads it.
func (m Model) openSessions() (tea.Model, tea.Cmd) {
	m.view = viewSessions
	m.status = ""
	m.sessionsErr = nil
	return m, m.sessionsCmd()
}

// sessionConcern explains why one session deserves attention.
func sessionConcern(s store.Session) string {
	if !s.Active() {
		return ""
	}
	if s.ObservationCount > 0 && s.OwnershipMode == "" {
		return "no ownership metadata"
	}
	if s.RuntimeLeaseExpiresAt != "" {
		return "holds a runtime lease"
	}
	return ""
}

func (m Model) sessionsView() string {
	if m.sessionsErr != nil {
		return "\n  " + styleError.Render("error: "+m.sessionsErr.Error()) + "\n"
	}
	if m.sessions == nil {
		return "\n  " + styleDim.Render("loading sessions…") + "\n"
	}
	if len(m.sessions) == 0 {
		return "\n  no sessions match.\n"
	}

	var active, ended int
	for _, s := range m.sessions {
		if s.Active() {
			active++
		} else {
			ended++
		}
	}

	header := fmt.Sprintf("%s   %s   %s",
		styleTitle.Render("ACTIVE SESSIONS & DRIFT"),
		styleBadge.Render(fmt.Sprintf("%d active", active)),
		styleBadgeMuted.Render(fmt.Sprintf("%d ended", ended)))

	visible := clamp(m.height-8, 5, 30)
	if visible > len(m.sessions) {
		visible = len(m.sessions)
	}
	start := clamp(m.sessionCursor-visible/2, 0, len(m.sessions)-visible)
	end := min(start+visible, len(m.sessions))

	var list strings.Builder
	for i := start; i < end; i++ {
		list.WriteString(m.sessionRow(i))
		if i < end-1 {
			list.WriteString("\n")
		}
	}

	box := styleBoxActive.
		Width(clamp(m.width-4, 50, 110)).
		Render(header + "\n\n" + list.String())

	return "\n" + box
}

func (m Model) sessionRow(i int) string {
	s := m.sessions[i]

	state := styleDim.Render("○")
	if s.Active() {
		state = styleSuccess.Render("●")
	}

	nameWidth := clamp(m.width/4, 14, 26)
	obs := fmt.Sprintf("%d obs", s.ObservationCount)
	dirWidth := clamp(m.width-nameWidth-len(obs)-24, 16, 60)

	line := fmt.Sprintf(" %s %s %s %s",
		state,
		styleTitle.Render(pad(trunc(s.Project, nameWidth), nameWidth)),
		styleBadgeMuted.Render(pad(obs, 8)),
		styleDim.Render(trunc(oneLine(s.Directory), dirWidth)))

	if c := sessionConcern(s); c != "" {
		line += " " + styleWarning.Render("⚑ "+c)
	}

	if i == m.sessionCursor {
		return styleRowSelected.Render(line)
	}
	return line
}

// filterSessionsBy narrows the panel to one project.
func (m Model) filterSessionsBy(project string) tea.Cmd {
	m.sessionProject = project
	return m.sessionsCmd()
}
