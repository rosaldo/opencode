package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/opencode-ai/opencode/internal/agentdef"
	"github.com/spf13/viper"
)

func TestAncestorsRootFirst(t *testing.T) {
	got := ancestorsRootFirst("/a/b/c")
	want := []string{"/", "/a", "/a/b", "/a/b/c"}
	if len(got) != len(want) {
		t.Fatalf("ancestors = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("ancestors = %v, want %v", got, want)
		}
	}
}

func TestFilesInOrderJSONBeforeJSONC(t *testing.T) {
	dir := t.TempDir()
	writeConfigFile(t, filepath.Join(dir, "opencode.json"), "{}")
	writeConfigFile(t, filepath.Join(dir, "opencode.jsonc"), "{}")

	files := filesIn(dir)
	if len(files) != 2 {
		t.Fatalf("files = %v, want 2", files)
	}
	if got := filepath.Base(files[0].path); got != "opencode.json" {
		t.Fatalf("first = %q, want opencode.json", got)
	}
	if got := filepath.Base(files[1].path); got != "opencode.jsonc" {
		t.Fatalf("second = %q, want opencode.jsonc", got)
	}
}

// TestConfigFilesPrecedence builds an isolated directory tree and proves the v2
// ordering: direct opencode.json at an ancestor < direct at workingDir <
// .opencode/opencode.json at workingDir. Only files under the temp tree are
// considered (global/system files elsewhere on the machine are ignored).
func TestConfigFilesPrecedence(t *testing.T) {
	root := t.TempDir()
	project := filepath.Join(root, "project")
	if err := os.MkdirAll(project, 0o755); err != nil {
		t.Fatal(err)
	}

	writeConfigFile(t, filepath.Join(root, "opencode.json"), "{}")
	writeConfigFile(t, filepath.Join(project, "opencode.json"), "{}")
	writeConfigFile(t, filepath.Join(project, defaultDataDirectory, "opencode.json"), "{}")

	files := configFiles(project)

	// Keep only files under the temp tree, in discovery order.
	var rel []string
	for _, f := range files {
		r, err := filepath.Rel(root, f.path)
		if err == nil && r != ".." && !strings.HasPrefix(r, ".."+string(filepath.Separator)) {
			rel = append(rel, r)
		}
	}

	// root/opencode.json is an ancestor of project, so it comes before both
	// project files; .opencode wins over the direct file at the same level.
	wantOrder := []string{"opencode.json", filepath.Join("project", "opencode.json"), filepath.Join("project", defaultDataDirectory, "opencode.json")}
	if len(rel) != len(wantOrder) {
		t.Fatalf("files = %v, want %v", rel, wantOrder)
	}
	for i := range wantOrder {
		if rel[i] != wantOrder[i] {
			t.Fatalf("precedence = %v, want %v", rel, wantOrder)
		}
	}
}

// TestV2JSONCFileLoads proves that opencode.jsonc (with comments) is read and
// that its agent declarations reach the registry.
func TestV2JSONCFileLoads(t *testing.T) {
	dir := t.TempDir()
	writeConfigFile(t, filepath.Join(dir, "opencode.jsonc"), `{
		// a security auditor, declared inline
		"agents": {
			"auditor": {
				"mode": "subagent",
				"system": "You audit code.",
				"permissions": [ { "action": "shell", "effect": "deny" }, ],
			},
		},
	}`)

	sources := jsonSources(dir)
	if len(sources) == 0 {
		t.Fatal("expected at least one source from opencode.jsonc")
	}
	reg, err := agentdef.Load(sources...)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	def, ok := reg.Get("auditor")
	if !ok {
		t.Fatal("expected auditor from opencode.jsonc")
	}
	body, _ := def.Prompt()
	if body != "You audit code." {
		t.Fatalf("prompt = %q", body)
	}
	if len(def.Permissions) != 1 || def.Permissions[0].Effect != "deny" {
		t.Fatalf("permissions = %+v", def.Permissions)
	}
}

// TestV2InterpolatesAgentStrings proves {env:} and {file:} are applied to agent
// strings (not just the prompt).
func TestV2InterpolatesAgentStrings(t *testing.T) {
	dir := t.TempDir()
	writeConfigFile(t, filepath.Join(dir, "team.txt"), "security")
	t.Setenv("OPENCODE_TEST_DESC", "auditoria")

	writeConfigFile(t, filepath.Join(dir, "opencode.json"), `{
		"agents": {
			"auditor": {
				"description": "{env:OPENCODE_TEST_DESC}",
				"team": "{file:./team.txt}",
				"mode": "subagent",
				"system": "x"
			}
		}
	}`)

	agents, err := readAgentConfigsFrom(filepath.Join(dir, "opencode.json"))
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	got := interpolateAgentConfigs(agents, dir)["auditor"]
	if got.Description != "auditoria" {
		t.Fatalf("description = %q", got.Description)
	}
	if got.Team != "security" {
		t.Fatalf("team = %q", got.Team)
	}
}

// TestLoadV2AndLegacyPrecedence exercises the full Load path with an isolated
// HOME: a global legacy file, a project v2 file and a project .opencode file,
// proving precedence and that non-agent top-level keys from a v2 file are read.
func TestLoadV2AndLegacyPrecedence(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, ".config"))
	// Give the loader a provider so agent validation succeeds in the test env.
	t.Setenv("ANTHROPIC_API_KEY", "test-key")

	// Global legacy config (v1 "defaultAgent" spelling, read as deprecated).
	writeConfigFile(t, filepath.Join(home, legacyConfigFilename), `{
		"defaultAgent": "coder",
		"agents": {"auditor": {"description": "global", "mode": "subagent", "system": "g"}}
	}`)

	project := t.TempDir()
	// Project v2 file wins over the global legacy file.
	writeConfigFile(t, filepath.Join(project, "opencode.json"), `{
		"default_agent": "task",
		"agents": {"auditor": {"description": "project", "mode": "subagent", "system": "p"}}
	}`)
	// Same-level .opencode file wins over the direct project v2 file.
	writeConfigFile(t, filepath.Join(project, defaultDataDirectory, "opencode.json"), `{
		"default_agent": "coder"
	}`)

	// Isolate global state.
	cfg = nil
	viper.Reset()

	loaded, err := Load(project, false)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if loaded.effectiveDefaultAgent() != "coder" {
		t.Fatalf("default agent = %q, want coder", loaded.effectiveDefaultAgent())
	}
	def, ok := agentdef.Get().Get("auditor")
	if !ok {
		t.Fatal("expected auditor agent after Load")
	}
	if def.Description != "project" {
		t.Fatalf("auditor description = %q, want project", def.Description)
	}

	cfg = nil
	viper.Reset()
}
