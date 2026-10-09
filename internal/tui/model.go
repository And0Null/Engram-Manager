// Package tui implements the terminal UI.
//
// This first slice is read-only: list, filter, search, and detail. Write
// commands land on top of the same model once the navigation is proven out.
package tui

import (
	"fmt"
	"time"

	"github.com/charmbracelet/bubbles/textarea"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"

	"engram-manager/internal/api"
	"engram-manager/internal/store"
)

const pageSize = 200

type view int

const (
	viewList view = iota
	viewDetail
	viewSearch
	viewError
	viewEdit
	viewConfirm
	viewHealth
	viewSessions
)

// Model is the root Bubbletea model.
type Model struct {
	store  *store.Store
	api    *api.Client
	filter store.Filter
	total  int

	items    []store.Observation
	cursor   int
	view     view
	search   string
	status   string
	err      error
	loading  bool
	loadedAt time.Time

	detail    store.Observation
	versions  []store.Version
	relations []store.Relation
	timeline  []store.Observation

	searching bool
	width     int
	height    int

	// Write path. title and body are the edit buffers; pending holds the
	// observation a destructive command is about to touch, so confirmation
	// shows exactly what will be affected.
	title       textinput.Model
	body        textarea.Model
	pending     *store.Observation
	pendingHard bool
	editing     store.Observation
	apiReady    bool

	// Metrics screen.
	health    *store.Health
	projects  []store.ProjectStats
	dbSize    int64
	walSize   int64
	healthErr error

	// Session panel.
	sessions       []store.Session
	sessionCursor  int
	sessionProject string
	sessionsErr    error
}

// New builds the root model. The store is read-only; the API client is only
// used by write commands, and its absence is not an error here.
func New(st *store.Store, cl *api.Client) Model {
	title := textinput.New()
	title.Prompt = "title: "
	title.CharLimit = 200

	body := textarea.New()
	body.Placeholder = "content"

	return Model{
		store:  st,
		api:    cl,
		filter: store.Filter{Limit: pageSize},
		// Loading is its own field rather than a status string: a status
		// string can be left behind and then rendered next to unrelated
		// screens, where it reads as if they are still loading.
		loading:  true,
		title:    title,
		body:     body,
		apiReady: cl != nil,
	}
}

// Init performs the first load.
func (m Model) Init() tea.Cmd { return m.loadCmd() }

// loadedMsg carries one page of results back to the model.
type loadedMsg struct {
	items []store.Observation
	total int
	err   error
}

// loadCmd reads the page in a Bubbletea command so the UI never blocks on
// SQLite while another Engram write holds the WAL.
func (m Model) loadCmd() tea.Cmd {
	st, f := m.store, m.filter
	return func() tea.Msg {
		items, err := st.List(f)
		if err != nil {
			return loadedMsg{err: err}
		}
		total, err := st.Count(f)
		return loadedMsg{items: items, total: total, err: err}
	}
}

// Run starts the program.
func Run(st *store.Store, cl *api.Client) error {
	p := tea.NewProgram(New(st, cl), tea.WithAltScreen())
	_, err := p.Run()
	return err
}

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		return m, nil

	case loadedMsg:
		m.loading = false
		if msg.err != nil {
			m.err, m.view = msg.err, viewError
			return m, nil
		}
		m.items, m.total = msg.items, msg.total
		m.cursor = clamp(m.cursor, 0, len(m.items)-1)
		m.loadedAt = time.Now()
		m.loading = false
		m.status = ""
		return m, nil

	case detailMsg:
		if msg.err != nil {
			m.err, m.view = msg.err, viewError
			return m, nil
		}
		m.versions, m.relations, m.timeline = msg.versions, msg.relations, msg.timeline
		return m, nil

	case writeMsg:
		return m.handleWriteMsg(msg)

	case healthMsg:
		if msg.err != nil {
			m.healthErr = msg.err
			return m, nil
		}
		m.healthErr = nil
		m.health, m.projects = &msg.health, msg.projects
		m.dbSize, m.walSize = msg.dbSize, msg.walSize
		return m, nil

	case sessionsMsg:
		if msg.err != nil {
			m.sessionsErr = msg.err
			return m, nil
		}
		m.sessionsErr = nil
		m.sessions = msg.sessions
		m.sessionCursor = clamp(m.sessionCursor, 0, len(m.sessions)-1)
		return m, nil

	case reloadedMsg:
		return m.applyReloaded(msg)

	// The embedded widgets are fed tea.KeyMsg from handleEditKeys, because
	// bubbles v1 exposes no widget-specific key message type to switch on.

	case tea.KeyMsg:
		return m.handleKey(msg)
	}
	return m, nil
}

func (m Model) handleKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	// The modal views own the keyboard entirely: a stray key must never fall
	// through to the list and move the cursor behind a confirmation dialog.
	switch m.view {
	case viewEdit:
		return m.handleEditKeys(msg)
	case viewConfirm:
		return m.handleConfirmKeys(msg)
	}

	// The metrics screen is read-only, so only refresh and exit apply.
	if m.view == viewHealth {
		switch msg.String() {
		case "esc", "q", "m":
			m.view = viewList
			return m, nil
		case "r":
			return m, m.healthCmd()
		}
		return m, nil
	}

	if m.view == viewSessions {
		switch msg.String() {
		case "esc", "q", "s":
			m.view = viewList
			return m, nil
		case "r":
			return m, m.sessionsCmd()
		case "up", "k":
			if m.sessionCursor > 0 {
				m.sessionCursor--
			}
			return m, nil
		case "down", "j":
			if m.sessionCursor < len(m.sessions)-1 {
				m.sessionCursor++
			}
			return m, nil
		}
		return m, nil
	}

	if m.searching {
		switch msg.Type {
		case tea.KeyEnter:
			m.searching = false
			return m.applySearch(m.search)
		case tea.KeyEsc:
			m.searching = false
			m.search = ""
			return m.applySearch("")
		case tea.KeyBackspace:
			if n := len(m.search); n > 0 {
				m.search = m.search[:n-1]
			}
			return m, nil
		case tea.KeyRunes, tea.KeySpace:
			m.search += string(msg.Runes)
			if msg.Type == tea.KeySpace {
				m.search += " "
			}
			return m, nil
		}
		return m, nil
	}

	switch msg.String() {
	case "q", "ctrl+c":
		return m, tea.Quit

	case "esc":
		if m.view == viewDetail {
			m.view = viewList
			return m, nil
		}
		m.filter = store.Filter{Limit: pageSize}
		return m.reload()

	case "/":
		m.searching = true
		m.search = m.filter.Query
		return m, nil

	case "enter":
		if m.view == viewList && len(m.items) > 0 {
			return m.openDetail()
		}
		return m, nil

	case "o":
		if m.view == viewDetail {
			return m.openDetail()
		}
		return m, nil

	case "up", "k":
		if m.view == viewList && m.cursor > 0 {
			m.cursor--
		}
		return m, nil

	case "down", "j":
		if m.view == viewList && m.cursor < len(m.items)-1 {
			m.cursor++
		}
		return m, nil

	case "g":
		if m.view == viewList {
			m.cursor = 0
		}
		return m, nil

	case "G":
		if m.view == viewList {
			m.cursor = len(m.items) - 1
		}
		return m, nil

	case "m":
		return m.loadHealthCmd()

	case "s":
		return m.openSessions()

	case "P":
		m.filter.IncludeDeleted = !m.filter.IncludeDeleted
		m.status = fmt.Sprintf("deleted memories: %s", yesNo(m.filter.IncludeDeleted))
		return m.reload()

	case "?":
		return m, nil
	}

	// Write keys only exist in the detail view, where there is a specific
	// observation to act on.
	if m.view == viewDetail {
		if next, cmd, handled := m.handleDetailWriteKeys(msg); handled {
			return next, cmd
		}
	}
	return m, nil
}

func (m Model) applySearch(q string) (tea.Model, tea.Cmd) {
	m.filter.Query = q
	m.cursor = 0
	m.status = "searching…"
	return m.reload()
}

func (m Model) reload() (tea.Model, tea.Cmd) {
	m.loading = true
	m.status = "loading…"
	return m, m.loadCmd()
}

func (m Model) openDetail() (tea.Model, tea.Cmd) {
	if m.cursor < 0 || m.cursor >= len(m.items) {
		return m, nil
	}
	o, err := m.store.Get(m.items[m.cursor].ID)
	if err != nil {
		m.err, m.view = err, viewError
		return m, nil
	}
	m.detail = o
	m.view = viewDetail
	return m, loadDetailCmd(m.store, o.ID)
}

type detailMsg struct {
	versions  []store.Version
	relations []store.Relation
	timeline  []store.Observation
	err       error
}

func loadDetailCmd(st *store.Store, id int64) tea.Cmd {
	return func() tea.Msg {
		v, err := st.Versions(id)
		if err != nil {
			return detailMsg{err: err}
		}
		r, err := st.RelationsFor(id)
		if err != nil {
			return detailMsg{err: err}
		}
		tl, err := st.Timeline(id, 5, 5)
		if err != nil {
			return detailMsg{err: err}
		}
		return detailMsg{versions: v, relations: r, timeline: tl}
	}
}

func clamp(v, lo, hi int) int {
	if hi < lo {
		return lo
	}
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}

func yesNo(b bool) string {
	if b {
		return "included"
	}
	return "hidden"
}
