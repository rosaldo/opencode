package provider

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/openai/openai-go/option"
	"github.com/opencode-ai/opencode/internal/llm/models"
	"github.com/opencode-ai/opencode/internal/llm/provider/zen"
	"github.com/opencode-ai/opencode/internal/llm/tools"
	"github.com/opencode-ai/opencode/internal/message"
)

type stubTool struct{ name string }

func (s stubTool) Info() tools.ToolInfo {
	return tools.ToolInfo{Name: s.name, Description: "stub", Parameters: map[string]any{"type": "object"}}
}
func (s stubTool) Run(context.Context, tools.ToolCall) (tools.ToolResponse, error) {
	return tools.NewTextResponse(""), nil
}

// capturedRequest is the parsed body of a request the fake server received.
type capturedRequest struct {
	Model  string `json:"model"`
	Stream bool   `json:"stream"`
	Tools  []struct {
		Function struct {
			Name string `json:"name"`
		} `json:"function"`
	} `json:"tools"`
	Messages []struct {
		Role    string `json:"role"`
		Content any    `json:"content"`
	} `json:"messages"`
}

func (c *capturedRequest) toolNames() []string {
	names := make([]string, 0, len(c.Tools))
	for _, tp := range c.Tools {
		names = append(names, tp.Function.Name)
	}
	return names
}

func newFakeOpenAIServer(t *testing.T, content string) (*httptest.Server, *capturedRequest, *http.Header) {
	t.Helper()
	got := &capturedRequest{}
	headers := &http.Header{}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/chat/completions" {
			http.Error(w, "unexpected path "+r.URL.Path, http.StatusNotFound)
			return
		}
		*headers = r.Header.Clone()

		body, _ := io.ReadAll(r.Body)
		if err := json.Unmarshal(body, got); err != nil {
			http.Error(w, "bad body", http.StatusBadRequest)
			return
		}

		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)

		chunks := []map[string]any{
			{
				"id":      "chatcmpl-1",
				"object":  "chat.completion.chunk",
				"created": 1,
				"model":   "test",
				"choices": []map[string]any{{"index": 0, "delta": map[string]any{"role": "assistant", "content": content}}},
			},
			{
				"id":      "chatcmpl-1",
				"object":  "chat.completion.chunk",
				"created": 1,
				"model":   "test",
				"choices": []map[string]any{{"index": 0, "delta": map[string]any{}, "finish_reason": "stop"}},
			},
		}
		for _, c := range chunks {
			b, _ := json.Marshal(c)
			fmt.Fprintf(w, "data: %s\n\n", b)
		}
		fmt.Fprint(w, "data: [DONE]\n\n")
	}))
	t.Cleanup(srv.Close)
	return srv, got, headers
}

func collectStream(t *testing.T, ch <-chan ProviderEvent) (*ProviderResponse, string) {
	t.Helper()
	var final *ProviderResponse
	var text strings.Builder
	timeout := time.After(10 * time.Second)
	for {
		select {
		case ev, ok := <-ch:
			if !ok {
				return final, text.String()
			}
			switch ev.Type {
			case EventContentDelta:
				text.WriteString(ev.Content)
			case EventComplete:
				final = ev.Response
			case EventError:
				t.Fatalf("stream error: %v", ev.Error)
			}
		case <-timeout:
			t.Fatal("timed out waiting for stream")
		}
	}
}

func TestOpenAIStreamBasic(t *testing.T) {
	srv, got, _ := newFakeOpenAIServer(t, "hello world")

	p, err := NewProvider(models.ProviderOpenAI,
		WithAPIKey("sk-test"),
		WithBaseURL(srv.URL),
		WithModel(models.Model{ID: "test", Provider: models.ProviderOpenAI, APIModel: "gpt-4.1"}),
		WithMaxTokens(32),
		WithSystemMessage("be terse"),
	)
	if err != nil {
		t.Fatalf("NewProvider: %v", err)
	}

	msgs := []message.Message{{Role: message.User, Parts: []message.ContentPart{message.TextContent{Text: "hi"}}}}
	final, text := collectStream(t, p.StreamResponse(context.Background(), msgs, nil))

	if text != "hello world" {
		t.Errorf("streamed text = %q", text)
	}
	if final == nil || final.FinishReason != message.FinishReasonEndTurn {
		t.Fatalf("final = %#v", final)
	}
	if !got.Stream {
		t.Error("request was not streaming")
	}
	if got.Model != "gpt-4.1" {
		t.Errorf("model = %q", got.Model)
	}
	if len(got.Messages) == 0 || got.Messages[0].Role != "system" {
		t.Errorf("first message should be the system prompt, got %#v", got.Messages)
	}
}

// The Zen free tier must advertise all four gated tool names, adding stubs for the
// ones the caller did not provide (our read tool is named "view").
func TestOpenAIZenFreeTierInjectsFingerprint(t *testing.T) {
	srv, got, headers := newFakeOpenAIServer(t, "ok")

	p, err := NewProvider(models.ProviderOpencode,
		WithAPIKey(""),
		WithBaseURL(srv.URL),
		WithModel(models.Model{ID: "test", Provider: models.ProviderOpencode, APIModel: "big-pickle"}),
		WithMaxTokens(32),
		WithSystemMessage("s"),
		WithOpenAIOptions(
			WithOpenAIRequiredTools(zen.RequiredToolNames),
			WithOpenAIExtraHeaders(map[string]string{
				"User-Agent":    zen.UserAgent,
				"Authorization": "Bearer " + zen.AuthBearerValue,
			}),
			WithOpenAIRequestOptions(func() []option.RequestOption {
				opts := []option.RequestOption{
					option.WithHeader("User-Agent", zen.UserAgent),
					option.WithHeader("Authorization", "Bearer "+zen.AuthBearerValue),
				}
				if id, err := zen.SessionID(0); err == nil {
					opts = append(opts, option.WithHeader("x-opencode-session", id))
				}
				return opts
			}),
		),
	)
	if err != nil {
		t.Fatalf("NewProvider: %v", err)
	}

	callerTools := []tools.BaseTool{stubTool{"bash"}, stubTool{"glob"}, stubTool{"grep"}, stubTool{"view"}}
	msgs := []message.Message{{Role: message.User, Parts: []message.ContentPart{message.TextContent{Text: "hi"}}}}
	collectStream(t, p.StreamResponse(context.Background(), msgs, callerTools))

	names := map[string]int{}
	for _, n := range got.toolNames() {
		names[n]++
	}
	for _, want := range zen.RequiredToolNames {
		if names[want] != 1 {
			t.Errorf("required tool %q advertised %d times, want 1 (all=%v)", want, names[want], got.toolNames())
		}
	}
	if names["view"] != 1 {
		t.Errorf("caller tool 'view' must be preserved, all=%v", got.toolNames())
	}

	if got := headers.Get("x-opencode-session"); !strings.HasPrefix(got, "ses_") || len(got) != 30 {
		t.Errorf("x-opencode-session = %q", got)
	}
	if got := headers.Get("User-Agent"); got != zen.UserAgent {
		t.Errorf("User-Agent = %q", got)
	}
	if got := headers.Get("Authorization"); got != "Bearer "+zen.AuthBearerValue {
		t.Errorf("Authorization = %q", got)
	}
}

// A keyed Zen call is a plain OpenAI request: no free-tier fingerprint.
func TestOpenAIZenKeyedHasNoFingerprint(t *testing.T) {
	srv, got, headers := newFakeOpenAIServer(t, "ok")

	p, err := NewProvider(models.ProviderOpencode,
		WithAPIKey("sk-real"),
		WithBaseURL(srv.URL),
		WithModel(models.Model{ID: "test", Provider: models.ProviderOpencode, APIModel: "gpt-5.5"}),
		WithMaxTokens(32),
		WithSystemMessage("s"),
	)
	if err != nil {
		t.Fatalf("NewProvider: %v", err)
	}

	msgs := []message.Message{{Role: message.User, Parts: []message.ContentPart{message.TextContent{Text: "hi"}}}}
	collectStream(t, p.StreamResponse(context.Background(), msgs, nil))

	if got := headers.Get("x-opencode-session"); got != "" {
		t.Errorf("keyed call must not send a session header, got %q", got)
	}
	if names := got.toolNames(); len(names) != 0 {
		t.Errorf("keyed call must not inject tools, got %v", names)
	}
	if auth := headers.Get("Authorization"); auth != "Bearer sk-real" {
		t.Errorf("Authorization = %q, want the real key", auth)
	}
}
