package dialog

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/opencode-ai/opencode/internal/message"
	utilComponents "github.com/opencode-ai/opencode/internal/tui/components/util"
)

func newTestMessageDialog(t *testing.T, msg *message.Message) *messageActionsDialogCmp {
	t.Helper()
	c, ok := NewMessageActionsDialogCmp().(*messageActionsDialogCmp)
	if !ok {
		t.Fatal("NewMessageActionsDialogCmp must return *messageActionsDialogCmp")
	}
	c.SetMessage(msg)
	return c
}

func testUserMsg() *message.Message {
	return &message.Message{
		ID:   "m1",
		Role: message.User,
		Parts: []message.ContentPart{
			message.TextContent{Text: "hello"},
		},
	}
}

func TestMessageActionsUserSeesFourRows(t *testing.T) {
	c := newTestMessageDialog(t, testUserMsg())

	items := c.listView.GetItems()
	if len(items) != 4 {
		t.Fatalf("items = %d, want 4 (Jump to, Revert, Copy, Fork)", len(items))
	}
	want := []MessageAction{MessageActionJump, MessageActionRevert, MessageActionCopy, MessageActionFork}
	for i, w := range want {
		if items[i].Action != w {
			t.Errorf("item[%d].Action = %q, want %q", i, items[i].Action, w)
		}
	}
	if got := items[2].Description; got != "message text to clipboard" {
		t.Errorf("Copy description = %q, want the TS text", got)
	}
}

func TestMessageActionsAssistantHasNoFork(t *testing.T) {
	msg := &message.Message{ID: "m2", Role: message.Assistant}
	c := newTestMessageDialog(t, msg)

	items := c.listView.GetItems()
	if len(items) != 3 {
		t.Fatalf("items = %d, want 3 (no Fork for assistant messages)", len(items))
	}
}

func TestMessageActionsEnterRunsJumpByDefault(t *testing.T) {
	c := newTestMessageDialog(t, testUserMsg())

	_, cmd := c.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if cmd == nil {
		t.Fatal("enter must run the selected action")
	}
	got, ok := cmd().(MessageActionMsg)
	if !ok {
		t.Fatalf("msg = %T, want MessageActionMsg", cmd())
	}
	if got.Action != MessageActionJump || got.MessageID != "m1" {
		t.Errorf("msg = %+v, want jump for m1 (Jump to is preselected)", got)
	}
}

func TestMessageActionsDownThenEnterRunsAction(t *testing.T) {
	c := newTestMessageDialog(t, testUserMsg())

	c.Update(tea.KeyMsg{Type: tea.KeyDown})
	c.Update(tea.KeyMsg{Type: tea.KeyDown})
	c.Update(tea.KeyMsg{Type: tea.KeyDown})
	_, cmd := c.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if cmd == nil {
		t.Fatal("enter must run the selected action")
	}
	got, ok := cmd().(MessageActionMsg)
	if !ok {
		t.Fatalf("msg = %T, want MessageActionMsg", cmd())
	}
	if got.Action != MessageActionFork {
		t.Errorf("Action = %q, want fork", got.Action)
	}
}

func TestMessageActionsEscapeCloses(t *testing.T) {
	c := newTestMessageDialog(t, testUserMsg())

	_, cmd := c.Update(tea.KeyMsg{Type: tea.KeyEsc})
	if cmd == nil {
		t.Fatal("escape must close the dialog")
	}
	if _, ok := cmd().(CloseMessageActionsMsg); !ok {
		t.Fatalf("msg = %T, want CloseMessageActionsMsg", cmd())
	}
}

func TestMessageActionsClickActivates(t *testing.T) {
	c := newTestMessageDialog(t, testUserMsg())

	// The click already moved the selection to Copy (Spec 09n/09u).
	c.listView.SelectIndex(2)
	_, cmd := c.Update(utilComponents.ListActivatedMsg{Index: 2})
	if cmd == nil {
		t.Fatal("a click must activate the row like Enter")
	}
	got, ok := cmd().(MessageActionMsg)
	if !ok {
		t.Fatalf("msg = %T, want MessageActionMsg", cmd())
	}
	if got.Action != MessageActionCopy {
		t.Errorf("Action = %q, want copy", got.Action)
	}
}

func TestMessageActionsSetMessageResetsSelection(t *testing.T) {
	c := newTestMessageDialog(t, testUserMsg())
	c.Update(tea.KeyMsg{Type: tea.KeyDown})

	c.SetMessage(testUserMsg())
	_, cmd := c.Update(tea.KeyMsg{Type: tea.KeyEnter})
	got, ok := cmd().(MessageActionMsg)
	if !ok {
		t.Fatalf("msg = %T, want MessageActionMsg", cmd())
	}
	if got.Action != MessageActionJump {
		t.Errorf("Action = %q, want jump (reopening preselects Jump to)", got.Action)
	}
}

func TestMessageActionsWithoutMessageIgnoresEnter(t *testing.T) {
	c := newTestMessageDialog(t, nil)

	if items := c.listView.GetItems(); len(items) != 3 || items[2].Enabled {
		t.Fatalf("no message: Copy must be disabled, items = %+v", items)
	}
	if _, cmd := c.Update(tea.KeyMsg{Type: tea.KeyEnter}); cmd != nil {
		t.Errorf("enter without a message must do nothing, got %v", cmd())
	}
	_, cmd := c.Update(tea.KeyMsg{Type: tea.KeyEsc})
	if _, ok := cmd().(CloseMessageActionsMsg); !ok {
		t.Errorf("escape must still close, got %T", cmd())
	}
}
