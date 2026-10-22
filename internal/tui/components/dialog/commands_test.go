package dialog

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

func testCommands() []Command {
	return []Command{
		{ID: "init", Title: "Initialize Project", Description: "Create the memory file"},
		{ID: "compact", Title: "Compact Session", Description: "Summarize the current session"},
		{ID: "plugins", Title: "Plugins", Description: "List the loaded plugins"},
	}
}

func keyRune(r rune) tea.KeyMsg {
	return tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}}
}

func newTestCommandDialog(t *testing.T) *commandDialogCmp {
	t.Helper()
	c, ok := NewCommandDialogCmp().(*commandDialogCmp)
	if !ok {
		t.Fatal("NewCommandDialogCmp must return *commandDialogCmp")
	}
	return c
}

func TestCommandDialogFuzzySearch(t *testing.T) {
	c := newTestCommandDialog(t)
	c.SetCommands(testCommands())

	if _, cmd := c.Update(keyRune('/')); cmd == nil {
		t.Fatal("entering search should blink the cursor")
	}
	if c.mode != commandModeSearch {
		t.Fatalf("mode = %v, want search", c.mode)
	}

	// "cs" is a subsequence of "Compact Session …" only: "Initialize Project"
	// has no s after its c, and "Plugins" has no c at all.
	for _, r := range "cs" {
		c.Update(keyRune(r))
	}
	got := c.listView.GetItems()
	if len(got) != 1 {
		t.Fatalf("matches = %d, want 1: %+v", len(got), got)
	}
	if got[0].ID != "compact" {
		t.Errorf("match = %q, want compact", got[0].ID)
	}

	// A single "c" also lands in "Initialize Project Create …" — RankFindFold
	// sorts by Levenshtein distance, so the shorter target comes first.
	c.input.SetValue("")
	c.applyFilter()
	c.input.SetValue("c")
	c.applyFilter()
	got = c.listView.GetItems()
	if len(got) != 2 {
		t.Fatalf("matches for c = %d, want 2: %+v", len(got), got)
	}
	if got[0].ID != "init" || got[1].ID != "compact" {
		t.Errorf("order = %q,%q, want init,compact", got[0].ID, got[1].ID)
	}
}

func TestCommandDialogSearchKeepsNarrowedListItemOnEnter(t *testing.T) {
	c := newTestCommandDialog(t)
	c.SetCommands(testCommands())

	c.Update(keyRune('/'))
	for _, r := range "plug" {
		c.Update(keyRune(r))
	}
	if c.mode != commandModeSearch {
		t.Fatalf("mode = %v, want search", c.mode)
	}
	if _, cmd := c.Update(tea.KeyMsg{Type: tea.KeyEnter}); cmd != nil {
		t.Fatalf("leaving search must not select: %v", cmd)
	}
	if c.mode != commandModeList {
		t.Fatalf("mode = %v, want list", c.mode)
	}
	got := c.listView.GetItems()
	if len(got) != 1 || got[0].ID != "plugins" {
		t.Fatalf("items after leaving search = %+v, want only plugins", got)
	}

	// The second Enter runs the surviving command.
	_, cmd := c.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if cmd == nil {
		t.Fatal("selecting a command must emit a command")
	}
	msg := cmd()
	selected, ok := msg.(CommandSelectedMsg)
	if !ok {
		t.Fatalf("msg = %T, want CommandSelectedMsg", msg)
	}
	if selected.Command.ID != "plugins" {
		t.Errorf("selected = %q, want plugins", selected.Command.ID)
	}
}

func TestCommandDialogSetCommandsResetsSearch(t *testing.T) {
	c := newTestCommandDialog(t)
	c.SetCommands(testCommands())

	c.Update(keyRune('/'))
	for _, r := range "zzz" {
		c.Update(keyRune(r))
	}
	if len(c.listView.GetItems()) != 0 {
		t.Fatalf("query zzz should match nothing, got %+v", c.listView.GetItems())
	}

	c.SetCommands(testCommands())
	if c.mode != commandModeList {
		t.Errorf("mode = %v, want list", c.mode)
	}
	if c.input.Value() != "" {
		t.Errorf("query = %q, want empty", c.input.Value())
	}
	if got := len(c.listView.GetItems()); got != 3 {
		t.Errorf("items = %d, want 3", got)
	}
}

func TestCommandDialogEmptyQueryRestoresOrder(t *testing.T) {
	c := newTestCommandDialog(t)
	c.SetCommands(testCommands())

	c.Update(keyRune('/'))
	c.Update(keyRune('p'))
	if _, cmd := c.Update(tea.KeyMsg{Type: tea.KeyEnter}); cmd != nil {
		t.Fatalf("leaving search must not select: %v", cmd)
	}
	c.input.SetValue("")
	c.applyFilter()

	got := c.listView.GetItems()
	if len(got) != 3 {
		t.Fatalf("items = %d, want 3", len(got))
	}
	for i, want := range []string{"init", "compact", "plugins"} {
		if got[i].ID != want {
			t.Errorf("item[%d] = %q, want %q", i, got[i].ID, want)
		}
	}
}
