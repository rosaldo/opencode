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
	wantOrder := []string{".", "b", "c", "d", "e", "n", "r", "u", "x", "y"}
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
	if !strings.Contains(rows[6], "rename session") {
		t.Errorf("row for r = %q, want the rename label", rows[6])
	}
	if !strings.Contains(rows[7], "undo message") {
		t.Errorf("row for u = %q, want the undo label", rows[7])
	}
	if !strings.Contains(rows[8], "export session") {
		t.Errorf("row for x = %q, want the export label", rows[8])
	}
	if !strings.Contains(rows[9], "copy message") {
		t.Errorf("row for y = %q, want the copy label", rows[9])
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
