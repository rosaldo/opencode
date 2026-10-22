package utilComponents

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/opencode-ai/opencode/internal/tui/util"
)

type testItem struct{ name string }

func (i testItem) Render(_ bool, _ int) string { return i.name }

type groupedItem struct{ name, group string }

func (i groupedItem) Render(_ bool, _ int) string { return i.name }

func (i groupedItem) GroupTitle() string { return i.group }

func TestListZoneID(t *testing.T) {
	if got := ListZoneID("pfx:", 7); got != "pfx:7" {
		t.Errorf("ListZoneID = %q, want pfx:7", got)
	}
}

func TestListClickedRowUnresolvedZone(t *testing.T) {
	var zones util.ZoneSet
	if _, ok := ListClickedRow(&zones, "pfx:", tea.MouseMsg{}); ok {
		t.Error("an empty zone set must not resolve a click")
	}
}

func TestSimpleListClickRow(t *testing.T) {
	list := NewSimpleList([]testItem{{"a"}, {"b"}, {"c"}}, 10, "none", false)
	c, ok := list.(*simpleListCmp[testItem])
	if !ok {
		t.Fatal("NewSimpleList must return *simpleListCmp")
	}

	cmd := c.clickRow(2)
	if cmd == nil {
		t.Fatal("clicking a valid row must report the activation")
	}
	msg, ok := cmd().(ListActivatedMsg)
	if !ok {
		t.Fatal("click must emit ListActivatedMsg")
	}
	if msg.Index != 2 {
		t.Errorf("Index = %d, want 2", msg.Index)
	}
	if c.selectedIdx != 2 {
		t.Errorf("selectedIdx = %d, want 2", c.selectedIdx)
	}

	if cmd := c.clickRow(99); cmd != nil {
		t.Error("out-of-range clicks must be ignored")
	}
	if cmd := c.clickRow(-1); cmd != nil {
		t.Error("negative clicks must be ignored")
	}
	if c.selectedIdx != 2 {
		t.Errorf("selectedIdx = %d, want selection unchanged", c.selectedIdx)
	}
}

func groupedFixture() (*simpleListCmp[groupedItem], []groupedItem) {
	items := []groupedItem{
		{"a", "G1"},
		{"b", "G1"},
		{"c", "G2"},
		{"d", "G3"},
		{"e", "G3"},
	}
	list := NewSimpleList(items, 10, "none", false)
	return list.(*simpleListCmp[groupedItem]), items
}

func TestSimpleListGroupHeaders(t *testing.T) {
	c, _ := groupedFixture()

	// Headers off by default: one line per row.
	if lines := strings.Split(c.View(), "\n"); len(lines) != 5 {
		t.Fatalf("ungrouped lines = %d, want 5:\n%q", len(lines), lines)
	}

	c.SetGroupHeaders(true)
	lines := strings.Split(c.View(), "\n")
	want := []string{"G1", "a", "b", "G2", "c", "G3", "d", "e"}
	if len(lines) != len(want) {
		t.Fatalf("lines = %d, want %d:\n%q", len(lines), len(want), lines)
	}
	for i, w := range want {
		if !strings.Contains(lines[i], w) {
			t.Errorf("line[%d] = %q, want it to contain %q", i, lines[i], w)
		}
	}
}

func TestSimpleListGroupHeaderLineBudget(t *testing.T) {
	items := []groupedItem{
		{"a", "G1"},
		{"b", "G2"},
		{"c", "G1"},
		{"d", "G2"},
		{"e", "G1"},
		{"f", "G2"},
	}
	list := NewSimpleList(items, 4, "none", false)
	c := list.(*simpleListCmp[groupedItem])
	c.SetGroupHeaders(true)

	// Every row starts a group, so each item costs two lines: the budget of
	// four lines must cap the render at four lines (Spec 09s).
	c.selectedIdx = 0
	if lines := strings.Split(c.View(), "\n"); len(lines) > 4 {
		t.Fatalf("lines = %d, want <= 4:\n%q", len(lines), lines)
	}
	if view := c.View(); !strings.Contains(view, "a") {
		t.Errorf("window must keep the selection visible:\n%s", view)
	}

	// Selection in the middle: still within the budget and still visible.
	c.selectedIdx = 3
	if lines := strings.Split(c.View(), "\n"); len(lines) > 4 {
		t.Fatalf("lines = %d, want <= 4:\n%q", len(lines), lines)
	}
	if view := c.View(); !strings.Contains(view, "d") {
		t.Errorf("window must keep the selection visible:\n%s", view)
	}
}

func TestSimpleListGroupHeaderNotZoned(t *testing.T) {
	c, _ := groupedFixture()
	c.SetGroupHeaders(true)
	c.View()

	// The view renders 8 lines (3 headers + 5 rows), but only the rows get
	// zones: a click on a header resolves to nothing (Spec 09s).
	if got := c.zones.Len(); got != 5 {
		t.Errorf("zoned ids = %d, want 5 row zones", got)
	}
}

func TestSimpleListSelectIndex(t *testing.T) {
	list := NewSimpleList([]testItem{{"a"}, {"b"}, {"c"}}, 10, "none", false)
	c, ok := list.(*simpleListCmp[testItem])
	if !ok {
		t.Fatal("NewSimpleList must return *simpleListCmp")
	}

	c.SelectIndex(1)
	if _, idx := c.GetSelectedItem(); idx != 1 {
		t.Errorf("selectedIdx = %d, want 1", idx)
	}
	c.SelectIndex(99)
	if _, idx := c.GetSelectedItem(); idx != 1 {
		t.Errorf("out-of-range select must be ignored, got %d", idx)
	}
	c.SelectIndex(-1)
	if _, idx := c.GetSelectedItem(); idx != 1 {
		t.Errorf("negative select must be ignored, got %d", idx)
	}
}
