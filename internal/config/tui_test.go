package config

import (
	"path/filepath"
	"testing"
)

func TestTUIKeybindsParse(t *testing.T) {
	home := t.TempDir()
	project := t.TempDir()
	writeConfigFile(t, filepath.Join(project, "opencode.json"), `{
		"tui": {
			"theme": "dracula",
			"leader_timeout": 1500,
			"keybinds": {
				"leader": "ctrl+x",
				"session_list": "ctrl+s",
				"session_delete": "ctrl+d",
				"session_new": "none"
			}
		}
	}`)

	loaded := loadForTest(t, home, project)
	defer resetLoad(t)

	if loaded.TUI.Theme != "dracula" {
		t.Fatalf("theme = %q", loaded.TUI.Theme)
	}
	if loaded.TUI.LeaderTimeout != 1500 {
		t.Fatalf("leader_timeout = %d, want 1500", loaded.TUI.LeaderTimeout)
	}
	kb := loaded.TUI.Keybinds
	if kb["session_delete"] != "ctrl+d" {
		t.Fatalf("session_delete = %q", kb["session_delete"])
	}
	if kb["session_new"] != "none" {
		t.Fatalf("session_new = %q", kb["session_new"])
	}
	if kb["leader"] != "ctrl+x" {
		t.Fatalf("leader = %q", kb["leader"])
	}
}

func TestTUILeaderTimeoutDefaultsToZero(t *testing.T) {
	home := t.TempDir()
	project := t.TempDir()
	writeConfigFile(t, filepath.Join(project, "opencode.json"), `{"tui": {"theme": "opencode"}}`)

	loaded := loadForTest(t, home, project)
	defer resetLoad(t)

	if loaded.TUI.LeaderTimeout != 0 {
		t.Fatalf("leader_timeout = %d, want 0 (TUI falls back to 2000ms)", loaded.TUI.LeaderTimeout)
	}
}
