package tui

import (
	"context"
	"fmt"

	tea "github.com/charmbracelet/bubbletea"

	"engram-manager/internal/api"
	"engram-manager/internal/store"
)

// This file holds every write path. Reads live in internal/store; writes all go
// through internal/api so Engram's own soft-delete, dedupe, and project rules
// stay in charge. Nothing here writes to SQLite.

// writeMsg reports the outcome of one write command.
type writeMsg struct {
	op   string
	note string
	hard bool
	err  error
}

const noServer = "Engram server unreachable — start it with `engram serve`"

// handleConfirmKeys owns the keyboard while a destructive command waits for
// an answer. The default is "no": only an explicit y/enter proceeds.
func (m Model) handleConfirmKeys(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "y", "Y", "enter":
		if m.pending == nil {
			return m.cancelPending(), nil
		}
		m.status = "working…"
		return m, m.deleteCmd()
	case "n", "N", "esc", "q":
		return m.cancelPending(), nil
	}
	return m, nil
}

// handleEditKeys owns the keyboard while an observation is being edited.
// ctrl+s saves; esc discards, and discarding is always available so a bad edit
// can never trap the user in the editor.
func (m Model) handleEditKeys(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.Type {
	case tea.KeyCtrlS:
		m.status = "saving…"
		return m, m.saveCmd()
	case tea.KeyEsc:
		m.view = viewDetail
		m.status = "discarded — nothing was changed"
		return m, nil
	}

	var cmd tea.Cmd
	if m.title.Focused() {
		m.title, cmd = m.title.Update(msg)
		// Tab hands the keyboard to the body; it does not insert a tab.
		if msg.String() == "tab" {
			m.title.Blur()
			m.body.Focus()
			return m, nil
		}
		return m, cmd
	}
	m.body, cmd = m.body.Update(msg)
	return m, cmd
}

// cancelEdit returns to the detail view without saving.
func (m Model) cancelEdit() Model {
	m.view = viewDetail
	m.status = "discarded — nothing was changed"
	return m
}

// handleDetailWriteKeys processes the write keys available in the detail view.
// It returns handled=false when the key is not a write command.
func (m Model) handleDetailWriteKeys(msg tea.KeyMsg) (tea.Model, tea.Cmd, bool) {
	if m.api == nil {
		return m, nil, false
	}
	switch msg.String() {
	case "e":
		return m.startEdit(), nil, true
	case "d":
		// Lowercase is the recoverable path, on purpose.
		return m.confirmDelete(false), nil, true
	case "D":
		return m.confirmDelete(true), nil, true
	case "p":
		return m, m.pinToggle(), true
	case "r":
		return m, m.markReviewed(), true
	}
	return m, nil, false
}

// startEdit copies the observation into the edit buffers.
func (m Model) startEdit() Model {
	o := m.detail
	m.editing = o
	m.title.SetValue(o.Title)
	m.body.SetValue(o.Content)
	m.title.Focus()
	m.view = viewEdit
	return m
}

func (m Model) confirmDelete(hard bool) Model {
	o := m.detail
	m.pending = &o
	m.pendingHard = hard
	m.view = viewConfirm
	return m
}

// cancelPending leaves a confirmation without touching anything.
func (m Model) cancelPending() Model {
	m.pending = nil
	m.pendingHard = false
	m.view = viewDetail
	m.status = "cancelled — nothing was changed"
	return m
}

// writeCmd runs one API call off the UI thread and reports the result.
func writeCmd(cl *api.Client, op string, fn func(context.Context) (string, error)) tea.Cmd {
	return func() tea.Msg {
		ctx := context.Background()
		note, err := fn(ctx)
		return writeMsg{op: op, note: note, err: err}
	}
}

// deleteCmd soft- or hard-deletes the pending observation.
func (m Model) deleteCmd() tea.Cmd {
	if m.pending == nil {
		return nil
	}
	id, project, hard := m.pending.ID, m.pending.Project, m.pendingHard
	return writeCmd(m.api, "delete", func(ctx context.Context) (string, error) {
		if err := m.api.Delete(ctx, id, project, hard); err != nil {
			return "", err
		}
		if hard {
			return fmt.Sprintf("permanently deleted #%d", id), nil
		}
		return fmt.Sprintf("soft-deleted #%d — recoverable, press P to show", id), nil
	})
}

// saveCmd patches the edited observation.
//
// Title and content are compared against the loaded values so an untouched
// field is never sent: Engram rejects a PATCH that clears a title, and sending
// an unchanged field is noise it would have to store as a new revision.
func (m Model) saveCmd() tea.Cmd {
	if m.api == nil {
		return nil
	}
	o := m.editing
	in := api.UpdateInput{ExpectedProject: o.Project}
	if t := m.title.Value(); t != "" && t != o.Title {
		in.Title = t
	}
	if b := m.body.Value(); b != o.Content {
		in.Content = b
	}
	if in.Title == "" && in.Content == "" {
		return func() tea.Msg {
			return writeMsg{op: "save", err: fmt.Errorf("nothing changed")}
		}
	}
	return writeCmd(m.api, "save", func(ctx context.Context) (string, error) {
		if err := m.api.Update(ctx, o.ID, in); err != nil {
			return "", err
		}
		return fmt.Sprintf("saved #%d", o.ID), nil
	})
}

// pinCmd flips the pinned flag, sending the inverse of what is stored.
func (m Model) pinToggle() tea.Cmd {
	o := m.detail
	target := !o.Pinned
	return writeCmd(m.api, "pin", func(ctx context.Context) (string, error) {
		if err := m.api.SetPinned(ctx, o.ID, target); err != nil {
			return "", err
		}
		if target {
			return fmt.Sprintf("pinned #%d", o.ID), nil
		}
		return fmt.Sprintf("unpinned #%d", o.ID), nil
	})
}

// markReviewedCmd closes the review loop the same way mem_review does.
func (m Model) markReviewed() tea.Cmd {
	o := m.detail
	return writeCmd(m.api, "review", func(ctx context.Context) (string, error) {
		if err := m.api.MarkReviewed(ctx, o.ID); err != nil {
			return "", err
		}
		return fmt.Sprintf("marked #%d reviewed", o.ID), nil
	})
}

// handleWriteMsg folds a write result back into the model.
//
// On success the observation is re-read from SQLite rather than patched in
// place: Engram may have created a new revision, bumped revision_count, or
// rewritten the row, and a locally patched copy would lie about that.
func (m Model) handleWriteMsg(msg writeMsg) (tea.Model, tea.Cmd) {
	if msg.err != nil {
		m.status = msg.op + " failed: " + msg.err.Error()
		// Stay on the editing screen after a failed save so the typed content
		// is not lost.
		if m.view == viewEdit || m.view == viewConfirm {
			return m, nil
		}
		return m, nil
	}

	m.status = msg.note
	m.view = viewDetail
	m.pending = nil
	m.pendingHard = false

	id := m.detail.ID
	cmds := []tea.Cmd{func() tea.Msg {
		fresh, err := m.store.Get(id)
		if err != nil {
			return detailMsg{err: err}
		}
		return reloadedMsg{id: id, obs: fresh}
	}}
	return m, tea.Batch(cmds...)
}

// reloadedMsg carries one freshly re-read observation back into the list and
// the detail pane at the same time.
type reloadedMsg struct {
	id  int64
	obs store.Observation
}

func (m Model) applyReloaded(msg reloadedMsg) (tea.Model, tea.Cmd) {
	for i := range m.items {
		if m.items[i].ID == msg.id {
			m.items[i] = msg.obs
			break
		}
	}
	if m.detail.ID == msg.id {
		m.detail = msg.obs
		m.versions, _ = m.store.Versions(msg.id)
		m.relations, _ = m.store.RelationsFor(msg.id)
	}
	// A soft-deleted observation drops out of the current list, so the row
	// count has to be recomputed instead of assumed.
	return m, m.loadCmd()
}
