package dialog

import (
	"reflect"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/opencode-ai/opencode/internal/message"
	utilComponents "github.com/opencode-ai/opencode/internal/tui/components/util"
)

func testTimelineMsgs() []message.Message {
	return []message.Message{
		{ID: "m1", Role: message.User, CreatedAt: 100, Parts: []message.ContentPart{
			message.TextContent{Text: "first question"},
		}},
		{ID: "m2", Role: message.Assistant, CreatedAt: 200},
		{ID: "m3", Role: message.User, CreatedAt: 300, Parts: []message.ContentPart{
			message.TextContent{Text: "second\nquestion"},
		}},
		{ID: "m4", Role: message.User, CreatedAt: 400, Parts: []message.ContentPart{
			message.TextContent{Text: "   "},
		}},
	}
}

func TestTimelineItemsUserMessagesNewestFirst(t *testing.T) {
	items := TimelineItems(testTimelineMsgs())
	if len(items) != 2 {
		t.Fatalf("items = %d, want 2 user messages", len(items))
	}
	// Newest first, like the TS timeline (Spec 09o).
	if items[0].MessageID != "m3" || items[1].MessageID != "m1" {
		t.Errorf("order = %s,%s, want m3,m1", items[0].MessageID, items[1].MessageID)
	}
	// The text is flattened into a single line.
	if items[0].Title != "second question" {
		t.Errorf("title = %q, want the flattened text", items[0].Title)
	}
	if want := time.Unix(300, 0); !items[0].CreatedAt.Equal(want) {
		t.Errorf("CreatedAt = %v, want %v", items[0].CreatedAt, want)
	}
}

func newTestTimelineDialog(t *testing.T) *timelineDialogCmp {
	t.Helper()
	c, ok := NewTimelineDialogCmp().(*timelineDialogCmp)
	if !ok {
		t.Fatal("NewTimelineDialogCmp must return *timelineDialogCmp")
	}
	c.SetItems(TimelineItems(testTimelineMsgs()))
	return c
}

func TestTimelineDialogEnterJumps(t *testing.T) {
	c := newTestTimelineDialog(t)

	_, cmd := c.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if cmd == nil {
		t.Fatal("enter must jump to the selected message")
	}
	msg, ok := cmd().(TimelineJumpMsg)
	if !ok {
		t.Fatalf("msg = %T, want TimelineJumpMsg", cmd())
	}
	// The list opens with the newest message selected.
	if msg.MessageID != "m3" {
		t.Errorf("MessageID = %q, want m3 (newest first)", msg.MessageID)
	}
}

func TestTimelineDialogEscapeCloses(t *testing.T) {
	c := newTestTimelineDialog(t)

	_, cmd := c.Update(tea.KeyMsg{Type: tea.KeyEsc})
	if cmd == nil {
		t.Fatal("escape must close the dialog")
	}
	if _, ok := cmd().(CloseTimelineDialogMsg); !ok {
		t.Fatalf("msg = %T, want CloseTimelineDialogMsg", cmd())
	}
}

func TestTimelineDialogListActivatedJumps(t *testing.T) {
	c := newTestTimelineDialog(t)

	// The click already moved the selection to the older message (Spec 09n);
	// Down is the keyboard equivalent of that move.
	c.Update(tea.KeyMsg{Type: tea.KeyDown})
	_, cmd := c.Update(utilComponents.ListActivatedMsg{Index: 1})
	if cmd == nil {
		t.Fatal("ListActivatedMsg must jump like Enter")
	}
	msg, ok := cmd().(TimelineJumpMsg)
	if !ok {
		t.Fatalf("msg = %T, want TimelineJumpMsg", cmd())
	}
	if msg.MessageID != "m1" {
		t.Errorf("MessageID = %q, want m1 after moving down", msg.MessageID)
	}
}

func TestTimelineDialogSearchNarrows(t *testing.T) {
	c := newTestTimelineDialog(t)

	if _, cmd := c.Update(keyRune('/')); cmd == nil {
		t.Fatal("entering search should blink the cursor")
	}
	c.Update(keyRune('f'))
	c.Update(keyRune('i'))
	got := c.listView.GetItems()
	if len(got) != 1 {
		t.Fatalf("matches = %d, want 1: %+v", len(got), got)
	}
	if got[0].MessageID != "m1" {
		t.Errorf("match = %q, want m1 (first question)", got[0].MessageID)
	}
}

func TestTimelineDialogPreviewOnMove(t *testing.T) {
	c := newTestTimelineDialog(t)

	_, cmd := c.Update(tea.KeyMsg{Type: tea.KeyDown})
	if cmd == nil {
		t.Fatal("moving the selection must emit the preview cmd")
	}
	found := false
	for _, m := range flattenBatch(cmd()) {
		if preview, ok := m.(TimelinePreviewMsg); ok {
			found = true
			if preview.MessageID != "m1" {
				t.Errorf("preview = %q, want m1", preview.MessageID)
			}
		}
	}
	if !found {
		t.Fatal("moving down must emit TimelinePreviewMsg")
	}
}

// flattenBatch executes a cmd and unwraps one level of the unexported
// tea.Batch message (a named slice of tea.Cmd) via reflection.
func flattenBatch(msg tea.Msg) []tea.Msg {
	v := reflect.ValueOf(msg)
	if v.Kind() != reflect.Slice || v.Type().Elem().String() != "tea.Cmd" {
		return []tea.Msg{msg}
	}
	out := make([]tea.Msg, 0, v.Len())
	for i := 0; i < v.Len(); i++ {
		if cmd, ok := v.Index(i).Interface().(tea.Cmd); ok && cmd != nil {
			out = append(out, cmd())
		}
	}
	return out
}

func TestTimelineItemRenderTruncates(t *testing.T) {
	item := TimelineItem{
		Title:     strings.Repeat("a", 100),
		CreatedAt: time.Unix(300, 0),
	}
	out := item.Render(false, 20)
	if !strings.Contains(out, "...") {
		t.Errorf("long titles must be truncated, got %q", out)
	}
}
