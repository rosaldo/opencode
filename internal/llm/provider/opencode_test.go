package provider

import (
	"testing"

	"github.com/opencode-ai/opencode/internal/llm/models"
	"github.com/opencode-ai/opencode/internal/llm/provider/zen"
)

func TestOpencodeProviderUsesZenGateway(t *testing.T) {
	p, err := NewProvider(models.ProviderOpencode, WithAPIKey("k"), WithModel(models.SupportedModels[models.OpencodeGPT55]))
	if err != nil {
		t.Fatalf("NewProvider: %v", err)
	}
	bp, ok := p.(*baseProvider[OpenAIClient])
	if !ok {
		t.Fatalf("provider type = %T", p)
	}
	c := bp.client.(*openaiClient)
	if c.options.baseURL != "https://opencode.ai/zen/v1" {
		t.Fatalf("baseURL = %q", c.options.baseURL)
	}
}

// With an API key the Zen provider is a plain OpenAI-compatible call: it must not
// advertise the free-tier fingerprint.
func TestOpencodeProviderKeyedHasNoFingerprint(t *testing.T) {
	p, err := NewProvider(models.ProviderOpencode, WithAPIKey("k"), WithModel(models.SupportedModels[models.OpencodeGPT55]))
	if err != nil {
		t.Fatalf("NewProvider: %v", err)
	}
	c := p.(*baseProvider[OpenAIClient]).client.(*openaiClient)
	if c.options.requestOptions != nil {
		t.Fatal("keyed provider must not install free-tier request options")
	}
	if len(c.options.requiredTools) != 0 {
		t.Fatalf("keyed provider must not require free-tier tools, got %v", c.options.requiredTools)
	}
}

// Without a key the Zen provider targets the free tier and must install the
// fingerprint: required tools plus per-request session headers.
func TestOpencodeProviderFreeTierFingerprint(t *testing.T) {
	p, err := NewProvider(models.ProviderOpencode, WithAPIKey(""), WithModel(models.SupportedModels[models.OpencodeBigPickle]))
	if err != nil {
		t.Fatalf("NewProvider: %v", err)
	}
	c := p.(*baseProvider[OpenAIClient]).client.(*openaiClient)
	if c.options.requestOptions == nil {
		t.Fatal("free-tier provider must install request options")
	}
	if len(c.options.requiredTools) != len(zen.RequiredToolNames) {
		t.Fatalf("requiredTools = %v, want %v", c.options.requiredTools, zen.RequiredToolNames)
	}
	if c.options.extraHeaders["User-Agent"] != zen.UserAgent {
		t.Fatalf("User-Agent header = %q", c.options.extraHeaders["User-Agent"])
	}
	if c.options.extraHeaders["Authorization"] != "Bearer "+zen.AuthBearerValue {
		t.Fatalf("Authorization header = %q", c.options.extraHeaders["Authorization"])
	}
}

// The required-tools injector must add missing names and never duplicate a present one.
func TestInjectRequiredToolsNoDuplicates(t *testing.T) {
	c := &openaiClient{options: openaiOptions{requiredTools: zen.RequiredToolNames}}
	tools := c.convertTools(nil)
	if len(tools) != len(zen.RequiredToolNames) {
		t.Fatalf("injected %d tools, want %d", len(tools), len(zen.RequiredToolNames))
	}
	seen := map[string]int{}
	for _, tool := range tools {
		seen[tool.Function.Name]++
	}
	for _, name := range zen.RequiredToolNames {
		if seen[name] != 1 {
			t.Fatalf("tool %q present %d times, want 1", name, seen[name])
		}
	}
}

func TestOpencodeModelResolution(t *testing.T) {
	id, ok := models.ResolveModelID("opencode/gpt-5.5")
	if !ok {
		t.Fatal("opencode/gpt-5.5 did not resolve")
	}
	if id != models.OpencodeGPT55 {
		t.Fatalf("resolved %q, want %q", id, models.OpencodeGPT55)
	}
	m := models.SupportedModels[id]
	if m.Provider != models.ProviderOpencode {
		t.Fatalf("provider = %q", m.Provider)
	}
	if m.APIModel != "gpt-5.5" {
		t.Fatalf("api model = %q", m.APIModel)
	}
}
