package web

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"sync"
	"time"
)

// SSEEvent represents a server-sent event sent to web clients.
type SSEEvent struct {
	Type      string `json:"type"`
	Timestamp string `json:"timestamp"`
	Data      any    `json:"data,omitempty"`
}

// Broker coordinates real-time SSE client connections and event dispatching.
type Broker struct {
	mu        sync.RWMutex
	clients   map[chan SSEEvent]struct{}
	dbPath    string
	walPath   string
	stopCh    chan struct{}
	closeOnce sync.Once
}

// NewBroker initializes an SSE Broker watching the given database and its WAL file.
func NewBroker(dbPath string) *Broker {
	walPath := dbPath + "-wal"
	return &Broker{
		clients: make(map[chan SSEEvent]struct{}),
		dbPath:  dbPath,
		walPath: walPath,
		stopCh:  make(chan struct{}),
	}
}

// Start begins the background WAL watcher loop.
func (b *Broker) Start() {
	if b.dbPath == "" || b.dbPath == ":memory:" {
		return
	}
	go b.watchLoop()
}

// Stop shuts down the broker and disconnects clients.
func (b *Broker) Stop() {
	b.closeOnce.Do(func() {
		close(b.stopCh)
		b.mu.Lock()
		for ch := range b.clients {
			close(ch)
			delete(b.clients, ch)
		}
		b.mu.Unlock()
	})
}

// Broadcast sends an event to all connected clients.
func (b *Broker) Broadcast(eventType string, data any) {
	evt := SSEEvent{
		Type:      eventType,
		Timestamp: time.Now().UTC().Format(time.RFC3339),
		Data:      data,
	}

	b.mu.RLock()
	defer b.mu.RUnlock()

	for ch := range b.clients {
		select {
		case ch <- evt:
		default:
			// client buffer full, drop to avoid stalling other clients
		}
	}
}

// ServeHTTP streams events to the browser client using Server-Sent Events.
func (b *Broker) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming unsupported", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "text/event-stream; charset=utf-8")
	w.Header().Set("Cache-Control", "no-cache, no-transform")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no")

	clientChan := make(chan SSEEvent, 16)
	b.mu.Lock()
	b.clients[clientChan] = struct{}{}
	b.mu.Unlock()

	defer func() {
		b.mu.Lock()
		delete(b.clients, clientChan)
		b.mu.Unlock()
	}()

	// Send initial connect notice
	initEvt := SSEEvent{
		Type:      "connected",
		Timestamp: time.Now().UTC().Format(time.RFC3339),
	}
	data, _ := json.Marshal(initEvt)
	_, _ = fmt.Fprintf(w, "event: message\ndata: %s\n\n", data)
	flusher.Flush()

	ticker := time.NewTicker(20 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-r.Context().Done():
			return
		case <-b.stopCh:
			return
		case <-ticker.C:
			// SSE keepalive comment
			if _, err := fmt.Fprintf(w, ": keepalive\n\n"); err != nil {
				return
			}
			flusher.Flush()
		case evt, ok := <-clientChan:
			if !ok {
				return
			}
			payload, err := json.Marshal(evt)
			if err != nil {
				continue
			}
			if _, err := fmt.Fprintf(w, "event: message\ndata: %s\n\n", payload); err != nil {
				return
			}
			flusher.Flush()
		}
	}
}

func (b *Broker) watchLoop() {
	ticker := time.NewTicker(350 * time.Millisecond)
	defer ticker.Stop()

	var (
		lastWalMod  time.Time
		lastWalSize int64
		lastDbMod   time.Time
		lastDbSize  int64
	)

	// Prime initial state
	if fi, err := os.Stat(b.walPath); err == nil {
		lastWalMod = fi.ModTime()
		lastWalSize = fi.Size()
	}
	if fi, err := os.Stat(b.dbPath); err == nil {
		lastDbMod = fi.ModTime()
		lastDbSize = fi.Size()
	}

	for {
		select {
		case <-b.stopCh:
			return
		case <-ticker.C:
			changed := false

			// Check WAL file
			if fi, err := os.Stat(b.walPath); err == nil {
				if fi.ModTime() != lastWalMod || fi.Size() != lastWalSize {
					lastWalMod = fi.ModTime()
					lastWalSize = fi.Size()
					changed = true
				}
			}

			// Check main DB file
			if fi, err := os.Stat(b.dbPath); err == nil {
				if fi.ModTime() != lastDbMod || fi.Size() != lastDbSize {
					lastDbMod = fi.ModTime()
					lastDbSize = fi.Size()
					changed = true
				}
			}

			if changed {
				// 300ms debounce window
				time.Sleep(300 * time.Millisecond)
				b.Broadcast("engram-mutation", map[string]string{
					"source": "wal-watcher",
				})
			}
		}
	}
}
