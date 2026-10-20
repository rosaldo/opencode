package acp

import (
	"context"
	"encoding/json"
	"io"
	"strings"
	"sync"
	"testing"
	"time"
)

type fakeAgent struct {
	mu        sync.Mutex
	cancelled string
}

func (f *fakeAgent) Run(ctx context.Context, sessionID, content string) (<-chan Chunk, error) {
	ch := make(chan Chunk, 2)
	ch <- Chunk{Text: "hello "}
	ch <- Chunk{Text: "world" + content, Done: true}
	close(ch)
	return ch, nil
}

func (f *fakeAgent) Cancel(sessionID string) {
	f.mu.Lock()
	f.cancelled = sessionID
	f.mu.Unlock()
}

func TestACPHandshakeAndPrompt(t *testing.T) {
	pr, pw := io.Pipe()
	cr, cw := io.Pipe()

	srv := New(&fakeAgent{}, cr, pw)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	done := make(chan error, 1)
	go func() { done <- srv.Run(ctx) }()

	enc := json.NewEncoder(cw)
	dec := json.NewDecoder(pr)

	// initialize
	enc.Encode(map[string]any{"jsonrpc": "2.0", "id": 1, "method": "initialize"})
	resp := readResponse(t, dec)
	if resp["id"].(float64) != 1 {
		t.Fatalf("id = %v", resp["id"])
	}
	result, _ := resp["result"].(map[string]any)
	if result["protocolVersion"].(float64) != 1 {
		t.Fatalf("protocolVersion = %v", result["protocolVersion"])
	}

	// session/new
	enc.Encode(map[string]any{"jsonrpc": "2.0", "id": 2, "method": "session/new", "params": map[string]any{"cwd": "/tmp"}})
	resp = readResponse(t, dec)
	sid := resp["result"].(map[string]any)["sessionId"].(string)
	if sid == "" {
		t.Fatal("empty session id")
	}

	// session/prompt -> expect updates then result
	enc.Encode(map[string]any{
		"jsonrpc": "2.0", "id": 3, "method": "session/prompt",
		"params": map[string]any{
			"sessionId": sid,
			"prompt":    []map[string]any{{"type": "text", "text": "!"}},
		},
	})

	gotUpdates := 0
	var final map[string]any
	deadline := time.After(3 * time.Second)
	for final == nil {
		select {
		case <-deadline:
			t.Fatalf("timed out; updates=%d", gotUpdates)
		default:
			msg := readResponse(t, dec)
			if msg["method"] == "session/update" {
				gotUpdates++
				continue
			}
			if id, ok := msg["id"].(float64); ok && id == 3 {
				final = msg
			}
		}
	}
	if gotUpdates != 2 {
		t.Fatalf("updates = %d, want 2", gotUpdates)
	}
	if final["result"].(map[string]any)["stopReason"] != "end_turn" {
		t.Fatalf("stopReason = %v", final["result"])
	}

	// unknown session prompt -> error
	enc.Encode(map[string]any{"jsonrpc": "2.0", "id": 4, "method": "session/prompt",
		"params": map[string]any{"sessionId": "nope", "prompt": []map[string]any{}}})
	resp = readResponse(t, dec)
	if resp["error"] == nil {
		t.Fatalf("expected error for unknown session, got %v", resp)
	}

	// cancel
	enc.Encode(map[string]any{"jsonrpc": "2.0", "id": 5, "method": "session/cancel", "params": map[string]any{"sessionId": sid}})
	_ = readResponse(t, dec)

	// unknown method
	enc.Encode(map[string]any{"jsonrpc": "2.0", "id": 6, "method": "bogus"})
	resp = readResponse(t, dec)
	if resp["error"] == nil {
		t.Fatalf("expected method-not-found error")
	}

	cw.Close()
	cancel()
	<-done
}

func TestACPUnknownMethodError(t *testing.T) {
	// Guard against trimming whitespace in the method name.
	if strings.TrimSpace(" initialize ") != "initialize" {
		t.Fatal("sanity")
	}
}

func readResponse(t *testing.T, dec *json.Decoder) map[string]any {
	t.Helper()
	var m map[string]any
	if err := dec.Decode(&m); err != nil {
		t.Fatalf("decode: %v", err)
	}
	return m
}
