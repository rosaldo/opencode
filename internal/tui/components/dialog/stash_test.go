package dialog

import (
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/opencode-ai/opencode/internal/promptstash"
	utilComponents "github.com/opencode-ai/opencode/internal/tui/components/util"
)

func testStashEntries() []promptstash.Entry {
	base := time.Now().Add(-time.Hour).UnixMilli()
	return []promptstash.Entry{
		{Text: "first prompt", Timestamp: base},
		{Text: "second prompt", Timestamp: base + 60_000},
		{Text: "third prompt\nsecond line", Timestamp: base + 120_000},
	}
}

func newTestStashDialog(t *testing.T) *stashDialogCmp {
	t.Helper()
	c, ok := NewStashDialogCmp().(*stashDialogCmp)
	if !ok {
		t.Fatal("NewStashDialogCmp must return *stashDialogCmp")
	}
	c.SetItems(StashItems(testStashEntries()))
	return c
}

func TestStashItemsNewestFirstWithStoreIndexes(t *testing.T) {
	items := StashItems(testStashEntries())
	if len(items) != 3 {
		t.Fatalf("items = %d, want 3", len(items))
	}
	if items[0].Title != "third prompt" || items[2].Title != "first prompt" {
		t.Errorf("order = %q…%q, want newest first", items[0].Title, items[2].Title)
	}
	// Rows keep their store index so select/delete report the right entry.
	if items[0].Index != 2 || items[2].Index != 0 {
		t.Errorf("indexes = %d,%d, want 2,0", items[0].Index, items[2].Index)
	}
	if items[0].Lines != 2 || items[1].Lines != 1 {
		t.Errorf("line counts = %d,%d, want 2,1", items[0].Lines, items[1].Lines)
	}
	if items[0].CreatedAt.IsZero() {
		t.Error("CreatedAt must come from the entry timestamp")
	}
}

func TestStashItemsTitlePreviewLimit(t *testing.T) {
	long := strings.Repeat("x", 80) + "\nsecond"
	items := StashItems([]promptstash.Entry{{Text: long, Timestamp: 1}})
	if r := []rune(items[0].Title); len(r) != stashPreviewLimit+3 {
		t.Fatalf("title runes = %d, want %d (50 + ...)", len(r), stashPreviewLimit+3)
	}
	if !strings.HasSuffix(items[0].Title, "...") {
		t.Errorf("title = %q, want the ellipsis", items[0].Title)
	}
	if items[0].Lines != 2 {
		t.Errorf("Lines = %d, want 2", items[0].Lines)
	}
}

func TestStashRelativeTime(t *testing.T) {
	now := time.Date(2026, 10, 22, 12, 0, 0, 0, time.UTC)
	cases := []struct {
		age  time.Duration
		want string
	}{
		{0, "just now"},
		{30 * time.Second, "just now"},
		{5 * time.Minute, "5m ago"},
		{3 * time.Hour, "3h ago"},
		{2 * 24 * time.Hour, "2d ago"},
	}
	for _, tc := range cases {
		if got := stashRelativeTime(now.Add(-tc.age), now); got != tc.want {
			t.Errorf("stashRelativeTime(%s) = %q, want %q", tc.age, got, tc.want)
		}
	}
	absolute := stashRelativeTime(now.Add(-8*24*time.Hour), now)
	if absolute != "2026-10-14 12:00" {
		t.Errorf("older than a week = %q, want the absolute timestamp", absolute)
	}
}

func TestStashDialogEnterRestoresNewest(t *testing.T) {
	c := newTestStashDialog(t)

	_, cmd := c.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if cmd == nil {
		t.Fatal("enter must restore the selected entry")
	}
	msg, ok := cmd().(StashSelectMsg)
	if !ok {
		t.Fatalf("msg = %T, want StashSelectMsg", cmd())
	}
	// The list opens with the newest entry (store index 2) selected.
	if msg.Index != 2 {
		t.Errorf("Index = %d, want 2", msg.Index)
	}
}

func TestStashDialogEnterAfterMoveSelectsThatEntry(t *testing.T) {
	c := newTestStashDialog(t)

	c.Update(tea.KeyMsg{Type: tea.KeyDown})
	_, cmd := c.Update(tea.KeyMsg{Type: tea.KeyEnter})
	msg, ok := cmd().(StashSelectMsg)
	if !ok {
		t.Fatalf("msg = %T, want StashSelectMsg", cmd())
	}
	if msg.Index != 1 {
		t.Errorf("Index = %d, want 1", msg.Index)
	}
}

func TestStashDialogEscapeCloses(t *testing.T) {
	c := newTestStashDialog(t)

	_, cmd := c.Update(tea.KeyMsg{Type: tea.KeyEsc})
	if cmd == nil {
		t.Fatal("escape must close the dialog")
	}
	if _, ok := cmd().(CloseStashDialogMsg); !ok {
		t.Fatalf("msg = %T, want CloseStashDialogMsg", cmd())
	}
}

func TestStashDialogDeleteNeedsTwoPresses(t *testing.T) {
	c := newTestStashDialog(t)

	// First ctrl+d only arms the selected row.
	_, cmd := c.Update(tea.KeyMsg{Type: tea.KeyCtrlD})
	if cmd != nil {
		t.Fatalf("first ctrl+d must not delete, got %v", cmd())
	}
	if !c.armed || c.armedIdx != 2 {
		t.Fatalf("armed = %v (idx %d), want armed for store index 2", c.armed, c.armedIdx)
	}

	// Second ctrl+d confirms: the row is dropped and renumbered.
	_, cmd = c.Update(tea.KeyMsg{Type: tea.KeyCtrlD})
	if cmd == nil {
		t.Fatal("second ctrl+d must confirm the deletion")
	}
	msg, ok := cmd().(StashDeleteMsg)
	if !ok {
		t.Fatalf("msg = %T, want StashDeleteMsg", cmd())
	}
	if msg.Index != 2 {
		t.Errorf("Index = %d, want the original store index 2", msg.Index)
	}
	if c.armed {
		t.Error("confirming must disarm")
	}
	if len(c.all) != 2 || c.all[0].Index != 1 || c.all[1].Index != 0 {
		t.Errorf("rows = %+v, want newest-first with renumbered indexes 1,0", c.all)
	}
}

func TestStashDialogMovingSelectionDisarms(t *testing.T) {
	c := newTestStashDialog(t)

	c.Update(tea.KeyMsg{Type: tea.KeyCtrlD})
	if !c.armed {
		t.Fatal("first ctrl+d must arm")
	}
	c.Update(tea.KeyMsg{Type: tea.KeyDown})
	if c.armed {
		t.Fatal("moving the selection must disarm")
	}

	// The next press arms the new row instead of deleting it.
	_, cmd := c.Update(tea.KeyMsg{Type: tea.KeyCtrlD})
	if cmd != nil {
		t.Fatalf("a lone press after moving must only arm, got %v", cmd())
	}
	if !c.armed || c.armedIdx != 1 {
		t.Fatalf("armed = %v (idx %d), want armed for store index 1", c.armed, c.armedIdx)
	}
}

func TestStashDialogEscapeDisarmsBeforeClosing(t *testing.T) {
	c := newTestStashDialog(t)

	c.Update(tea.KeyMsg{Type: tea.KeyCtrlD})
	_, cmd := c.Update(tea.KeyMsg{Type: tea.KeyEsc})
	if cmd != nil {
		t.Fatalf("first escape must only disarm, got %v", cmd())
	}
	_, cmd = c.Update(tea.KeyMsg{Type: tea.KeyEsc})
	if _, ok := cmd().(CloseStashDialogMsg); !ok {
		t.Fatalf("second escape msg = %T, want CloseStashDialogMsg", cmd())
	}
}

func TestStashDialogListActivatedRestoresLikeEnter(t *testing.T) {
	c := newTestStashDialog(t)

	// The click already moved the selection (Spec 09n); Down mirrors it.
	c.Update(tea.KeyMsg{Type: tea.KeyDown})
	_, cmd := c.Update(utilComponents.ListActivatedMsg{Index: 1})
	msg, ok := cmd().(StashSelectMsg)
	if !ok {
		t.Fatalf("msg = %T, want StashSelectMsg", cmd())
	}
	if msg.Index != 1 {
		t.Errorf("Index = %d, want 1", msg.Index)
	}
}

func TestStashDialogSearchNarrowsAndDisarms(t *testing.T) {
	c := newTestStashDialog(t)

	c.Update(tea.KeyMsg{Type: tea.KeyCtrlD})
	if _, cmd := c.Update(keyRune('/')); cmd == nil {
		t.Fatal("entering search should blink the cursor")
	}
	for _, r := range "third" {
		c.Update(keyRune(r))
	}
	if c.armed {
		t.Error("searching must disarm the pending deletion")
	}
	got := c.listView.GetItems()
	if len(got) != 1 || got[0].Index != 2 {
		t.Fatalf("matches = %+v, want only store index 2", got)
	}
}

func TestStashDialogDeleteWhileFilteredKeepsRowsConsistent(t *testing.T) {
	c := newTestStashDialog(t)

	c.Update(keyRune('/'))
	c.Update(keyRune('f')) // narrows to "first prompt" (store index 0)
	c.mode = commandModeList

	_, cmd := c.Update(tea.KeyMsg{Type: tea.KeyCtrlD})
	_, cmd = c.Update(tea.KeyMsg{Type: tea.KeyCtrlD})
	if cmd == nil {
		t.Fatal("second ctrl+d must delete the filtered row")
	}
	if msg, ok := cmd().(StashDeleteMsg); !ok || msg.Index != 0 {
		t.Fatalf("msg = %v, want StashDeleteMsg{0}", cmd())
	}
	// Removing store index 0 leaves "second" at 0 and "third" at 1, still
	// rendered newest first.
	if len(c.all) != 2 || c.all[0].Index != 1 || c.all[1].Index != 0 {
		t.Errorf("rows = %+v, want newest-first with renumbered indexes 1,0", c.all)
	}
}

func TestStashItemRenderFooter(t *testing.T) {
	single := StashItem{Title: "hello", CreatedAt: time.Now(), Lines: 1}
	out := single.Render(false, 40)
	if strings.Contains(out, "~1 lines") {
		t.Errorf("single-line rows must omit the line count, got %q", out)
	}

	multi := StashItem{Title: "hello", CreatedAt: time.Now().Add(-90 * time.Minute), Lines: 3}
	out = multi.Render(false, 40)
	if !strings.Contains(out, "~3 lines") {
		t.Errorf("multiline rows must show the count, got %q", out)
	}
	if !strings.Contains(out, "1h ago") {
		t.Errorf("footer must show the relative age, got %q", out)
	}
}

func TestStashDialogSetItemsResetsSearchAndArm(t *testing.T) {
	c := newTestStashDialog(t)

	c.Update(tea.KeyMsg{Type: tea.KeyCtrlD})
	c.Update(keyRune('/'))
	c.Update(keyRune('f'))

	c.SetItems(StashItems(testStashEntries()))
	if c.mode != commandModeList {
		t.Error("SetItems must leave search mode")
	}
	if c.armed {
		t.Error("SetItems must disarm")
	}
	if len(c.listView.GetItems()) != 3 {
		t.Errorf("items = %d, want a fresh full list", len(c.listView.GetItems()))
	}
}
