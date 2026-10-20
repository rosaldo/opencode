package agentdef

import (
	"os"
	"path/filepath"
	"testing"
)

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
}

func TestLoadDiscoversAgentDirs(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "agent", "review.md"), "---\ndescription: review\nmode: subagent\n---\nReview body")
	writeFile(t, filepath.Join(dir, "agents", "docs.md"), "Docs body only")

	reg, err := Load(Options{GlobalDirs: []string{dir}})
	if err != nil {
		t.Fatalf("load: %v", err)
	}

	review, ok := reg.Get("review")
	if !ok {
		t.Fatal("expected review agent")
	}
	if review.Mode != ModeSubagent {
		t.Fatalf("mode = %q", review.Mode)
	}
	if review.Description != "review" {
		t.Fatalf("description = %q", review.Description)
	}

	docs, ok := reg.Get("docs")
	if !ok {
		t.Fatal("expected docs agent")
	}
	if docs.Mode != ModeAll {
		t.Fatalf("default mode = %q", docs.Mode)
	}
}

func TestFrontmatterNameOverridesFile(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "agent", "file.md"), "---\nname: custom\n---\nbody")

	reg, _ := Load(Options{GlobalDirs: []string{dir}})
	if _, ok := reg.Get("custom"); !ok {
		t.Fatal("expected custom name from frontmatter")
	}
	if _, ok := reg.Get("file"); ok {
		t.Fatal("file name should not be registered when frontmatter name is set")
	}
}

func TestTeamComesFromDirectoryOrFrontmatter(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "agent", "root.md"), "no team here")
	writeFile(t, filepath.Join(dir, "agent", "security", "audit.md"), "in security")
	writeFile(t, filepath.Join(dir, "agent", "security", "deep", "nested.md"), "nested path")
	writeFile(t, filepath.Join(dir, "agents", "override.md"), "---\nteam: ops\n---\nfrontmatter team wins")

	reg, err := Load(Options{GlobalDirs: []string{dir}})
	if err != nil {
		t.Fatalf("load: %v", err)
	}

	cases := map[string]string{
		"root":     "",
		"audit":    "security",
		"nested":   "security/deep",
		"override": "ops",
	}
	for name, want := range cases {
		def, ok := reg.Get(name)
		if !ok {
			t.Fatalf("expected %s agent", name)
		}
		if def.Team != want {
			t.Errorf("%s team = %q, want %q", name, def.Team, want)
		}
	}
}

func TestPrecedenceProjectOverGlobalOverBuiltin(t *testing.T) {
	global := t.TempDir()
	project := t.TempDir()
	writeFile(t, filepath.Join(global, "agent", "coder.md"), "---\ndescription: global\n---\ng")
	writeFile(t, filepath.Join(project, "agent", "coder.md"), "---\ndescription: project\n---\np")

	reg, err := Load(Options{
		GlobalDirs: []string{global},
		ProjectDir: project,
		Builtins:   []*Definition{{Name: "coder", Description: "builtin", Mode: ModePrimary, Builtin: true}},
	})
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	coder, _ := reg.Get("coder")
	if coder.Description != "project" {
		t.Fatalf("expected project to win, got %q", coder.Description)
	}
	if coder.Builtin {
		t.Fatal("project override should replace the builtin")
	}
}

func TestPromptIsLazy(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "agent", "lazy.md")
	writeFile(t, path, "---\ndescription: lazy\n---\nHello lazy prompt")

	reg, err := Load(Options{GlobalDirs: []string{dir}})
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	def, _ := reg.Get("lazy")

	// Removing the file proves the body was not read during Load.
	if err := os.Remove(path); err != nil {
		t.Fatalf("remove: %v", err)
	}
	if _, err := def.Prompt(); err == nil {
		t.Fatal("expected error reading missing file, body was read eagerly")
	}
}

func TestPromptReturnsBody(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "agent", "greeter.md"), "---\ndescription: greeter\n---\nYou greet people.")

	reg, _ := Load(Options{GlobalDirs: []string{dir}})
	def, _ := reg.Get("greeter")
	body, err := def.Prompt()
	if err != nil {
		t.Fatalf("prompt: %v", err)
	}
	if body != "You greet people." {
		t.Fatalf("body = %q", body)
	}
}

func TestInvalidFrontmatterIsSkipped(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "agent", "bad.md"), "---\nmode: [not, a, string\n---\nbody")
	writeFile(t, filepath.Join(dir, "agent", "good.md"), "---\ndescription: ok\n---\nbody")

	reg, err := Load(Options{GlobalDirs: []string{dir}})
	if err != nil {
		t.Fatalf("load should not fail: %v", err)
	}
	if _, ok := reg.Get("bad"); ok {
		t.Fatal("invalid frontmatter should be skipped")
	}
	if _, ok := reg.Get("good"); !ok {
		t.Fatal("valid agent should be loaded")
	}
}

func TestListByMode(t *testing.T) {
	reg := NewRegistry()
	_ = reg.Register(&Definition{Name: "build", Mode: ModePrimary})
	_ = reg.Register(&Definition{Name: "task", Mode: ModeSubagent, Hidden: true})
	_ = reg.Register(&Definition{Name: "any", Mode: ModeAll})
	_ = reg.Register(&Definition{Name: "off", Mode: ModePrimary, Disabled: true})

	primary := reg.ListByMode(ModePrimary, false)
	if len(primary) != 2 {
		t.Fatalf("primary count = %d, want 2", len(primary))
	}
	sub := reg.ListByMode(ModeSubagent, true)
	if len(sub) != 2 {
		t.Fatalf("subagent (incl hidden) count = %d, want 2", len(sub))
	}
}

func TestSplitFrontmatter(t *testing.T) {
	header, body := SplitFrontmatter("---\na: 1\n---\nhello")
	if header != "a: 1" || body != "hello" {
		t.Fatalf("header=%q body=%q", header, body)
	}
	header, body = SplitFrontmatter("no frontmatter")
	if header != "" || body != "no frontmatter" {
		t.Fatalf("header=%q body=%q", header, body)
	}
}

func TestConfigDefinitionsWinAtSameLevel(t *testing.T) {
	global := t.TempDir()
	project := t.TempDir()
	// Both levels have a markdown agent with the same name as the JSON def.
	writeFile(t, filepath.Join(global, "agent", "audit.md"), "---\ndescription: md-global\n---\nbody")
	writeFile(t, filepath.Join(project, "agent", "audit.md"), "---\ndescription: md-project\n---\nbody")

	reg, err := Load(Options{
		GlobalDirs: []string{global},
		GlobalDefinitions: []*Definition{
			{Name: "audit", Description: "json-global", Mode: ModeSubagent},
		},
		ProjectDir: project,
	})
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	audit, _ := reg.Get("audit")
	// Project markdown outranks global JSON.
	if audit.Description != "md-project" {
		t.Fatalf("description = %q, want md-project", audit.Description)
	}

	reg, err = Load(Options{
		GlobalDirs: []string{global},
		ProjectDir: project,
		ProjectDefinitions: []*Definition{
			{Name: "audit", Description: "json-project", Mode: ModeSubagent},
		},
	})
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	audit, _ = reg.Get("audit")
	// Project JSON outranks project markdown.
	if audit.Description != "json-project" {
		t.Fatalf("description = %q, want json-project", audit.Description)
	}
}
