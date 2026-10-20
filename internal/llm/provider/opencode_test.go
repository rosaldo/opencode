package provider

import (
	"testing"

	"github.com/opencode-ai/opencode/internal/llm/models"
)

func TestOpencodeProviderUsesInferenceGateway(t *testing.T) {
	p, err := NewProvider(models.ProviderOpencode, WithAPIKey("k"), WithModel(models.SupportedModels[models.OpencodeGPT55]))
	if err != nil {
		t.Fatalf("NewProvider: %v", err)
	}
	bp, ok := p.(*baseProvider[OpenAIClient])
	if !ok {
		t.Fatalf("provider type = %T", p)
	}
	c := bp.client.(*openaiClient)
	if c.options.baseURL != "https://opencode.ai/inference/openai/v1" {
		t.Fatalf("baseURL = %q", c.options.baseURL)
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
