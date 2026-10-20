package config

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/opencode-ai/opencode/internal/agentdef"
)

func writeConfigFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
}

func TestLoadConfigDefinitionsParsesAgentMap(t *testing.T) {
	dir := t.TempDir()
	writeConfigFile(t, filepath.Join(dir, ".opencode.json"), `{
		"agent": {
			"security-auditor": {
				"description": "Auditoria de segurança",
				"mode": "subagent",
				"model": "anthropic/claude-sonnet-4",
				"temperature": 0.2,
				"tools": {"edit": false}
			}
		}
	}`)

	defs := loadConfigDefinitions([]string{dir})
	if len(defs) != 1 {
		t.Fatalf("defs = %d, want 1", len(defs))
	}
	d := defs[0]
	if d.Name != "security-auditor" {
		t.Fatalf("name = %q", d.Name)
	}
	if d.Mode != agentdef.ModeSubagent {
		t.Fatalf("mode = %q", d.Mode)
	}
	if d.Model != "anthropic/claude-sonnet-4" {
		t.Fatalf("model = %q", d.Model)
	}
	if d.Tools["edit"] {
		t.Fatal("expected edit tool to be disabled")
	}
}

func TestLoadConfigDefinitionsPromptFile(t *testing.T) {
	dir := t.TempDir()
	writeConfigFile(t, filepath.Join(dir, "prompts", "audit.txt"), "Você é um auditor.")
	writeConfigFile(t, filepath.Join(dir, ".opencode.json"), `{
		"agent": {
			"auditor": {"mode": "subagent", "prompt": "{file:./prompts/audit.txt}"}
		}
	}`)

	defs := loadConfigDefinitions([]string{dir})
	if len(defs) != 1 {
		t.Fatalf("defs = %d, want 1", len(defs))
	}
	body, err := defs[0].Prompt()
	if err != nil {
		t.Fatalf("prompt: %v", err)
	}
	if body != "Você é um auditor." {
		t.Fatalf("body = %q", body)
	}
}

func TestLoadConfigDefinitionsInlinePrompt(t *testing.T) {
	dir := t.TempDir()
	writeConfigFile(t, filepath.Join(dir, ".opencode.json"), `{
		"agent": {"greeter": {"prompt": "Olá!"}}
	}`)

	defs := loadConfigDefinitions([]string{dir})
	if len(defs) != 1 {
		t.Fatalf("defs = %d, want 1", len(defs))
	}
	body, _ := defs[0].Prompt()
	if body != "Olá!" {
		t.Fatalf("body = %q", body)
	}
}

func TestLoadConfigDefinitionsLocalWins(t *testing.T) {
	global := t.TempDir()
	project := t.TempDir()
	writeConfigFile(t, filepath.Join(global, ".opencode.json"), `{
		"agent": {"audit": {"description": "global", "mode": "subagent"}}
	}`)
	writeConfigFile(t, filepath.Join(project, ".opencode.json"), `{
		"agent": {"audit": {"description": "project", "mode": "subagent"}}
	}`)

	defs := loadConfigDefinitions([]string{global, project})
	if len(defs) != 1 {
		t.Fatalf("defs = %d, want 1 (deduplicated by name)", len(defs))
	}
	if defs[0].Description != "project" {
		t.Fatalf("description = %q, want project", defs[0].Description)
	}
}

func TestLoadConfigDefinitionsInvalidModeFallsBack(t *testing.T) {
	dir := t.TempDir()
	writeConfigFile(t, filepath.Join(dir, ".opencode.json"), `{
		"agent": {"weird": {"mode": "bogus"}}
	}`)

	defs := loadConfigDefinitions([]string{dir})
	if len(defs) != 1 {
		t.Fatalf("defs = %d, want 1", len(defs))
	}
	if defs[0].Mode != agentdef.ModeAll {
		t.Fatalf("mode = %q, want all", defs[0].Mode)
	}
}

func TestLoadConfigDefinitionsMissingFile(t *testing.T) {
	if defs := loadConfigDefinitions([]string{t.TempDir()}); len(defs) != 0 {
		t.Fatalf("defs = %d, want 0", len(defs))
	}
}

func TestResolvePromptInlineAndFile(t *testing.T) {
	dir := t.TempDir()
	writeConfigFile(t, filepath.Join(dir, "p.txt"), "file prompt")
	if got := resolvePrompt("inline text", dir); got != "inline text" {
		t.Fatalf("inline = %q", got)
	}
	if got := resolvePrompt("{file:./p.txt}", dir); got != "file prompt" {
		t.Fatalf("file = %q", got)
	}
	if got := resolvePrompt("{file:./missing.txt}", dir); got != "" {
		t.Fatalf("missing = %q, want empty", got)
	}
}

func TestApplyDefaultAgent(t *testing.T) {
	reg := agentdef.NewRegistry()
	_ = reg.Register(&agentdef.Definition{Name: "coder", Mode: agentdef.ModePrimary})
	_ = reg.Register(&agentdef.Definition{Name: "auditor", Mode: agentdef.ModePrimary})
	_ = reg.Register(&agentdef.Definition{Name: "off", Mode: agentdef.ModePrimary, Disabled: true})

	// Valid name selects it.
	applyDefaultAgent(reg, "auditor")
	if got := reg.Default(); got != "auditor" {
		t.Fatalf("default = %q, want auditor", got)
	}

	// Disabled name is ignored, keeping the previous default.
	applyDefaultAgent(reg, "off")
	if got := reg.Default(); got != "auditor" {
		t.Fatalf("default = %q, want auditor", got)
	}

	// Empty is a no-op.
	applyDefaultAgent(reg, "")
	if got := reg.Default(); got != "auditor" {
		t.Fatalf("default = %q, want auditor", got)
	}
}

func TestValidateDefinitionsInheritsAndFillsTokens(t *testing.T) {
	prev := cfg
	defer func() { cfg = prev }()

	cfg = &Config{Agents: map[AgentName]Agent{
		AgentCoder: {Model: "claude-4-sonnet", MaxTokens: 5000},
	}}

	reg := agentdef.NewRegistry()
	bad := &agentdef.Definition{Name: "bad-model", Model: "does/not-exist"}
	inherit := &agentdef.Definition{Name: "no-model"}
	reg.Register(bad)
	reg.Register(inherit)
	agentdef.Set(reg)

	validateDefinitions()

	if bad.Model != "claude-4-sonnet" {
		t.Fatalf("bad model = %q, want inherited claude-4-sonnet", bad.Model)
	}
	if inherit.Model != "claude-4-sonnet" {
		t.Fatalf("inherit model = %q, want claude-4-sonnet", inherit.Model)
	}
	if bad.MaxTokens <= 0 || inherit.MaxTokens <= 0 {
		t.Fatalf("maxTokens not filled: bad=%d inherit=%d", bad.MaxTokens, inherit.MaxTokens)
	}
}
