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
	wantOrder := []string{"b", "c", "d", "n", "r", "x", "y"}
	if len(rows) != len(wantOrder) {
		t.Fatalf("rows = %q, want %d entries", rows, len(wantOrder))
	}
	for i, suffix := range wantOrder {
		if !strings.HasPrefix(rows[i], suffix) {
			t.Errorf("row[%d] = %q, want prefix %q", i, rows[i], suffix)
		}
	}
	if !strings.Contains(rows[0], "toggle sidebar") {
		t.Errorf("row for b = %q, want the sidebar label", rows[0])
	}
	if !strings.Contains(rows[4], "rename session") {
		t.Errorf("row for r = %q, want the rename label", rows[4])
	}
	if !strings.Contains(rows[5], "export session") {
		t.Errorf("row for x = %q, want the export label", rows[5])
	}
	if !strings.Contains(rows[6], "copy message") {
		t.Errorf("row for y = %q, want the copy label", rows[6])
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
