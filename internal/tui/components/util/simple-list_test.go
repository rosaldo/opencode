package utilComponents

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/opencode-ai/opencode/internal/tui/util"
)

type testItem struct{ name string }

func (i testItem) Render(_ bool, _ int) string { return i.name }

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
