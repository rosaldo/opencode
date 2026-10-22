package tui

import (
	"testing"

	"github.com/opencode-ai/opencode/internal/message"
)

func TestLastUserMessageIndex(t *testing.T) {
	if got := lastUserMessageIndex(nil); got != -1 {
		t.Errorf("empty list = %d, want -1", got)
	}

	assistantOnly := []message.Message{
		{Role: message.Assistant},
		{Role: message.System},
	}
	if got := lastUserMessageIndex(assistantOnly); got != -1 {
		t.Errorf("no user message = %d, want -1", got)
	}

	msgs := []message.Message{
		{Role: message.User},
		{Role: message.Assistant},
		{Role: message.User},
		{Role: message.Assistant},
		{Role: message.Assistant},
	}
	if got := lastUserMessageIndex(msgs); got != 2 {
		t.Errorf("index = %d, want 2 (the newest user message)", got)
	}
}

func TestStripFinishParts(t *testing.T) {
	parts := []message.ContentPart{
		message.TextContent{Text: "hello"},
		message.Finish{Reason: "stop"},
		message.ToolCall{Name: "read"},
	}
	stripped := stripFinishParts(parts)
	if len(stripped) != 2 {
		t.Fatalf("stripped = %d parts, want 2", len(stripped))
	}
	if _, isText := stripped[0].(message.TextContent); !isText {
		t.Errorf("first part = %T, want TextContent", stripped[0])
	}
	if _, isCall := stripped[1].(message.ToolCall); !isCall {
		t.Errorf("second part = %T, want ToolCall", stripped[1])
	}
	if len(parts) != 3 {
		t.Error("stripFinishParts must not mutate the input slice")
	}

	if got := stripFinishParts(nil); got == nil || len(got) != 0 {
		t.Errorf("nil parts = %v, want an empty slice", got)
	}
}
