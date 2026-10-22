package dialog

import (
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	utilComponents "github.com/opencode-ai/opencode/internal/tui/components/util"
)

func TestForkItemsFullSessionFirstThenNewest(t *testing.T) {
	items := ForkItems(testTimelineMsgs())
	if len(items) != 3 {
		t.Fatalf("items = %d, want full session + 2 user messages", len(items))
	}
	// "Full session" leads with no boundary and no timestamp (Spec 09p).
	if items[0].MessageID != "" || items[0].Title != "Full session" {
		t.Errorf("first row = %#v, want the full-session row", items[0])
	}
	if !items[0].CreatedAt.IsZero() {
		t.Errorf("full-session CreatedAt = %v, want zero", items[0].CreatedAt)
	}
	if items[1].MessageID != "m3" || items[2].MessageID != "m1" {
		t.Errorf("order = %s,%s, want m3,m1 (newest first)", items[1].MessageID, items[2].MessageID)
	}
}

func newTestForkDialog(t *testing.T) *forkDialogCmp {
	t.Helper()
	c, ok := NewForkDialogCmp().(*forkDialogCmp)
	if !ok {
		t.Fatal("NewForkDialogCmp must return *forkDialogCmp")
	}
	c.SetItems(ForkItems(testTimelineMsgs()))
	return c
}

func TestForkDialogEnterForksFullSessionByDefault(t *testing.T) {
	c := newTestForkDialog(t)

	_, cmd := c.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if cmd == nil {
		t.Fatal("enter must fork the selected row")
	}
	msg, ok := cmd().(ForkMsg)
	if !ok {
		t.Fatalf("msg = %T, want ForkMsg", cmd())
	}
	// The list opens with "Full session" selected (Spec 09p).
	if msg.MessageID != "" {
		t.Errorf("MessageID = %q, want empty (full session)", msg.MessageID)
	}
}

func TestForkDialogEnterForksUpToMessage(t *testing.T) {
	c := newTestForkDialog(t)

	// Move past "Full session" to the newest user message.
	c.Update(tea.KeyMsg{Type: tea.KeyDown})
	_, cmd := c.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if cmd == nil {
		t.Fatal("enter must fork the selected row")
	}
	msg, ok := cmd().(ForkMsg)
	if !ok {
		t.Fatalf("msg = %T, want ForkMsg", cmd())
	}
	if msg.MessageID != "m3" {
		t.Errorf("MessageID = %q, want m3", msg.MessageID)
	}
}

func TestForkDialogEscapeCloses(t *testing.T) {
	c := newTestForkDialog(t)

	_, cmd := c.Update(tea.KeyMsg{Type: tea.KeyEsc})
	if cmd == nil {
		t.Fatal("escape must close the dialog")
	}
	if _, ok := cmd().(CloseForkDialogMsg); !ok {
		t.Fatalf("msg = %T, want CloseForkDialogMsg", cmd())
	}
}

func TestForkDialogListActivatedForksLikeEnter(t *testing.T) {
	c := newTestForkDialog(t)

	// The click already moved the selection (Spec 09n); Down is the keyboard
	// equivalent of that move.
	c.Update(tea.KeyMsg{Type: tea.KeyDown})
	_, cmd := c.Update(utilComponents.ListActivatedMsg{Index: 1})
	if cmd == nil {
		t.Fatal("ListActivatedMsg must fork like Enter")
	}
	msg, ok := cmd().(ForkMsg)
	if !ok {
		t.Fatalf("msg = %T, want ForkMsg", cmd())
	}
	if msg.MessageID != "m3" {
		t.Errorf("MessageID = %q, want m3", msg.MessageID)
	}
}

func TestForkDialogPendingSwallowsKeysAndClicks(t *testing.T) {
	c := newTestForkDialog(t)
	c.SetPending(true)

	if _, cmd := c.Update(tea.KeyMsg{Type: tea.KeyEnter}); cmd != nil {
		t.Error("enter must be ignored while the fork is in flight")
	}
	if _, cmd := c.Update(tea.KeyMsg{Type: tea.KeyEsc}); cmd != nil {
		t.Error("escape must be ignored while the fork is in flight")
	}
	if _, cmd := c.Update(utilComponents.ListActivatedMsg{Index: 1}); cmd != nil {
		t.Error("clicks must be ignored while the fork is in flight")
	}
	if !c.pending {
		t.Fatal("pending state must stick")
	}

	// Reloading the items clears the pending flag (Spec 09p).
	c.SetItems(ForkItems(testTimelineMsgs()))
	if c.pending {
		t.Error("SetItems must reset the pending state")
	}
}

func TestForkDialogPendingViewShowsProgress(t *testing.T) {
	c := newTestForkDialog(t)
	c.SetPending(true)

	out := c.View()
	if !strings.Contains(out, "Forking session") {
		t.Errorf("pending view = %q, want the progress text", out)
	}
	if strings.Contains(out, "Full session") {
		t.Errorf("pending view must hide the rows, got %q", out)
	}
}

func TestForkDialogSearchNarrows(t *testing.T) {
	c := newTestForkDialog(t)

	if _, cmd := c.Update(keyRune('/')); cmd == nil {
		t.Fatal("entering search should blink the cursor")
	}
	for _, r := range "first" {
		c.Update(keyRune(r))
	}
	got := c.listView.GetItems()
	if len(got) != 1 {
		t.Fatalf("matches = %d, want 1: %+v", len(got), got)
	}
	if got[0].MessageID != "m1" {
		t.Errorf("match = %q, want m1 (first question)", got[0].MessageID)
	}
}

func TestForkItemRenderSkipsZeroTimestamp(t *testing.T) {
	item := TimelineItem{Title: "Full session"}
	out := item.Render(false, 30)
	if strings.Contains(out, "00:00:00") {
		t.Errorf("zero CreatedAt must not render a timestamp, got %q", out)
	}
	stamped := TimelineItem{Title: "hello", CreatedAt: time.Unix(300, 0)}
	out = stamped.Render(false, 30)
	foot := strings.TrimSpace(strings.Split(out, "\n")[1])
	if _, err := time.Parse("15:04:05", foot); err != nil {
		t.Errorf("stamped rows must keep the time footer, got %q", foot)
	}
}

func TestForkDialogSelectMessage(t *testing.T) {
	c := newTestForkDialog(t)

	c.SelectMessage("m1")
	selected, idx := c.listView.GetSelectedItem()
	if idx == -1 || selected.MessageID != "m1" {
		t.Errorf("selected = %+v (idx %d), want the m1 row", selected, idx)
	}

	// Unknown ids leave the selection alone.
	c.SelectMessage("missing")
	if _, idx := c.listView.GetSelectedItem(); idx != 2 {
		t.Errorf("unknown id moved the selection to %d, want it unchanged", idx)
	}
}
