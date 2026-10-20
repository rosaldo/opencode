package provider

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"github.com/opencode-ai/opencode/internal/config"
	"github.com/opencode-ai/opencode/internal/llm/models"
	"github.com/opencode-ai/opencode/internal/message"
	"github.com/spf13/viper"
)

func TestNewOpenAIClientGenericSettingsOverride(t *testing.T) {
	// Provider default set via option, then generic v2 setting must win.
	client := newOpenAIClient(providerClientOptions{
		apiKey:  "k",
		baseURL: "https://override.example/v1",
		headers: map[string]string{"X-Tenant": "acme"},
		openaiOptions: []OpenAIOption{
			WithOpenAIBaseURL("https://default.example/v1"),
			WithOpenAIExtraHeaders(map[string]string{"X-Default": "1"}),
		},
	})
	c := client.(*openaiClient)
	if c.options.baseURL != "https://override.example/v1" {
		t.Fatalf("baseURL = %q", c.options.baseURL)
	}
	if c.options.extraHeaders["X-Tenant"] != "acme" {
		t.Fatalf("headers = %v", c.options.extraHeaders)
	}
	if c.options.extraHeaders["X-Default"] != "1" {
		t.Fatalf("default header lost: %v", c.options.extraHeaders)
	}
}

func TestOpenAIBaseURLAndHeadersReachRequest(t *testing.T) {
	var gotPath, gotTenant string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		gotTenant = r.Header.Get("X-Tenant")
		io.ReadAll(r.Body)
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{
			"id": "1", "object": "chat.completion", "model": "gpt-4.1",
			"choices": []map[string]any{{
				"index":         0,
				"message":       map[string]any{"role": "assistant", "content": "hi"},
				"finish_reason": "stop",
			}},
			"usage": map[string]any{"prompt_tokens": 1, "completion_tokens": 1, "total_tokens": 2},
		})
	}))
	defer srv.Close()

	loadConfigForTest(t)
	client := newOpenAIClient(providerClientOptions{
		apiKey:  "k",
		model:   models.SupportedModels[models.GPT41],
		baseURL: srv.URL,
		headers: map[string]string{"X-Tenant": "acme"},
	}).(*openaiClient)

	_, err := client.send(t.Context(), []message.Message{
		{Role: message.User, Parts: []message.ContentPart{message.TextContent{Text: "hi"}}},
	}, nil)
	if err != nil {
		t.Fatalf("send: %v", err)
	}
	if gotTenant != "acme" {
		t.Fatalf("header not sent, got %q", gotTenant)
	}
	if gotPath != "/chat/completions" {
		t.Fatalf("path = %q (baseURL not applied)", gotPath)
	}
}

// loadConfigForTest loads a minimal config so provider.send (which calls
// config.Get) does not panic.
func loadConfigForTest(t *testing.T) {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, ".config"))
	t.Setenv("ANTHROPIC_API_KEY", "test")
	viper.Reset()
	if _, err := config.Load(t.TempDir(), false); err != nil {
		t.Fatalf("load config: %v", err)
	}
}
