package chat

import (
	"testing"

	"github.com/opencode-ai/opencode/internal/message"
)

func TestToggleThinking(t *testing.T) {
	cmp, ok := NewMessagesCmp(nil).(*messagesCmp)
	if !ok {
		t.Fatal("NewMessagesCmp must return a *messagesCmp")
	}
	if !cmp.showThinking {
		t.Fatal("thinking blocks must be visible by default")
	}

	next, cmd := cmp.Update(ToggleThinkingMsg{})
	got := next.(*messagesCmp)
	if got.showThinking {
		t.Error("first toggle must hide the reasoning blocks")
	}
	if cmd == nil {
		t.Error("toggle must report the new state")
	}

	next, _ = got.Update(ToggleThinkingMsg{})
	if !next.(*messagesCmp).showThinking {
		t.Error("second toggle must show the reasoning blocks again")
	}
}

func TestMessagePlainText(t *testing.T) {
	user := message.Message{Role: message.User, Parts: []message.ContentPart{
		message.TextContent{Text: "  hello there  "},
	}}
	if got := MessagePlainText(user); got != "hello there" {
		t.Errorf("user = %q, want the trimmed text", got)
	}

	assistant := message.Message{Role: message.Assistant, Parts: []message.ContentPart{
		message.ReasoningContent{Thinking: "secret"},
		message.TextContent{Text: "first"},
		message.ToolCall{Name: "read", Input: "{}"},
		message.TextContent{Text: "second"},
	}}
	if got := MessagePlainText(assistant); got != "first\n\nsecond" {
		t.Errorf("assistant = %q, want the text parts joined", got)
	}

	toolOnly := message.Message{Role: message.Assistant, Parts: []message.ContentPart{
		message.ToolCall{Name: "bash"},
	}}
	if got := MessagePlainText(toolOnly); got != "`bash`" {
		t.Errorf("tool-only = %q, want the call name", got)
	}

	if got := MessagePlainText(message.Message{}); got != "" {
		t.Errorf("empty message = %q, want empty", got)
	}
}

func TestCopyTarget(t *testing.T) {
	m := &messagesCmp{messages: []message.Message{
		{ID: "a", Role: message.User},
		{ID: "b", Role: message.Assistant},
	}}

	if got := m.copyTarget(); got == nil || got.ID != "b" {
		t.Fatalf("no focus must fall back to the last message, got %+v", got)
	}

	m.currentMsgID = "a"
	if got := m.copyTarget(); got == nil || got.ID != "a" {
		t.Fatalf("focused message must win, got %+v", got)
	}

	m.currentMsgID = "missing"
	if got := m.copyTarget(); got == nil || got.ID != "b" {
		t.Fatalf("an unknown focus must fall back to the last message, got %+v", got)
	}

	empty := &messagesCmp{}
	if got := empty.copyTarget(); got != nil {
		t.Fatalf("an empty list must have no target, got %+v", got)
	}
}
