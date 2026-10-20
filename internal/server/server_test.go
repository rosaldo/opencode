package server

import (
	"bufio"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/opencode-ai/opencode/internal/message"
	"github.com/opencode-ai/opencode/internal/session"
)

type fakeSessions struct{ items []session.Session }

func (f fakeSessions) List(context.Context) ([]session.Session, error) { return f.items, nil }
func (f fakeSessions) Get(_ context.Context, id string) (session.Session, error) {
	for _, s := range f.items {
		if s.ID == id {
			return s, nil
		}
	}
	return session.Session{}, context.Canceled
}

type fakeMessages struct{}

func (fakeMessages) List(context.Context, string) ([]message.Message, error) {
	return []message.Message{{ID: "m1"}}, nil
}

func newTestServer(t *testing.T, token string) *Server {
	t.Helper()
	s, err := New(Config{
		Addr:     "127.0.0.1:0",
		Token:    token,
		Sessions: fakeSessions{items: []session.Session{{ID: "s1", Title: "first"}}},
		Messages: fakeMessages{},
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if err := s.Start(); err != nil {
		t.Fatalf("Start: %v", err)
	}
	t.Cleanup(func() { s.Close() })
	return s
}

func TestUIPageServed(t *testing.T) {
	s := newTestServer(t, "")
	for _, path := range []string{"/", "/ui"} {
		resp, err := http.Get(s.URL() + path)
		if err != nil {
			t.Fatal(err)
		}
		body, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			t.Fatalf("%s status = %d", path, resp.StatusCode)
		}
		if ct := resp.Header.Get("Content-Type"); !strings.HasPrefix(ct, "text/html") {
			t.Fatalf("%s content-type = %q", path, ct)
		}
		if !strings.Contains(string(body), "/api/sessions") {
			t.Fatalf("%s body does not reference the API", path)
		}
	}
	resp, err := http.Get(s.URL() + "/nope")
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("unknown path status = %d", resp.StatusCode)
	}
}

func TestHealthNoAuth(t *testing.T) {
	s := newTestServer(t, "")
	resp, err := http.Get(s.URL() + "/api/health")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d", resp.StatusCode)
	}
}

func TestAuthRequired(t *testing.T) {
	s := newTestServer(t, "secret")

	resp, _ := http.Get(s.URL() + "/api/sessions")
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("no token: status = %d, want 401", resp.StatusCode)
	}
	resp.Body.Close()

	req, _ := http.NewRequest(http.MethodGet, s.URL()+"/api/sessions", nil)
	req.Header.Set("Authorization", "Bearer secret")
	resp, _ = http.DefaultClient.Do(req)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("with token: status = %d, want 200", resp.StatusCode)
	}
	resp.Body.Close()

	// Query-param token is also accepted.
	resp, _ = http.Get(s.URL() + "/api/sessions?token=secret")
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("query token: status = %d", resp.StatusCode)
	}
	resp.Body.Close()
}

func TestSessionsAndMessages(t *testing.T) {
	s := newTestServer(t, "")

	var sessions []session.Session
	getJSON(t, s.URL()+"/api/sessions", &sessions)
	if len(sessions) != 1 || sessions[0].ID != "s1" {
		t.Fatalf("sessions = %+v", sessions)
	}

	var sess session.Session
	getJSON(t, s.URL()+"/api/sessions/s1", &sess)
	if sess.ID != "s1" {
		t.Fatalf("session = %+v", sess)
	}

	var msgs []message.Message
	getJSON(t, s.URL()+"/api/sessions/s1/messages", &msgs)
	if len(msgs) != 1 || msgs[0].ID != "m1" {
		t.Fatalf("messages = %+v", msgs)
	}
}

func TestEventsSSE(t *testing.T) {
	s := newTestServer(t, "")

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, s.URL()+"/api/events", nil)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	reader := bufio.NewReader(resp.Body)
	// Give the handler a moment to subscribe.
	time.Sleep(50 * time.Millisecond)
	s.Emit(Event{Type: "session.created", Session: "s1", Payload: map[string]any{"type": "session.created"}})

	deadline := time.After(2 * time.Second)
	lines := make(chan string, 4)
	go func() {
		for {
			line, err := reader.ReadString('\n')
			if err != nil {
				return
			}
			lines <- line
		}
	}()
	for {
		select {
		case <-deadline:
			t.Fatal("did not receive SSE event")
		case line := <-lines:
			if strings.Contains(line, "session.created") {
				return
			}
		}
	}
}

func TestShareCreateAndRevoke(t *testing.T) {
	s := newTestServer(t, "")

	resp, err := http.Post(s.URL()+"/api/share", "application/json", strings.NewReader(`{"session_id":"s1"}`))
	if err != nil {
		t.Fatal(err)
	}
	var share ShareResponse
	if err := json.NewDecoder(resp.Body).Decode(&share); err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if share.ID == "" || share.SessionID != "s1" {
		t.Fatalf("share = %+v", share)
	}

	// Fetch shared payload.
	presp, _ := http.Get(s.URL() + "/api/share/" + share.ID)
	if presp.StatusCode != http.StatusOK {
		t.Fatalf("get share status = %d", presp.StatusCode)
	}
	presp.Body.Close()

	// Revoke.
	req, _ := http.NewRequest(http.MethodDelete, s.URL()+"/api/share/"+share.ID, nil)
	dresp, _ := http.DefaultClient.Do(req)
	if dresp.StatusCode != http.StatusOK {
		t.Fatalf("delete share status = %d", dresp.StatusCode)
	}
	dresp.Body.Close()

	// Gone afterwards.
	gresp, _ := http.Get(s.URL() + "/api/share/" + share.ID)
	if gresp.StatusCode != http.StatusNotFound {
		t.Fatalf("after revoke status = %d, want 404", gresp.StatusCode)
	}
	gresp.Body.Close()
}

func TestShareUnknownSession(t *testing.T) {
	s := newTestServer(t, "")
	resp, err := http.Post(s.URL()+"/api/share", "application/json", strings.NewReader(`{"session_id":"nope"}`))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", resp.StatusCode)
	}
}

func TestNonLoopbackRequiresToken(t *testing.T) {
	_, err := New(Config{Addr: "0.0.0.0:0"})
	if err == nil {
		t.Fatal("expected error for non-loopback bind without token")
	}
	if _, err := New(Config{Addr: "0.0.0.0:0", Token: "t"}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func getJSON(t *testing.T, url string, v any) {
	t.Helper()
	resp, err := http.Get(url)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("GET %s status = %d", url, resp.StatusCode)
	}
	if err := json.NewDecoder(resp.Body).Decode(v); err != nil {
		t.Fatalf("decode %s: %v", url, err)
	}
}
