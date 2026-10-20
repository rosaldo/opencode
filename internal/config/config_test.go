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

func TestReadAgentConfigsParsesAgentsMap(t *testing.T) {
	dir := t.TempDir()
	writeConfigFile(t, filepath.Join(dir, ".opencode.json"), `{
		"agents": {
			"security-auditor": {
				"description": "Auditoria de segurança",
				"mode": "subagent",
				"model": "anthropic/claude-sonnet-4",
				"temperature": 0.2,
				"tools": {"edit": false}
			}
		}
	}`)

	agents, err := readAgentConfigs(dir)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	def, ok := agents["security-auditor"]
	if !ok {
		t.Fatalf("agents = %v, want security-auditor", agents)
	}
	if def.Mode != "subagent" {
		t.Fatalf("mode = %q", def.Mode)
	}
	if def.Model != "anthropic/claude-sonnet-4" {
		t.Fatalf("model = %q", def.Model)
	}
	if def.Tools["edit"] {
		t.Fatal("expected edit tool to be disabled")
	}

	reg, err := agentdef.Load(agentdef.JSONSource{Agents: agents, BaseDir: dir, ResolvePrompt: resolvePrompt})
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if _, ok := reg.Get("security-auditor"); !ok {
		t.Fatal("expected security-auditor in registry")
	}
}

func TestReadAgentConfigsDeprecatedAgentKey(t *testing.T) {
	dir := t.TempDir()
	writeConfigFile(t, filepath.Join(dir, ".opencode.json"), `{
		"agent": {
			"auditor": {"mode": "subagent", "system": "Você é um auditor."}
		}
	}`)

	agents, err := readAgentConfigs(dir)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if _, ok := agents["auditor"]; !ok {
		t.Fatalf("deprecated \"agent\" key must still load, got %v", agents)
	}
}

func TestJSONSourcePromptFile(t *testing.T) {
	dir := t.TempDir()
	writeConfigFile(t, filepath.Join(dir, "prompts", "audit.txt"), "Você é um auditor.")
	writeConfigFile(t, filepath.Join(dir, ".opencode.json"), `{
		"agents": {
			"auditor": {"mode": "subagent", "system": "{file:./prompts/audit.txt}"}
		}
	}`)

	agents, _ := readAgentConfigs(dir)
	reg, err := agentdef.Load(agentdef.JSONSource{Agents: agents, BaseDir: dir, ResolvePrompt: resolvePrompt})
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	def, _ := reg.Get("auditor")
	body, err := def.Prompt()
	if err != nil {
		t.Fatalf("prompt: %v", err)
	}
	if body != "Você é um auditor." {
		t.Fatalf("body = %q", body)
	}
}

func TestJSONSourceInlinePrompt(t *testing.T) {
	dir := t.TempDir()
	writeConfigFile(t, filepath.Join(dir, ".opencode.json"), `{
		"agents": {"greeter": {"system": "Olá!"}}
	}`)

	agents, _ := readAgentConfigs(dir)
	reg, err := agentdef.Load(agentdef.JSONSource{Agents: agents, BaseDir: dir, ResolvePrompt: resolvePrompt})
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	def, _ := reg.Get("greeter")
	body, _ := def.Prompt()
	if body != "Olá!" {
		t.Fatalf("body = %q", body)
	}
}

func TestJSONSourcesLocalWins(t *testing.T) {
	global := t.TempDir()
	project := t.TempDir()
	writeConfigFile(t, filepath.Join(global, ".opencode.json"), `{
		"agents": {"audit": {"description": "global", "mode": "subagent"}}
	}`)
	writeConfigFile(t, filepath.Join(project, ".opencode.json"), `{
		"agents": {"audit": {"description": "project", "mode": "subagent"}}
	}`)

	reg, err := agentdef.Load(jsonSources([]string{global, project})...)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	def, ok := reg.Get("audit")
	if !ok {
		t.Fatal("expected audit agent")
	}
	if def.Description != "project" {
		t.Fatalf("description = %q, want project", def.Description)
	}
}

func TestJSONSourceInvalidModeFallsBack(t *testing.T) {
	dir := t.TempDir()
	writeConfigFile(t, filepath.Join(dir, ".opencode.json"), `{
		"agents": {"weird": {"mode": "bogus"}}
	}`)

	agents, _ := readAgentConfigs(dir)
	reg, err := agentdef.Load(agentdef.JSONSource{Agents: agents, BaseDir: dir})
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	def, ok := reg.Get("weird")
	if !ok {
		t.Fatal("expected weird agent")
	}
	if def.Mode != agentdef.ModeAll {
		t.Fatalf("mode = %q, want all", def.Mode)
	}
}

func TestReadAgentConfigsMissingFile(t *testing.T) {
	agents, err := readAgentConfigs(t.TempDir())
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if len(agents) != 0 {
		t.Fatalf("agents = %v, want empty", agents)
	}
	if len(jsonSources([]string{t.TempDir()})) != 0 {
		t.Fatal("missing config must yield no sources")
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

	cfg = &Config{Agents: map[string]AgentDefinition{
		string(AgentCoder): {Model: "claude-4-sonnet", MaxTokens: 5000},
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
