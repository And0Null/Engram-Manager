package api

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// Every test here runs against a fake server. The real Engram database is the
// user's memory store, so the write path is never exercised against it.

func newTestServer(t *testing.T, handler http.HandlerFunc) *Client {
	t.Helper()
	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)
	return New(strings.TrimPrefix(srv.URL, "http://"))
}

func TestCreateSendsTitleAndReturnsID(t *testing.T) {
	var gotMethod, gotPath, gotType, gotBody string
	c := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		gotMethod, gotPath, gotType = r.Method, r.URL.Path, r.Header.Get("Content-Type")
		b, _ := io.ReadAll(r.Body)
		gotBody = string(b)
		w.Header().Set("Content-Type", "application/json")
		io.WriteString(w, `{"id":42}`)
	})

	id, err := c.Create(context.Background(), ObservationInput{
		Title: "test", Content: "body", Type: "manual", Project: "demo", Scope: "project",
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if id != 42 {
		t.Errorf("id = %d, want 42", id)
	}
	if gotMethod != http.MethodPost || gotPath != "/observations" {
		t.Errorf("%s %s", gotMethod, gotPath)
	}
	if gotType != "application/json" {
		t.Errorf("Content-Type = %q", gotType)
	}
	if !strings.Contains(gotBody, `"title":"test"`) {
		t.Errorf("body = %s", gotBody)
	}
}

func TestCreateOmitsEmptyOptionalFields(t *testing.T) {
	var gotBody map[string]any
	c := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		json.NewDecoder(r.Body).Decode(&gotBody)
		io.WriteString(w, `{"id":1}`)
	})
	_, err := c.Create(context.Background(), ObservationInput{Title: "only title"})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	for _, k := range []string{"type", "project", "scope", "session_id", "topic_key"} {
		if _, present := gotBody[k]; present {
			t.Errorf("empty %q was sent anyway: %v", k, gotBody)
		}
	}
}

func TestUpdateAlwaysSendsExpectedProject(t *testing.T) {
	var gotBody map[string]any
	var gotMethod, gotPath string
	c := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		gotMethod, gotPath = r.Method, r.URL.Path
		json.NewDecoder(r.Body).Decode(&gotBody)
		io.WriteString(w, `{}`)
	})

	if err := c.Update(context.Background(), 7, UpdateInput{
		ExpectedProject: "and0null", Content: "new content",
	}); err != nil {
		t.Fatalf("Update: %v", err)
	}
	if gotMethod != http.MethodPatch || gotPath != "/observations/7" {
		t.Errorf("%s %s", gotMethod, gotPath)
	}
	// Engram rejects a PATCH without expected_project, so this field is not
	// optional in the client either.
	if gotBody["expected_project"] != "and0null" {
		t.Errorf("expected_project missing from %v", gotBody)
	}
	if gotBody["content"] != "new content" {
		t.Errorf("content = %v", gotBody["content"])
	}
	// An empty title must not be sent: it would mean "clear the title".
	if _, present := gotBody["title"]; present {
		t.Errorf("empty title was sent and would erase it: %v", gotBody)
	}
}

func TestDeleteSendsExpectedProject(t *testing.T) {
	var gotBody map[string]any
	var gotMethod, gotPath, gotQuery string
	c := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		gotMethod, gotPath, gotQuery = r.Method, r.URL.Path, r.URL.RawQuery
		json.NewDecoder(r.Body).Decode(&gotBody)
		io.WriteString(w, `{}`)
	})

	if err := c.Delete(context.Background(), 9, "demo", false); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if gotMethod != http.MethodDelete || gotPath != "/observations/9" {
		t.Errorf("%s %s", gotMethod, gotPath)
	}
	if gotBody["expected_project"] != "demo" {
		t.Errorf("expected_project missing: %v", gotBody)
	}
	if gotQuery != "" {
		t.Errorf("soft delete asked for %q", gotQuery)
	}
}

func TestDeleteHardSetsFlag(t *testing.T) {
	var gotQuery string
	c := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		gotQuery = r.URL.RawQuery
		io.WriteString(w, `{}`)
	})
	if err := c.Delete(context.Background(), 9, "demo", true); err != nil {
		t.Fatalf("Delete hard: %v", err)
	}
	if !strings.Contains(gotQuery, "hard") {
		t.Errorf("hard delete query = %q", gotQuery)
	}
}

func TestSetPinnedUsesVerb(t *testing.T) {
	tests := []struct {
		pinned bool
		want   string
	}{
		{true, http.MethodPut},
		{false, http.MethodDelete},
	}
	for _, tc := range tests {
		var gotMethod, gotPath string
		c := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
			gotMethod, gotPath = r.Method, r.URL.Path
			io.WriteString(w, `{}`)
		})
		if err := c.SetPinned(context.Background(), 3, tc.pinned); err != nil {
			t.Fatalf("SetPinned(%v): %v", tc.pinned, err)
		}
		if gotMethod != tc.want || gotPath != "/observations/3/pin" {
			t.Errorf("SetPinned(%v) = %s %s, want %s", tc.pinned, gotMethod, gotPath, tc.want)
		}
	}
}

func TestMarkReviewedPostsObservationID(t *testing.T) {
	var gotPath string
	var gotBody map[string]any
	c := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		json.NewDecoder(r.Body).Decode(&gotBody)
		io.WriteString(w, `{}`)
	})
	if err := c.MarkReviewed(context.Background(), 11); err != nil {
		t.Fatalf("MarkReviewed: %v", err)
	}
	if gotPath != "/review/mark_reviewed" {
		t.Errorf("path = %q", gotPath)
	}
	if gotBody["observation_id"].(float64) != 11 {
		t.Errorf("observation_id = %v", gotBody["observation_id"])
	}
}

// A rejected write must say what Engram objected to, not just the status code.
func TestErrorSurfacesEngramMessage(t *testing.T) {
	c := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		io.WriteString(w, `{"error":"expected_project must be a valid non-empty project name"}`)
	})
	err := c.Update(context.Background(), 1, UpdateInput{ExpectedProject: ""})
	if err == nil {
		t.Fatal("expected an error")
	}
	if !strings.Contains(err.Error(), "expected_project must be a valid") {
		t.Errorf("error lost Engram's reason: %v", err)
	}
	if !strings.Contains(err.Error(), "400") {
		t.Errorf("error lost the status: %v", err)
	}
}

func TestErrorHandlesNonJSONBody(t *testing.T) {
	c := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		io.WriteString(w, `<html>proxy error</html>`)
	})
	err := c.Update(context.Background(), 1, UpdateInput{ExpectedProject: "p"})
	if err == nil || !strings.Contains(err.Error(), "proxy error") {
		t.Errorf("expected the raw body in the error, got %v", err)
	}
}

func TestErrorHandlesEmptyBody(t *testing.T) {
	c := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
	})
	err := c.Update(context.Background(), 1, UpdateInput{ExpectedProject: "p"})
	if err == nil || !strings.Contains(err.Error(), "no detail") {
		t.Errorf("expected a fallback detail, got %v", err)
	}
}

func TestEnsureServerReportsHowToStartIt(t *testing.T) {
	c := New("127.0.0.1:1") // nothing listens here
	err := c.EnsureServer(context.Background())
	if err == nil {
		t.Fatal("expected an error against a dead address")
	}
	if !strings.Contains(err.Error(), "engram serve") {
		t.Errorf("error should tell the user how to start the server: %v", err)
	}
}

func TestEnsureServerOK(t *testing.T) {
	c := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/health" {
			t.Errorf("probed %q, want /health", r.URL.Path)
		}
		w.WriteHeader(http.StatusOK)
	})
	if err := c.EnsureServer(context.Background()); err != nil {
		t.Errorf("EnsureServer: %v", err)
	}
}

func TestNewNormalizesAddr(t *testing.T) {
	tests := []struct{ in, want string }{
		{"", "http://" + DefaultAddr},
		{"127.0.0.1:9000", "http://127.0.0.1:9000"},
		{"http://x:1", "http://x:1"},
		{"https://x:1", "https://x:1"},
		{"127.0.0.1:9000/", "http://127.0.0.1:9000"},
	}
	for _, tc := range tests {
		c := New(tc.in)
		if c.base != tc.want {
			t.Errorf("New(%q).base = %q, want %q", tc.in, c.base, tc.want)
		}
	}
}
