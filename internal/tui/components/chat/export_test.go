package chat

import (
	"strings"
	"testing"

	"github.com/opencode-ai/opencode/internal/message"
)

func TestSessionToMarkdownRendersConversation(t *testing.T) {
	msgs := []message.Message{
		{Role: message.User, Parts: []message.ContentPart{
			message.TextContent{Text: "hello there"},
		}},
		{Role: message.Assistant, Parts: []message.ContentPart{
			message.ReasoningContent{Thinking: "thinking hard"},
			message.TextContent{Text: "hi!"},
			message.ToolCall{Name: "read", Input: `{"path":"a.go"}`},
			message.ToolResult{Name: "read", Content: "package a"},
			message.Finish{Reason: message.FinishReasonEndTurn},
		}},
	}

	out := SessionToMarkdown("My session", msgs)

	if !strings.HasPrefix(out, "# My session\n") {
		t.Errorf("header = %q, want the session title", strings.SplitN(out, "\n", 2)[0])
	}
	if !strings.Contains(out, "2 messages") {
		t.Errorf("summary line missing in %q", out)
	}
	for _, want := range []string{
		"## User",
		"## Assistant",
		"hello there",
		"hi!",
		"**Thinking:**",
		"thinking hard",
		"**Tool:** `read`",
		"```json\n{\"path\":\"a.go\"}\n```",
		"**Tool result:** `read`",
		"```\npackage a\n```",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("missing %q in:\n%s", want, out)
		}
	}
	if strings.Contains(out, "end_turn") {
		t.Errorf("finish part must be dropped, got:\n%s", out)
	}
	if !strings.HasSuffix(out, "\n") {
		t.Errorf("output must end with a newline, got %q", out)
	}
	if strings.HasSuffix(out, "\n\n") {
		t.Errorf("output must not end with a blank line, got %q", out)
	}
}

func TestSessionToMarkdownEmptyTitleAndErrors(t *testing.T) {
	msgs := []message.Message{
		{Role: message.Assistant, Parts: []message.ContentPart{
			message.ToolResult{Name: "bash", Content: "exit 1", IsError: true},
		}},
	}
	out := SessionToMarkdown("   ", msgs)
	if !strings.HasPrefix(out, "# Session\n") {
		t.Errorf("header = %q, want the default title", strings.SplitN(out, "\n", 2)[0])
	}
	if !strings.Contains(out, "**Tool error:** `bash`") {
		t.Errorf("error result not marked in:\n%s", out)
	}

	blank := SessionToMarkdown("Only empty", []message.Message{
		{Role: message.User, Parts: []message.ContentPart{message.TextContent{Text: "  "}}},
	})
	if !strings.Contains(blank, "1 message ·") {
		t.Errorf("summary line missing in %q", blank)
	}
	if strings.Contains(blank, "## User\n\n\n") {
		t.Errorf("empty text must not leave blank sections:\n%s", blank)
	}
}
