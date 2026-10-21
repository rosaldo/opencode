package config

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/opencode-ai/opencode/internal/llm/models"
)

// TestSetProviderAPIKeyPersists verifies /connect stores the key in memory and
// on disk, enables the provider and adopts its default model (Spec 12d).
func TestSetProviderAPIKeyPersists(t *testing.T) {
	home := t.TempDir()
	project := t.TempDir()
	writeConfigFile(t, filepath.Join(project, "opencode.json"), `{}`)

	loaded := loadForTest(t, home, project)
	defer resetLoad(t)

	// Drop OPENCODE_API_KEY so the key comes from the dialog, not the env.
	t.Setenv("OPENCODE_API_KEY", "")

	if err := SetProviderAPIKey(models.ProviderOpencode, "zen-secret"); err != nil {
		t.Fatalf("SetProviderAPIKey: %v", err)
	}

	if got := loaded.Providers[models.ProviderOpencode].EffectiveAPIKey(); got != "zen-secret" {
		t.Fatalf("in-memory key = %q", got)
	}
	if loaded.Providers[models.ProviderOpencode].Disabled {
		t.Fatal("provider should be enabled")
	}

	// Persisted to the user config file under HOME.
	data, err := os.ReadFile(filepath.Join(home, ".opencode.json"))
	if err != nil {
		t.Fatalf("read persisted config: %v", err)
	}
	if !contains(string(data), "zen-secret") {
		t.Fatalf("persisted config missing key: %s", data)
	}
}

func contains(haystack, needle string) bool {
	for i := 0; i+len(needle) <= len(haystack); i++ {
		if haystack[i:i+len(needle)] == needle {
			return true
		}
	}
	return false
}

// TestFriendlyDefaultIsFreeZen checks that with no credentials of any kind the
// default model falls back to OpenCode Zen's free tier (Spec 12d).
func TestFriendlyDefaultIsFreeZen(t *testing.T) {
	home := t.TempDir()
	project := t.TempDir()

	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, ".config"))
	for _, k := range []string{
		"ANTHROPIC_API_KEY", "OPENAI_API_KEY", "GEMINI_API_KEY", "GROQ_API_KEY",
		"OPENROUTER_API_KEY", "OPENCODE_API_KEY", "AZURE_OPENAI_API_KEY",
	} {
		t.Setenv(k, "")
	}
	cfg = nil
	defer resetLoad(t)

	var err error
	loaded, err := Load(project, false)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}

	if loaded.Agents[string(AgentCoder)].Model != string(models.OpencodeBigPickle) {
		t.Fatalf("coder model = %q, want %q",
			loaded.Agents[string(AgentCoder)].Model, models.OpencodeBigPickle)
	}
	if _, ok := loaded.Providers[models.ProviderOpencode]; !ok {
		t.Fatal("opencode provider not registered for the free default")
	}
}
