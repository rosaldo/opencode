// Package server implements a local HTTP API over sessions, messages and the
// live event stream (Spec 12a). It binds to loopback by default and requires a
// bearer token for any non-loopback access.
package server

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/opencode-ai/opencode/internal/logging"
	"github.com/opencode-ai/opencode/internal/message"
	"github.com/opencode-ai/opencode/internal/pubsub"
	"github.com/opencode-ai/opencode/internal/session"
)

// Event is a generic event fanned out over SSE.
type Event struct {
	Type    string `json:"type"`
	Session string `json:"session,omitempty"`
	Payload any    `json:"payload,omitempty"`
}

// Sessions is the subset of session.Service the server needs.
type Sessions interface {
	List(ctx context.Context) ([]session.Session, error)
	Get(ctx context.Context, id string) (session.Session, error)
}

// Messages is the subset of message.Service the server needs.
type Messages interface {
	List(ctx context.Context, sessionID string) ([]message.Message, error)
}

// Config configures a Server.
type Config struct {
	// Addr is the listen address. Defaults to 127.0.0.1:0 (loopback, random
	// port) when empty.
	Addr string
	// Token, when set, is required on every request (header
	// "Authorization: Bearer <token>" or "?token="). It is mandatory for any
	// non-loopback bind.
	Token string
	// Sessions/Messages back the read endpoints.
	Sessions Sessions
	Messages Messages
}

// Server is the HTTP API server.
type Server struct {
	cfg    Config
	http   *http.Server
	broker *pubsub.Broker[Event]
	share  *shareStore
	ln     net.Listener
}

// New builds a server, validating that a token is present for non-loopback
// binds (acceptance criterion 6).
func New(cfg Config) (*Server, error) {
	if cfg.Addr == "" {
		cfg.Addr = "127.0.0.1:0"
	}
	host, _, err := net.SplitHostPort(cfg.Addr)
	if err != nil {
		return nil, fmt.Errorf("invalid addr %q: %w", cfg.Addr, err)
	}
	if !isLoopback(host) && cfg.Token == "" {
		return nil, fmt.Errorf("refusing to bind %q without a token", cfg.Addr)
	}
	s := &Server{
		cfg:    cfg,
		broker: pubsub.NewBroker[Event](),
		share:  newShareStore(),
	}
	mux := http.NewServeMux()
	mux.HandleFunc("/api/health", s.auth(s.handleHealth))
	mux.HandleFunc("/api/sessions", s.auth(s.handleSessions))
	mux.HandleFunc("/api/sessions/", s.auth(s.handleSession))
	mux.HandleFunc("/api/events", s.auth(s.handleEvents))
	mux.HandleFunc("/api/share", s.auth(s.handleShare))
	mux.HandleFunc("/api/share/", s.auth(s.handleShareByID))
	s.http = &http.Server{Handler: mux}
	return s, nil
}

// Start begins listening. It returns after the listener is bound; serving runs
// in the background.
func (s *Server) Start() error {
	ln, err := net.Listen("tcp", s.cfg.Addr)
	if err != nil {
		return err
	}
	s.ln = ln
	go func() {
		if err := s.http.Serve(ln); err != nil && err != http.ErrServerClosed {
			logging.Error("http server stopped", "error", err)
		}
	}()
	return nil
}

// Addr returns the actual listen address (useful when the port was 0).
func (s *Server) Addr() string {
	if s.ln != nil {
		return s.ln.Addr().String()
	}
	return s.cfg.Addr
}

// URL returns the base http URL of the server.
func (s *Server) URL() string { return "http://" + s.Addr() }

// Close shuts the server down.
func (s *Server) Close() error {
	s.broker.Shutdown()
	if s.http != nil {
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		return s.http.Shutdown(ctx)
	}
	return nil
}

// Emit publishes an event to SSE subscribers.
func (s *Server) Emit(e Event) { s.broker.Publish(pubsub.UpdatedEvent, e) }

func isLoopback(host string) bool {
	if host == "" || host == "localhost" {
		return true
	}
	if ip := net.ParseIP(host); ip != nil {
		return ip.IsLoopback()
	}
	return false
}

// auth enforces the bearer token when configured.
func (s *Server) auth(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if s.cfg.Token == "" {
			next(w, r)
			return
		}
		if tokenFromRequest(r) != s.cfg.Token {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		next(w, r)
	}
}

func tokenFromRequest(r *http.Request) string {
	if h := r.Header.Get("Authorization"); strings.HasPrefix(h, "Bearer ") {
		return strings.TrimPrefix(h, "Bearer ")
	}
	return r.URL.Query().Get("token")
}

func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (s *Server) handleSessions(w http.ResponseWriter, r *http.Request) {
	if s.cfg.Sessions == nil {
		http.Error(w, "sessions unavailable", http.StatusServiceUnavailable)
		return
	}
	sessions, err := s.cfg.Sessions.List(r.Context())
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusOK, sessions)
}

// handleSession serves /api/sessions/{id} and /api/sessions/{id}/messages.
func (s *Server) handleSession(w http.ResponseWriter, r *http.Request) {
	rest := strings.TrimPrefix(r.URL.Path, "/api/sessions/")
	parts := strings.Split(rest, "/")
	id := parts[0]
	if id == "" {
		http.Error(w, "missing session id", http.StatusBadRequest)
		return
	}
	if len(parts) >= 2 && parts[1] == "messages" {
		if s.cfg.Messages == nil {
			http.Error(w, "messages unavailable", http.StatusServiceUnavailable)
			return
		}
		msgs, err := s.cfg.Messages.List(r.Context(), id)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		writeJSON(w, http.StatusOK, msgs)
		return
	}
	if s.cfg.Sessions == nil {
		http.Error(w, "sessions unavailable", http.StatusServiceUnavailable)
		return
	}
	sess, err := s.cfg.Sessions.Get(r.Context(), id)
	if err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}
	writeJSON(w, http.StatusOK, sess)
}

// handleEvents streams server events as Server-Sent Events.
func (s *Server) handleEvents(w http.ResponseWriter, r *http.Request) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming unsupported", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.WriteHeader(http.StatusOK)
	flusher.Flush()

	ch := s.broker.Subscribe(r.Context())
	for {
		select {
		case <-r.Context().Done():
			return
		case ev, ok := <-ch:
			if !ok {
				return
			}
			data, err := json.Marshal(ev.Payload)
			if err != nil {
				continue
			}
			fmt.Fprintf(w, "event: %s\ndata: %s\n\n", ev.Payload.Type, data)
			flusher.Flush()
		}
	}
}

func writeJSON(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(v)
}

// shareStore holds share links in memory (Spec 12a local share).
type shareStore struct {
	mu    sync.Mutex
	links map[string]string // id -> session id
}

func newShareStore() *shareStore { return &shareStore{links: map[string]string{}} }

func (s *shareStore) create(id, sessionID string) {
	s.mu.Lock()
	s.links[id] = sessionID
	s.mu.Unlock()
}

func (s *shareStore) get(id string) (string, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	v, ok := s.links[id]
	return v, ok
}

func (s *shareStore) remove(id string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.links[id]; !ok {
		return false
	}
	delete(s.links, id)
	return true
}
