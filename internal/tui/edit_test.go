package tui

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"engram-manager/internal/api"
	"engram-manager/internal/store"
)

// fakeAPI stands in for `engram serve`. No test here touches the real store.
type fakeAPI struct {
	srv    *httptest.Server
	calls  []string
	bodies []string
}

func newFakeAPI(t *testing.T) *fakeAPI {
	t.Helper()
	f := &fakeAPI{}
	f.srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		f.calls = append(f.calls, r.Method+" "+r.URL.Path)
		f.bodies = append(f.bodies, string(body))
		w.Header().Set("Content-Type", "application/json")
		if r.Method == http.MethodPost && strings.Contains(r.URL.Path, "mark_reviewed") {
			io.WriteString(w, `{}`)
			return
		}
		io.WriteString(w, `{"id":1}`)
	}))
	t.Cleanup(f.srv.Close)
	return f
}

func (f *fakeAPI) client() *api.Client {
	return api.New(strings.TrimPrefix(f.srv.URL, "http://"))
}

// detailModel returns a model sitting in the detail view on a real memory.
func detailModel(t *testing.T, withAPI bool) (Model, store.Observation) {
	t.Helper()
	st := realStore(t)
	items, err := st.List(store.Filter{Limit: 50})
	if err != nil || len(items) == 0 {
		t.Skip("no memories available")
	}
	o, err := st.Get(items[0].ID)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	m := New(st, nil)
	if withAPI {
		m.api = newFakeAPI(t).client()
		m.apiReady = true
	}
	m.width, m.height = 120, 30
	m.items = items
	m.view = viewDetail
	m.detail = o
	return m, o
}

func press(m Model, s string) (Model, tea.Cmd) {
	next, cmd := m.Update(tea.KeyMsg{Type: keyType(s), Runes: []rune(s)})
	return next.(Model), cmd
}

func keyType(s string) tea.KeyType {
	switch s {
	case "enter":
		return tea.KeyEnter
	case "esc":
		return tea.KeyEsc
	case "ctrl+s":
		return tea.KeyCtrlS
	case "tab":
		return tea.KeyTab
	default:
		return tea.KeyRunes
	}
}

// drain runs a command and unwraps any batch, returning every message it
// produced. Bubbletea batches the widget's cursor blink in with the real
// command, so the write message has to be looked for rather than assumed to be
// the only one.
func drain(cmd tea.Cmd) []tea.Msg {
	if cmd == nil {
		return nil
	}
	msg := cmd()
	if batch, ok := msg.(tea.BatchMsg); ok {
		var out []tea.Msg
		for _, c := range batch {
			out = append(out, drain(c)...)
		}
		return out
	}
	return []tea.Msg{msg}
}

// firstWrite finds the writeMsg in a command's output.
func firstWrite(t *testing.T, cmd tea.Cmd) writeMsg {
	t.Helper()
	for _, msg := range drain(cmd) {
		if wm, ok := msg.(writeMsg); ok {
			return wm
		}
	}
	t.Fatal("no writeMsg in the command output")
	return writeMsg{}
}

func TestDeleteKeyOpensConfirmationAndSendsNothing(t *testing.T) {
	f := newFakeAPI(t)
	m, _ := detailModel(t, false)
	m.api, m.apiReady = f.client(), true

	next, cmd := press(m, "d")
	m = next

	if m.view != viewConfirm {
		t.Fatal("d did not open the confirmation")
	}
	if cmd != nil {
		t.Error("opening the confirmation must not issue a request")
	}
	if len(f.calls) != 0 {
		t.Errorf("a request was sent before confirming: %v", f.calls)
	}
}

func TestConfirmSoftDeleteSendsExpectedProject(t *testing.T) {
	f := newFakeAPI(t)
	m, o := detailModel(t, false)
	m.api, m.apiReady = f.client(), true

	next, _ := press(m, "d")
	m = next
	next, cmd := press(m, "y")
	if cmd == nil {
		t.Fatal("confirming produced no command")
	}
	msg := cmd()
	wm, ok := msg.(writeMsg)
	if !ok {
		t.Fatalf("unexpected message %T", msg)
	}
	if wm.err != nil {
		t.Fatalf("delete failed: %v", wm.err)
	}
	if len(f.calls) != 1 || !strings.HasPrefix(f.calls[0], "DELETE /observations/") {
		t.Fatalf("calls = %v", f.calls)
	}
	// Engram rejects a DELETE without expected_project.
	if !strings.Contains(f.bodies[0], o.Project) {
		t.Errorf("expected_project not sent: %s", f.bodies[0])
	}
	_ = next
}

func TestHardDeleteIsDistinctFromSoft(t *testing.T) {
	f := newFakeAPI(t)
	m, _ := detailModel(t, false)
	m.api, m.apiReady = f.client(), true

	next, _ := press(m, "D")
	m = next
	if !m.pendingHard {
		t.Error("D did not mark the delete as hard")
	}
	if !strings.Contains(m.confirmView(), "cannot be undone") {
		t.Error("hard delete must say it is irreversible")
	}
}

func TestSoftDeleteIsPresentedAsRecoverable(t *testing.T) {
	m, _ := detailModel(t, false)
	m.api, m.apiReady = newFakeAPI(t).client(), true

	next, _ := press(m, "d")
	m = next
	view := m.confirmView()
	if !strings.Contains(view, "can be restored") {
		t.Errorf("soft delete should say it is recoverable:\n%s", view)
	}
}

func TestCancellingConfirmationChangesNothing(t *testing.T) {
	for _, key := range []string{"n", "esc", "q"} {
		f := newFakeAPI(t)
		m, _ := detailModel(t, false)
		m.api, m.apiReady = f.client(), true

		next, _ := press(m, "d")
		m = next
		next, _ = press(m, key)
		m = next

		if m.view != viewDetail {
			t.Errorf("%q left the confirmation open (view=%v)", key, m.view)
		}
		if len(f.calls) != 0 {
			t.Errorf("%q sent a request: %v", key, f.calls)
		}
	}
}

// While a confirmation is open, list keys must not reach the list: silently
// moving the cursor under a dialog is how the wrong memory gets deleted.
func TestListKeysAreInertDuringConfirmation(t *testing.T) {
	m, _ := detailModel(t, false)
	m.api, m.apiReady = newFakeAPI(t).client(), true
	m.cursor = 3

	next, _ := press(m, "d")
	m = next
	next, _ = press(m, "j")
	m = next

	if m.cursor != 3 {
		t.Errorf("cursor moved to %d during the confirmation", m.cursor)
	}
	if m.view != viewConfirm {
		t.Error("a stray key dismissed the confirmation")
	}
}

func TestSaveSendsOnlyChangedFields(t *testing.T) {
	f := newFakeAPI(t)
	m, _ := detailModel(t, false)
	m.api, m.apiReady = f.client(), true

	next, _ := press(m, "e")
	m = next
	if m.view != viewEdit {
		t.Fatal("e did not open the editor")
	}
	if m.title.Value() != m.editing.Title {
		t.Error("the title buffer was not seeded with the current title")
	}

	m.body.SetValue("edited body")
	_, cmd := press(m, "ctrl+s")
	if cmd == nil {
		t.Fatal("ctrl+s produced no command")
	}
	if wm := firstWrite(t, cmd); wm.err != nil {
		t.Fatalf("save failed: %v", wm.err)
	}

	if len(f.calls) != 1 || !strings.HasPrefix(f.calls[0], "PATCH") {
		t.Fatalf("calls = %v", f.calls)
	}
	body := f.bodies[0]
	if !strings.Contains(body, `"content":"edited body"`) {
		t.Errorf("changed content not sent: %s", body)
	}
	// An unchanged title must not be sent: Engram treats a blank title as
	// "clear the title", and a redundant field creates a needless revision.
	if strings.Contains(body, `"title"`) {
		t.Errorf("unchanged title was sent anyway: %s", body)
	}
}

func TestEscapingTheEditorSendsNothing(t *testing.T) {
	f := newFakeAPI(t)
	m, _ := detailModel(t, false)
	m.api, m.apiReady = f.client(), true

	next, _ := press(m, "e")
	m = next
	m.body.SetValue("discarded work")
	next, cmd := press(m, "esc")
	m = next

	if cmd != nil {
		t.Error("discarding the edit issued a command")
	}
	if len(f.calls) != 0 {
		t.Errorf("discarding sent a request: %v", f.calls)
	}
	if m.view != viewDetail {
		t.Errorf("esc left the editor (view=%v)", m.view)
	}
}

func TestSaveWithNoChangesDoesNotCallTheAPI(t *testing.T) {
	f := newFakeAPI(t)
	m, _ := detailModel(t, false)
	m.api, m.apiReady = f.client(), true

	next, _ := press(m, "e")
	m = next
	next, cmd := press(m, "ctrl+s")
	m = next

	if wm := firstWrite(t, cmd); wm.err == nil {
		t.Error("expected a no-change error rather than a write")
	}
	if len(f.calls) != 0 {
		t.Errorf("a no-op save still called the API: %v", f.calls)
	}
	_ = m
}

// A failed write must keep the edit on screen: losing typed content because
// the server was down is the worst outcome here.
func TestFailedSaveKeepsTheEditorOpen(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		io.WriteString(w, `{"error":"expected_project must be a valid non-empty project name"}`)
	}))
	defer srv.Close()

	st := realStore(t)
	items, _ := st.List(store.Filter{Limit: 1})
	if len(items) == 0 {
		t.Skip("no memories")
	}
	m := New(st, api.New(strings.TrimPrefix(srv.URL, "http://")))
	m.width, m.height = 100, 30
	m.items = items
	m.view = viewEdit
	m.editing = items[0]
	m.body.SetValue("work in progress")

	m2, cmd := m.Update(writeMsg{op: "save", err: errString("expected_project must be a valid")})
	got := m2.(Model)
	if cmd != nil {
		t.Error("a failed write should not trigger a reload")
	}
	if got.view != viewEdit {
		t.Error("the editor closed on a failed save")
	}
	if !strings.Contains(got.status, "failed") {
		t.Errorf("status does not report the failure: %q", got.status)
	}
	if !strings.Contains(got.body.Value(), "work in progress") {
		t.Error("the typed content was lost after a failed save")
	}
}

func TestPinToggleSendsInverseOfStoredFlag(t *testing.T) {
	f := newFakeAPI(t)
	m, o := detailModel(t, false)
	m.api, m.apiReady = f.client(), true

	next, cmd := press(m, "p")
	m = next
	if cmd == nil {
		t.Fatal("p produced no command")
	}
	if wm := firstWrite(t, cmd); wm.err != nil {
		t.Fatalf("pin failed: %v", wm.err)
	}
	if len(f.calls) != 1 {
		t.Fatalf("calls = %v", f.calls)
	}
	wantVerb := "PUT"
	if o.Pinned {
		wantVerb = "DELETE"
	}
	if !strings.HasPrefix(f.calls[0], wantVerb) {
		t.Errorf("pinned=%v so %s was expected, got %s", o.Pinned, wantVerb, f.calls[0])
	}
}

func TestMarkReviewed(t *testing.T) {
	f := newFakeAPI(t)
	m, _ := detailModel(t, false)
	m.api, m.apiReady = f.client(), true

	next, cmd := press(m, "r")
	m = next
	if cmd == nil {
		t.Fatal("r produced no command")
	}
	if wm := firstWrite(t, cmd); wm.err != nil {
		t.Fatalf("review failed: %v", wm.err)
	}
	if !strings.Contains(f.calls[0], "mark_reviewed") {
		t.Errorf("calls = %v", f.calls)
	}
	_ = m
}

// With no API client the manager is read-only and must not crash or pretend.
func TestWriteKeysAreInertWithoutAnAPI(t *testing.T) {
	m, _ := detailModel(t, false)
	if m.api != nil {
		t.Fatal("test setup: expected a nil API client")
	}
	for _, key := range []string{"e", "d", "D", "p", "r"} {
		next, cmd := press(m, key)
		got := next
		if cmd != nil {
			t.Errorf("%q issued a command with no API client", key)
		}
		if got.view != viewDetail {
			t.Errorf("%q changed the view with no API client", key)
		}
	}
}

// After a successful write the row is re-read, not patched in place: Engram
// bumps revision_count and may rewrite the row underneath us.
func TestSuccessfulWriteTriggersAReload(t *testing.T) {
	m, o := detailModel(t, false)
	m.api, m.apiReady = newFakeAPI(t).client(), true
	m.detail.Pinned = true

	next, cmd := m.Update(writeMsg{op: "pin", note: "pinned"})
	got := next
	if cmd == nil {
		t.Fatal("a successful write must schedule a reload")
	}
	msg := cmd()
	switch msg.(type) {
	case reloadedMsg:
		if msg.(reloadedMsg).id != o.ID {
			t.Errorf("reloaded the wrong observation: %d", msg.(reloadedMsg).id)
		}
	default:
		t.Errorf("unexpected message %T", msg)
	}
	_ = got
}

func TestApplyReloadedReplacesTheRowInPlace(t *testing.T) {
	m, o := detailModel(t, false)
	o.Pinned = true
	o.RevisionCount = 3

	next, _ := m.Update(reloadedMsg{id: o.ID, obs: o})
	got := next.(Model)
	if !got.detail.Pinned {
		t.Error("the detail pane kept the stale pinned flag")
	}
	if got.detail.RevisionCount != 3 {
		t.Errorf("revision_count = %d, want 3", got.detail.RevisionCount)
	}
	found := false
	for _, it := range got.items {
		if it.ID == o.ID {
			found = true
			if !it.Pinned {
				t.Error("the list row kept the stale pinned flag")
			}
		}
	}
	if !found {
		t.Error("the reloaded observation is missing from the list")
	}
}

type errString string

func (e errString) Error() string { return string(e) }
