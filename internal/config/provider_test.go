package config

import (
	"path/filepath"
	"testing"

	"github.com/opencode-ai/opencode/internal/llm/models"
)

func TestProviderSettingsAndModelsParse(t *testing.T) {
	home := t.TempDir()
	project := t.TempDir()
	writeConfigFile(t, filepath.Join(project, "opencode.json"), `{
		"providers": {
			"openai": {
				"settings": {
					"apiKey": "sk-test",
					"baseURL": "https://proxy.example/v1",
					"headers": {"X-Tenant": "acme"},
					"transport": "websocket"
				},
				"models": {
					"gpt-5.2-custom": {
						"modelID": "gpt-5.2",
						"name": "GPT-5.2 Custom",
						"limit": {"context": 200000, "output": 32000}
					}
				}
			}
		}
	}`)

	loaded := loadForTest(t, home, project)
	defer resetLoad(t)

	p := loaded.Providers[models.ProviderOpenAI]
	if p.EffectiveAPIKey() != "sk-test" {
		t.Fatalf("api key = %q", p.EffectiveAPIKey())
	}
	if p.Settings.BaseURL != "https://proxy.example/v1" {
		t.Fatalf("baseURL = %q", p.Settings.BaseURL)
	}
	if p.Settings.Headers["X-Tenant"] != "acme" {
		t.Fatalf("headers = %v", p.Settings.Headers)
	}
	if p.Settings.Transport != "websocket" {
		t.Fatalf("transport = %q", p.Settings.Transport)
	}
	if len(p.Models) != 1 {
		t.Fatalf("models = %v", p.Models)
	}

	// The model is merged into the catalog and resolves via provider/id.
	id, ok := models.ResolveModelID("openai/gpt-5.2-custom")
	if !ok {
		t.Fatal("expected custom model to resolve")
	}
	m := models.SupportedModels[id]
	if m.ContextWindow != 200000 || m.DefaultMaxTokens != 32000 {
		t.Fatalf("limits = context %d output %d", m.ContextWindow, m.DefaultMaxTokens)
	}
	if m.APIModel != "gpt-5.2" {
		t.Fatalf("api model = %q", m.APIModel)
	}
}

func TestProviderFlatAPIKeyBackCompat(t *testing.T) {
	home := t.TempDir()
	project := t.TempDir()
	writeConfigFile(t, filepath.Join(project, "opencode.json"), `{
		"providers": {"openai": {"apiKey": "sk-flat"}}
	}`)

	loaded := loadForTest(t, home, project)
	defer resetLoad(t)

	if got := loaded.Providers[models.ProviderOpenAI].EffectiveAPIKey(); got != "sk-flat" {
		t.Fatalf("api key = %q, want sk-flat", got)
	}
}

func TestCustomModelSelectableByTopLevelModel(t *testing.T) {
	home := t.TempDir()
	project := t.TempDir()
	writeConfigFile(t, filepath.Join(project, "opencode.json"), `{
		"model": "openai/gpt-5.2-custom",
		"providers": {
			"openai": {
				"settings": {"apiKey": "sk-test"},
				"models": {"gpt-5.2-custom": {"modelID": "gpt-5.2"}}
			}
		}
	}`)

	loaded := loadForTest(t, home, project)
	defer resetLoad(t)

	if got := loaded.Agents[string(AgentCoder)].Model; got != "openai.gpt-5.2-custom" {
		t.Fatalf("coder model = %q, want openai.gpt-5.2-custom", got)
	}
}
