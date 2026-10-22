package web

import (
	"context"
	"embed"
	"encoding/json"
	"errors"
	"io/fs"
	"net/http"
	"strconv"
	"strings"
	"time"

	"engram-manager/internal/api"
	"engram-manager/internal/store"
)

//go:embed static/*
var embeddedFiles embed.FS

// Server hosts the web dashboard and REST API.
type Server struct {
	store  *store.Store
	client *api.Client
	mux    *http.ServeMux
}

// NewServer returns an initialized web dashboard server.
func NewServer(st *store.Store, cl *api.Client) (*Server, error) {
	s := &Server{
		store:  st,
		client: cl,
		mux:    http.NewServeMux(),
	}
	s.registerRoutes()
	return s, nil
}

// Handler returns the underlying http.Handler.
func (s *Server) Handler() http.Handler {
	return s.mux
}

func (s *Server) registerRoutes() {
	// API routes
	s.mux.HandleFunc("GET /api/health", s.handleHealth)
	s.mux.HandleFunc("GET /api/overview", s.handleOverview)
	s.mux.HandleFunc("GET /api/observations", s.handleListObservations)
	s.mux.HandleFunc("GET /api/observations/{id}", s.handleGetObservation)
	s.mux.HandleFunc("PATCH /api/observations/{id}", s.handleUpdateObservation)
	s.mux.HandleFunc("DELETE /api/observations/{id}", s.handleDeleteObservation)
	s.mux.HandleFunc("PUT /api/observations/{id}/pin", s.handlePinObservation)
	s.mux.HandleFunc("DELETE /api/observations/{id}/pin", s.handleUnpinObservation)
	s.mux.HandleFunc("POST /api/observations/{id}/review", s.handleReviewObservation)

	s.mux.HandleFunc("GET /api/relations", s.handleListRelations)
	s.mux.HandleFunc("GET /api/sessions", s.handleListSessions)
	s.mux.HandleFunc("GET /api/sessions/{id}/timeline", s.handleSessionTimeline)
	s.mux.HandleFunc("GET /api/projects", s.handleListProjects)
	s.mux.HandleFunc("GET /api/review-queue", s.handleReviewQueue)

	// Static assets
	sub, err := fs.Sub(embeddedFiles, "static")
	if err != nil {
		panic(err)
	}
	fileServer := http.FileServer(http.FS(sub))

	s.mux.HandleFunc("GET /", func(w http.ResponseWriter, r *http.Request) {
		path := strings.TrimPrefix(r.URL.Path, "/")
		if path == "" || path == "index.html" {
			indexData, err := fs.ReadFile(sub, "index.html")
			if err != nil {
				http.Error(w, "index not found", http.StatusInternalServerError)
				return
			}
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			w.WriteHeader(http.StatusOK)
			w.Write(indexData)
			return
		}

		// If the file exists in static FS, serve it
		if _, err := fs.Stat(sub, path); err == nil {
			fileServer.ServeHTTP(w, r)
			return
		}

		// Fallback to index.html for SPA hash/history routing
		indexData, err := fs.ReadFile(sub, "index.html")
		if err != nil {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.WriteHeader(http.StatusOK)
		w.Write(indexData)
	})
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func writeError(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]string{"error": msg})
}

func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	h, err := s.store.Health()
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	dbSize, walSize, _ := s.store.DBSize()

	var apiStatus string
	ctx, cancel := context.WithTimeout(r.Context(), 1*time.Second)
	defer cancel()
	if s.client != nil {
		if err := s.client.EnsureServer(ctx); err == nil {
			apiStatus = "connected"
		} else {
			apiStatus = "unreachable"
		}
	} else {
		apiStatus = "disabled"
	}

	emb, _ := s.store.EmbeddingStats()
	// Nav counters live here so the sidebar never shows a number that belongs
	// to a different measure. Both counters come from the store's own
	// definitions, never from an inline query that could drift from them.
	relSum, _ := s.store.RelationsSummary()
	reviewTotal, _ := s.store.ReviewPendingCount()

	writeJSON(w, http.StatusOK, map[string]any{
		"health":         h,
		"db_size":        dbSize,
		"wal_size":       walSize,
		"api_status":     apiStatus,
		"embeddings":     emb,
		"relations":      relSum.Total,
		"review_pending": reviewTotal,
	})
}

func (s *Server) handleOverview(w http.ResponseWriter, r *http.Request) {
	h, err := s.store.Health()
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	act, err := s.store.ActivitySummary()
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	relSum, err := s.store.RelationsSummary()
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	projects, err := s.store.Projects()
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	emb, _ := s.store.EmbeddingStats()
	daily, err := s.store.DailyActivity(30)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	hourly, err := s.store.HourlyActivity(24)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	typesAll, err := s.store.TypeCounts(0)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	// All three windows ship together so the client can switch period without
	// refetching, and so a shown number always has its window behind it. The
	// boundaries are the chart's own: the hourly bucket start, and the first
	// midnight of the 7- and 30-day daily series. A rolling duration here would
	// make a range's type total disagree with the bars directly above it.
	now := time.Now().UTC()
	today := now.Truncate(24 * time.Hour)
	windows := map[string][]store.TypeCount{}
	for _, win := range []struct {
		key   string
		since time.Time
	}{
		{"24h", now.Truncate(time.Hour).Add(-23 * time.Hour)},
		{"7d", today.AddDate(0, 0, -6)},
		{"30d", today.AddDate(0, 0, -29)},
	} {
		counts, err := s.store.TypeCountsSince(win.since)
		if err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		windows[win.key] = counts
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"health":     h,
		"activity":   act,
		"relations":  relSum,
		"projects":   projects,
		"embeddings": emb,
		"daily":      daily,
		"hourly":     hourly,
		"types_all":  typesAll,
		"types":      windows,
	})
}

func (s *Server) handleListObservations(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	f := store.Filter{
		Query:          q.Get("q"),
		Project:        q.Get("project"),
		Type:           q.Get("type"),
		Scope:          q.Get("scope"),
		SessionID:      q.Get("session_id"),
		TopicKey:       q.Get("topic_key"),
		PinnedOnly:     q.Get("pinned") == "true" || q.Get("pinned") == "1",
		IncludeDeleted: q.Get("deleted") == "true" || q.Get("deleted") == "1",
	}

	if l, err := strconv.Atoi(q.Get("limit")); err == nil && l > 0 {
		f.Limit = l
	}
	if off, err := strconv.Atoi(q.Get("offset")); err == nil && off >= 0 {
		f.Offset = off
	}

	list, err := s.store.List(f)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if list == nil {
		list = []store.Observation{}
	}
	// The total is the same filter without the page window: a view that says
	// "100 shown" has to be able to say of how many.
	total, err := s.store.Count(f)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"observations": list,
		"count":        len(list),
		"total":        total,
		"offset":       f.Offset,
		"limit":        f.Limit,
	})
}

func (s *Server) handleGetObservation(w http.ResponseWriter, r *http.Request) {
	idStr := r.PathValue("id")
	if idStr == "" {
		writeError(w, http.StatusBadRequest, "invalid observation id")
		return
	}

	obs, err := s.store.GetByRef(idStr)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			writeError(w, http.StatusNotFound, "observation not found")
			return
		}
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	rels, _ := s.store.Relations(store.RelationFilter{ObservationID: strconv.FormatInt(obs.ID, 10)})
	if rels == nil {
		rels = []store.MemoryRelation{}
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"observation": obs,
		"relations":   rels,
	})
}

func (s *Server) handleUpdateObservation(w http.ResponseWriter, r *http.Request) {
	idStr := r.PathValue("id")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid observation id")
		return
	}

	if s.client == nil {
		writeError(w, http.StatusServiceUnavailable, "HTTP API client not configured for writes")
		return
	}

	var in api.UpdateInput
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	if in.ExpectedProject == "" {
		obs, err := s.store.Get(id)
		if err == nil {
			in.ExpectedProject = obs.Project
		}
	}

	if err := s.client.Update(r.Context(), id, in); err != nil {
		writeError(w, http.StatusBadGateway, err.Error())
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "id": id})
}

func (s *Server) handleDeleteObservation(w http.ResponseWriter, r *http.Request) {
	idStr := r.PathValue("id")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid observation id")
		return
	}

	if s.client == nil {
		writeError(w, http.StatusServiceUnavailable, "HTTP API client not configured for writes")
		return
	}

	hard := r.URL.Query().Get("hard") == "true" || r.URL.Query().Get("hard") == "1"
	expectedProject := r.URL.Query().Get("expected_project")
	if expectedProject == "" {
		obs, err := s.store.Get(id)
		if err == nil {
			expectedProject = obs.Project
		}
	}

	if err := s.client.Delete(r.Context(), id, expectedProject, hard); err != nil {
		writeError(w, http.StatusBadGateway, err.Error())
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "id": id, "hard": hard})
}

func (s *Server) handlePinObservation(w http.ResponseWriter, r *http.Request) {
	idStr := r.PathValue("id")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid observation id")
		return
	}
	if s.client == nil {
		writeError(w, http.StatusServiceUnavailable, "HTTP API client not configured for writes")
		return
	}
	if err := s.client.SetPinned(r.Context(), id, true); err != nil {
		writeError(w, http.StatusBadGateway, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "pinned": true})
}

func (s *Server) handleUnpinObservation(w http.ResponseWriter, r *http.Request) {
	idStr := r.PathValue("id")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid observation id")
		return
	}
	if s.client == nil {
		writeError(w, http.StatusServiceUnavailable, "HTTP API client not configured for writes")
		return
	}
	if err := s.client.SetPinned(r.Context(), id, false); err != nil {
		writeError(w, http.StatusBadGateway, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "pinned": false})
}

func (s *Server) handleReviewObservation(w http.ResponseWriter, r *http.Request) {
	idStr := r.PathValue("id")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid observation id")
		return
	}
	if s.client == nil {
		writeError(w, http.StatusServiceUnavailable, "HTTP API client not configured for writes")
		return
	}
	if err := s.client.MarkReviewed(r.Context(), id); err != nil {
		writeError(w, http.StatusBadGateway, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "reviewed": true})
}

func (s *Server) handleListRelations(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	f := store.RelationFilter{
		ObservationID: q.Get("observation_id"),
		SourceID:      q.Get("source_id"),
		TargetID:      q.Get("target_id"),
		Relation:      q.Get("relation"),
		Status:        q.Get("status"),
	}
	if l, err := strconv.Atoi(q.Get("limit")); err == nil && l > 0 {
		f.Limit = l
	}
	if off, err := strconv.Atoi(q.Get("offset")); err == nil && off >= 0 {
		f.Offset = off
	}

	rels, err := s.store.Relations(f)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if rels == nil {
		rels = []store.MemoryRelation{}
	}

	sum, _ := s.store.RelationsSummary()

	writeJSON(w, http.StatusOK, map[string]any{
		"relations": rels,
		"count":     len(rels),
		"summary":   sum,
	})
}

func (s *Server) handleListSessions(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	project := q.Get("project")
	includeEnded := q.Get("ended") == "true" || q.Get("ended") == "1" || q.Get("ended") == "" // default true for web
	limit := 100
	if l, err := strconv.Atoi(q.Get("limit")); err == nil && l > 0 {
		limit = l
	}
	offset := 0
	if off, err := strconv.Atoi(q.Get("offset")); err == nil && off > 0 {
		offset = off
	}

	sessions, err := s.store.SessionsPage(project, includeEnded, limit, offset)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if sessions == nil {
		sessions = []store.Session{}
	}
	total, err := s.store.SessionsCount(project, includeEnded)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"sessions": sessions,
		"count":    len(sessions),
		"total":    total,
		"offset":   offset,
	})
}

func (s *Server) handleSessionTimeline(w http.ResponseWriter, r *http.Request) {
	sessionID := r.PathValue("id")
	if sessionID == "" {
		writeError(w, http.StatusBadRequest, "session id required")
		return
	}

	timeline, err := s.store.SessionTimeline(sessionID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if timeline == nil {
		timeline = []store.Observation{}
	}

	prompts, _ := s.store.SessionPrompts(sessionID)
	if prompts == nil {
		prompts = []store.Prompt{}
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"session_id":   sessionID,
		"observations": timeline,
		"prompts":      prompts,
	})
}

func (s *Server) handleListProjects(w http.ResponseWriter, r *http.Request) {
	projects, err := s.store.Projects()
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if projects == nil {
		projects = []store.ProjectStats{}
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"projects": projects,
		"count":    len(projects),
	})
}

func (s *Server) handleReviewQueue(w http.ResponseWriter, r *http.Request) {
	limit := 50
	if l, err := strconv.Atoi(r.URL.Query().Get("limit")); err == nil && l > 0 {
		limit = l
	}

	queue, err := s.store.ReviewQueue(limit)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if queue == nil {
		queue = []store.Observation{}
	}
	total, err := s.store.ReviewPendingCount()
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"queue": queue,
		"count": len(queue),
		"total": total,
	})
}
