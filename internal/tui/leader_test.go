package tui

import (
	"strings"
	"testing"
	"time"
)

func TestLeaderHintRowsSorted(t *testing.T) {
	restore := resolvedKeybinds
	defer func() { resolvedKeybinds = restore }()

	kb, warnings := ResolveKeybinds(nil)
	if len(warnings) != 0 {
		t.Fatalf("unexpected warnings: %v", warnings)
	}
	resolvedKeybinds = kb
	applyLeaderBindings()

	rows := leaderHintRows()
	wantOrder := []string{".", "b", "c", "d", "e", "g", "n", "r", "u", "x"}
	if len(rows) != len(wantOrder) {
		t.Fatalf("rows = %q, want %d entries", rows, len(wantOrder))
	}
	for i, suffix := range wantOrder {
		if !strings.HasPrefix(rows[i], suffix) {
			t.Errorf("row[%d] = %q, want prefix %q", i, rows[i], suffix)
		}
	}
	if !strings.Contains(rows[0], "redo message") {
		t.Errorf("row for . = %q, want the redo label", rows[0])
	}
	if !strings.Contains(rows[1], "toggle sidebar") {
		t.Errorf("row for b = %q, want the sidebar label", rows[1])
	}
	if !strings.Contains(rows[4], "open editor") {
		t.Errorf("row for e = %q, want the editor label", rows[4])
	}
	if !strings.Contains(rows[5], "jump to message") {
		t.Errorf("row for g = %q, want the timeline label", rows[5])
	}
	if !strings.Contains(rows[7], "rename session") {
		t.Errorf("row for r = %q, want the rename label", rows[7])
	}
	if !strings.Contains(rows[8], "undo message") {
		t.Errorf("row for u = %q, want the undo label", rows[8])
	}
	if !strings.Contains(rows[9], "export session") {
		t.Errorf("row for x = %q, want the export label", rows[9])
	}
}

func TestLeaderHintRowsFallbackWhenNothingBound(t *testing.T) {
	restore := resolvedKeybinds
	// Rebuild from the restored keybinds so later tests see the defaults.
	defer func() {
		resolvedKeybinds = restore
		applyLeaderBindings()
	}()

	resolvedKeybinds = Keybinds{}
	applyLeaderBindings()

	rows := leaderHintRows()
	if len(rows) != 1 || rows[0] != "no leader actions bound" {
		t.Errorf("rows = %q, want the empty fallback", rows)
	}
}

func TestLeaderTimeoutFromConfig(t *testing.T) {
	if got := leaderTimeoutFromConfig(); got != defaultLeaderTimeout {
		t.Errorf("unset config = %v, want %v", got, defaultLeaderTimeout)
	}
	if defaultLeaderTimeout != 2*time.Second {
		t.Errorf("defaultLeaderTimeout = %v, want 2s (tui.json parity)", defaultLeaderTimeout)
	}
}

func TestLeaderSuffixesIncludeForkWhenBound(t *testing.T) {
	restore := resolvedKeybinds
	defer func() {
		resolvedKeybinds = restore
		applyLeaderBindings()
	}()

	kb, warnings := ResolveKeybinds(map[string]string{"session_fork": "<leader>f"})
	if len(warnings) != 0 {
		t.Fatalf("unexpected warnings: %v", warnings)
	}
	resolvedKeybinds = kb
	applyLeaderBindings()

	rows := leaderHintRows()
	found := false
	for _, row := range rows {
		if strings.HasPrefix(row, "f ") && strings.Contains(row, "fork session") {
			found = true
		}
	}
	if !found {
		t.Errorf("rows = %q, want the fork suffix when session_fork is bound", rows)
	}
	if action, ok := leaderSuffixes["f"]; !ok || action != leaderForkSession {
		t.Errorf("leaderSuffixes[f] = %v,%v, want leaderForkSession", action, ok)
	}
}

func TestLeaderSuffixesIncludeStashWhenBound(t *testing.T) {
	restore := resolvedKeybinds
	defer func() {
		resolvedKeybinds = restore
		applyLeaderBindings()
	}()

	kb, warnings := ResolveKeybinds(map[string]string{
		"prompt_stash":      "<leader>i",
		"prompt_stash_pop":  "<leader>o",
		"prompt_stash_list": "<leader>p",
	})
	if len(warnings) != 0 {
		t.Fatalf("unexpected warnings: %v", warnings)
	}
	resolvedKeybinds = kb
	applyLeaderBindings()

	want := map[string]struct {
		action leaderAction
		label  string
	}{
		"i": {leaderStash, "stash prompt"},
		"o": {leaderStashPop, "pop stash"},
		"p": {leaderStashList, "list stash"},
	}
	rows := leaderHintRows()
	for suffix, want := range want {
		action, ok := leaderSuffixes[suffix]
		if !ok || action != want.action {
			t.Errorf("leaderSuffixes[%q] = %v,%v, want %v", suffix, action, ok, want.action)
		}
		found := false
		for _, row := range rows {
			if strings.HasPrefix(row, suffix+" ") && strings.Contains(row, want.label) {
				found = true
			}
		}
		if !found {
			t.Errorf("rows = %q, want suffix %q with label %q", rows, suffix, want.label)
		}
	}
}
