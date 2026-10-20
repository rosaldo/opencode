package config

import (
	"path/filepath"
	"testing"
)

func TestAuxConfigParsesAndDefaults(t *testing.T) {
	home := t.TempDir()
	project := t.TempDir()
	writeConfigFile(t, filepath.Join(project, "opencode.json"), `{
		"instructions": ["AGENTS.md", "docs/*.md"],
		"commands": {
			"review": {"template": "Review this", "agent": "coder"}
		},
		"references": {
			"local": "./docs",
			"git": {"repository": "https://example.com/x.git", "branch": "main"}
		},
		"skills": {"paths": ["skills"], "urls": ["https://example.com/.well-known/skills/"]},
		"formatter": {"gofmt": {"command": ["gofmt", "-w"], "extensions": [".go"]}},
		"watcher": {"ignore": ["node_modules"]},
		"snapshots": true,
		"media": {"image": {"auto_resize": true, "max_width": 1600}},
		"tool_output": {"max_lines": 500},
		"websearch": {"provider": "exa"},
		"compaction": {"keep": {"tokens": 1000}, "buffer": 200},
		"warming": true,
		"worktree": {"directory": ".wt"},
		"update": "notify",
		"username": "rosaldo"
	}`)

	loaded := loadForTest(t, home, project)
	defer resetLoad(t)

	if len(loaded.Instructions) != 2 {
		t.Fatalf("instructions = %v", loaded.Instructions)
	}
	if got := loaded.Commands["review"].Agent; got != "coder" {
		t.Fatalf("command agent = %q", got)
	}
	if got := loaded.References["local"].Path; got != "./docs" {
		t.Fatalf("reference local path = %q", got)
	}
	if got := loaded.References["git"].Repository; got != "https://example.com/x.git" {
		t.Fatalf("reference git repo = %q", got)
	}
	if len(loaded.Skills.Paths) != 1 || len(loaded.Skills.URLs) != 1 {
		t.Fatalf("skills = %+v", loaded.Skills)
	}
	if len(loaded.Formatter.Formatters) != 1 {
		t.Fatalf("formatter = %+v", loaded.Formatter)
	}
	if got := loaded.Watcher.Ignore; len(got) != 1 || got[0] != "node_modules" {
		t.Fatalf("watcher ignore = %v", got)
	}
	if loaded.Snapshots == nil || !*loaded.Snapshots {
		t.Fatalf("snapshots = %v", loaded.Snapshots)
	}
	if loaded.Media.Image.MaxWidth != 1600 {
		t.Fatalf("media max_width = %d", loaded.Media.Image.MaxWidth)
	}
	// Defaults preserved when only one threshold is set.
	if loaded.ToolOutput.MaxLines != 500 || loaded.ToolOutput.MaxBytes != defaultToolOutputMaxBytes {
		t.Fatalf("tool_output = %+v", loaded.ToolOutput)
	}
	if loaded.Websearch.Provider != "exa" {
		t.Fatalf("websearch = %+v", loaded.Websearch)
	}
	if loaded.Compaction.Keep.Tokens != 1000 || loaded.Compaction.Buffer != 200 {
		t.Fatalf("compaction = %+v", loaded.Compaction)
	}
	if loaded.Warming.Enabled == nil || !*loaded.Warming.Enabled {
		t.Fatalf("warming = %+v", loaded.Warming)
	}
	if loaded.Worktree.Directory != ".wt" {
		t.Fatalf("worktree = %+v", loaded.Worktree)
	}
	if loaded.Update != "notify" {
		t.Fatalf("update = %q", loaded.Update)
	}
}

func TestAuxConfigPolymorphicBool(t *testing.T) {
	home := t.TempDir()
	project := t.TempDir()
	writeConfigFile(t, filepath.Join(project, "opencode.json"), `{
		"formatter": false,
		"websearch": false,
		"warming": false,
		"skills": ["a", "b"]
	}`)

	loaded := loadForTest(t, home, project)
	defer resetLoad(t)

	if loaded.Formatter.Enabled == nil || *loaded.Formatter.Enabled {
		t.Fatalf("formatter enabled = %v, want false", loaded.Formatter.Enabled)
	}
	if loaded.Websearch.Disabled == nil || !*loaded.Websearch.Disabled {
		t.Fatalf("websearch disabled = %v, want true", loaded.Websearch.Disabled)
	}
	if loaded.Warming.Enabled == nil || *loaded.Warming.Enabled {
		t.Fatalf("warming enabled = %v, want false", loaded.Warming.Enabled)
	}
	if len(loaded.Skills.Paths) != 2 {
		t.Fatalf("skills = %+v", loaded.Skills)
	}
}

func TestAuxConfigDefaults(t *testing.T) {
	home := t.TempDir()
	project := t.TempDir()
	writeConfigFile(t, filepath.Join(project, "opencode.json"), `{}`)

	loaded := loadForTest(t, home, project)
	defer resetLoad(t)

	if loaded.ToolOutput.MaxLines != defaultToolOutputMaxLines {
		t.Fatalf("default max_lines = %d", loaded.ToolOutput.MaxLines)
	}
	if loaded.ToolOutput.MaxBytes != defaultToolOutputMaxBytes {
		t.Fatalf("default max_bytes = %d", loaded.ToolOutput.MaxBytes)
	}
	if loaded.Compaction.Auto == nil || !*loaded.Compaction.Auto {
		t.Fatalf("compaction.auto = %v, want true", loaded.Compaction.Auto)
	}
}

func TestAuxConfigInterpolation(t *testing.T) {
	home := t.TempDir()
	project := t.TempDir()
	t.Setenv("OPENCODE_TEST_USER", "alice")
	writeConfigFile(t, filepath.Join(project, "opencode.json"), `{
		"username": "{env:OPENCODE_TEST_USER}",
		"commands": {"hi": {"template": "Hello {env:OPENCODE_TEST_USER}"}}
	}`)

	loaded := loadForTest(t, home, project)
	defer resetLoad(t)

	if loaded.Username != "alice" {
		t.Fatalf("username = %q", loaded.Username)
	}
	if got := loaded.Commands["hi"].Template; got != "Hello alice" {
		t.Fatalf("template = %q", got)
	}
}

func TestAuxConfigInvalidUpdateDropped(t *testing.T) {
	home := t.TempDir()
	project := t.TempDir()
	writeConfigFile(t, filepath.Join(project, "opencode.json"), `{"update": "bogus"}`)

	loaded := loadForTest(t, home, project)
	defer resetLoad(t)

	if loaded.Update != "" {
		t.Fatalf("update = %q, want empty", loaded.Update)
	}
}
