// Package api writes to Engram over its HTTP API.
//
// Reads deliberately go to SQLite instead (see internal/store), because this
// is the only path that gets Engram's own soft-delete, relation, dedupe, and
// project-resolution rules applied. Writing straight to the database would
// mean reimplementing those rules and drifting from them on the next upgrade.
package api

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// DefaultAddr is the address `engram serve` binds by default.
const DefaultAddr = "127.0.0.1:7437"

// Client talks to a running `engram serve`.
type Client struct {
	base string
	http *http.Client
}

// New returns a client for addr. The timeout is generous because a write can
// block behind Engram's own WAL checkpoint.
func New(addr string) *Client {
	if addr == "" {
		addr = DefaultAddr
	}
	if !strings.HasPrefix(addr, "http://") && !strings.HasPrefix(addr, "https://") {
		addr = "http://" + addr
	}
	return &Client{
		base: strings.TrimRight(addr, "/"),
		http: &http.Client{Timeout: 30 * time.Second},
	}
}

// EnsureServer returns an error explaining how to start the server when it is
// not reachable, instead of a bare dial failure.
func (c *Client) EnsureServer(ctx context.Context) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.base+"/health", nil)
	if err != nil {
		return err
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return fmt.Errorf("Engram server unreachable at %s — start it with `engram serve`: %w", c.base, err)
	}
	defer resp.Body.Close()
	io.Copy(io.Discard, resp.Body)
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("Engram server at %s returned %s on /health", c.base, resp.Status)
	}
	return nil
}

// ObservationInput is the payload for creating an observation.
type ObservationInput struct {
	Title     string `json:"title"`
	Content   string `json:"content"`
	Type      string `json:"type,omitempty"`
	Project   string `json:"project,omitempty"`
	Scope     string `json:"scope,omitempty"`
	SessionID string `json:"session_id,omitempty"`
	TopicKey  string `json:"topic_key,omitempty"`
}

// UpdateInput is the payload for patching an observation. ExpectedProject is
// required by the API as a concurrency guard: the write is rejected if the
// observation no longer belongs to that project.
type UpdateInput struct {
	ExpectedProject string `json:"expected_project"`
	Title           string `json:"title,omitempty"`
	Content         string `json:"content,omitempty"`
	Type            string `json:"type,omitempty"`
	TopicKey        string `json:"topic_key,omitempty"`
}

// Create saves a new observation.
func (c *Client) Create(ctx context.Context, in ObservationInput) (int64, error) {
	var out struct {
		ID int64 `json:"id"`
	}
	if err := c.do(ctx, http.MethodPost, "/observations", in, &out); err != nil {
		return 0, err
	}
	return out.ID, nil
}

// Update patches an existing observation.
func (c *Client) Update(ctx context.Context, id int64, in UpdateInput) error {
	return c.do(ctx, http.MethodPatch, fmt.Sprintf("/observations/%d", id), in, nil)
}

// Delete soft-deletes an observation. Pass hard to remove it permanently.
//
// The soft-delete path is the default on purpose: it is recoverable, and a
// manager UI should not be the thing that makes a memory unrecoverable.
func (c *Client) Delete(ctx context.Context, id int64, expectedProject string, hard bool) error {
	path := fmt.Sprintf("/observations/%d", id)
	if hard {
		path += "?hard=1"
	}
	body := map[string]string{"expected_project": expectedProject}
	return c.do(ctx, http.MethodDelete, path, body, nil)
}

// SetPinned pins or unpins an observation.
func (c *Client) SetPinned(ctx context.Context, id int64, pinned bool) error {
	method := http.MethodPut
	if !pinned {
		method = http.MethodDelete
	}
	return c.do(ctx, method, fmt.Sprintf("/observations/%d/pin", id), nil, nil)
}

// MarkReviewed records that the user reviewed an observation, closing the
// review loop the same way mem_review does.
func (c *Client) MarkReviewed(ctx context.Context, id int64) error {
	return c.do(ctx, http.MethodPost, "/review/mark_reviewed",
		map[string]any{"observation_id": id}, nil)
}

func (c *Client) do(ctx context.Context, method, path string, in, out any) error {
	var body io.Reader
	if in != nil {
		buf, err := json.Marshal(in)
		if err != nil {
			return fmt.Errorf("encode request: %w", err)
		}
		body = bytes.NewReader(buf)
	}
	req, err := http.NewRequestWithContext(ctx, method, c.base+path, body)
	if err != nil {
		return fmt.Errorf("build request: %w", err)
	}
	if in != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	req.Header.Set("Accept", "application/json")

	resp, err := c.http.Do(req)
	if err != nil {
		return fmt.Errorf("%s %s: %w", method, path, err)
	}
	defer resp.Body.Close()

	raw, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if resp.StatusCode >= 300 {
		return fmt.Errorf("%s %s: %s: %s", method, path, resp.Status, apiError(raw))
	}
	if out != nil {
		if err := json.Unmarshal(raw, out); err != nil {
			return fmt.Errorf("decode %s %s response: %w", method, url.PathEscape(path), err)
		}
	}
	return nil
}

// apiError surfaces Engram's own message when there is one; the fallback keeps
// an HTML or empty body from hiding the status.
func apiError(raw []byte) string {
	var e struct {
		Error   string `json:"error"`
		Message string `json:"message"`
	}
	if err := json.Unmarshal(raw, &e); err == nil {
		if e.Error != "" {
			return e.Error
		}
		if e.Message != "" {
			return e.Message
		}
	}
	if s := strings.TrimSpace(string(raw)); s != "" && len(s) < 300 {
		return s
	}
	return "no detail"
}
