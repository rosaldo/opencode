package config

import (
	"path/filepath"
	"testing"

	"github.com/opencode-ai/opencode/internal/agentdef"
	"github.com/spf13/viper"
)

func loadForTest(t *testing.T, home string, project string) *Config {
	t.Helper()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, ".config"))
	t.Setenv("ANTHROPIC_API_KEY", "test-key")
	t.Setenv("OPENAI_API_KEY", "test-key")
	cfg = nil
	viper.Reset()
	loaded, err := Load(project, false)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	return loaded
}

func resetLoad(t *testing.T) {
	t.Helper()
	cfg = nil
	viper.Reset()
}

func TestModelSelectionPrimary(t *testing.T) {
	home := t.TempDir()
	project := t.TempDir()
	writeConfigFile(t, filepath.Join(project, "opencode.json"), `{
		"model": "openai/gpt-4.1"
	}`)

	loaded := loadForTest(t, home, project)
	defer resetLoad(t)

	if got := loaded.Agents[string(AgentCoder)].Model; got != "gpt-4.1" {
		t.Fatalf("coder model = %q, want gpt-4.1", got)
	}
}

func TestModelSelectionDefaultAgent(t *testing.T) {
	home := t.TempDir()
	project := t.TempDir()
	writeConfigFile(t, filepath.Join(project, "opencode.json"), `{
		"default_agent": "task",
		"model": "anthropic/claude-4-sonnet"
	}`)

	loaded := loadForTest(t, home, project)
	defer resetLoad(t)

	if got := loaded.Agents[string(AgentCoder)].Model; got != "claude-4-sonnet" {
		t.Fatalf("coder model (primary=task) = %q, want claude-4-sonnet", got)
	}
	// The configured default agent is the primary, so it also gets the model.
	if got := loaded.Agents[string(AgentTask)].Model; got != "claude-4-sonnet" {
		t.Fatalf("task model = %q, want claude-4-sonnet", got)
	}
}

func TestSmallModelSelection(t *testing.T) {
	home := t.TempDir()
	project := t.TempDir()
	writeConfigFile(t, filepath.Join(project, "opencode.json"), `{
		"model": "anthropic/claude-4-sonnet",
		"small_model": "anthropic/claude-3.5-haiku"
	}`)

	loaded := loadForTest(t, home, project)
	defer resetLoad(t)

	if got := loaded.Agents[string(AgentTitle)].Model; got != "claude-3.5-haiku" {
		t.Fatalf("title model = %q, want claude-3.5-haiku", got)
	}
	if got := loaded.Agents[string(AgentSummarizer)].Model; got != "claude-3.5-haiku" {
		t.Fatalf("summarizer model = %q, want claude-3.5-haiku", got)
	}
}

func TestExplicitAgentModelWinsOverTopLevel(t *testing.T) {
	home := t.TempDir()
	project := t.TempDir()
	writeConfigFile(t, filepath.Join(project, "opencode.json"), `{
		"model": "anthropic/claude-4-sonnet",
		"small_model": "anthropic/claude-3.5-haiku",
		"agents": {
			"coder": {"model": "openai/gpt-4.1"},
			"title": {"model": "openai/gpt-4.1-mini", "mode": "subagent"}
		}
	}`)

	loaded := loadForTest(t, home, project)
	defer resetLoad(t)

	if got := loaded.Agents["coder"].Model; got != "gpt-4.1" {
		t.Fatalf("coder model = %q, want gpt-4.1 (explicit wins)", got)
	}
	if got := loaded.Agents["title"].Model; got != "gpt-4.1-mini" {
		t.Fatalf("title model = %q, want gpt-4.1-mini (explicit wins)", got)
	}
}

func TestUnsupportedTopLevelModelIgnored(t *testing.T) {
	home := t.TempDir()
	project := t.TempDir()
	writeConfigFile(t, filepath.Join(project, "opencode.json"), `{
		"model": "nope/does-not-exist"
	}`)

	loaded := loadForTest(t, home, project)
	defer resetLoad(t)

	if got := loaded.Agents[string(AgentCoder)].Model; got == "does-not-exist" || got == "" {
		t.Fatalf("coder model = %q, want a valid provider default", got)
	}
	_ = agentdef.Get()
}
