package chat

import (
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"

	"github.com/opencode-ai/opencode/internal/message"
	"github.com/opencode-ai/opencode/internal/tui/theme"
)

func assistantWith(parts ...message.ContentPart) message.Message {
	return message.Message{
		ID:    "m1",
		Role:  message.Assistant,
		Parts: parts,
	}
}

func TestRenderThinkingBlockCollapsed(t *testing.T) {
	if theme.CurrentTheme() == nil {
		t.Fatal("expected a registered theme")
	}
	out := ansi.Strip(renderThinkingBlock("first line\nsecond line", false, 80))
	if !strings.Contains(out, "Thinking") {
		t.Errorf("collapsed block must be labelled, got %q", out)
	}
	if !strings.Contains(out, "2 lines") {
		t.Errorf("collapsed block must show the line count, got %q", out)
	}
	if strings.Contains(out, "second line") {
		t.Errorf("collapsed block must hide the reasoning, got %q", out)
	}
}

func TestRenderThinkingBlockExpanded(t *testing.T) {
	out := ansi.Strip(renderThinkingBlock("first line\nsecond line", true, 80))
	if !strings.Contains(out, "Thinking") {
		t.Errorf("expanded block must keep its header, got %q", out)
	}
	if !strings.Contains(out, "second line") {
		t.Errorf("expanded block must show the reasoning, got %q", out)
	}
	if strings.Contains(out, "▸") {
		t.Errorf("expanded block must flip the indicator, got %q", out)
	}
}

func TestRenderAssistantMessageThinkingOnly(t *testing.T) {
	msg := assistantWith(message.ReasoningContent{Thinking: "pondering\nharder"})
	got := renderAssistantMessage(msg, 0, nil, nil, "", false, true, true, false, 80, 0)

	if len(got) != 1 {
		t.Fatalf("want 1 uiMessage, got %d: %+v", len(got), got)
	}
	if got[0].messageType != thinkingMessageType {
		t.Errorf("messageType = %v, want thinkingMessageType", got[0].messageType)
	}
	if got[0].zoneID != "think:m1" {
		t.Errorf("zoneID = %q, want think:m1", got[0].zoneID)
	}
	if got[0].height <= 0 {
		t.Errorf("height = %d, want > 0", got[0].height)
	}
	if !strings.Contains(ansi.Strip(got[0].content), "Thinking") {
		t.Errorf("content = %q, want the Thinking header", got[0].content)
	}
}

func TestRenderAssistantMessageThinkingAndAnswer(t *testing.T) {
	msg := assistantWith(
		message.ReasoningContent{Thinking: "the reasoning"},
		message.TextContent{Text: "the answer"},
	)
	got := renderAssistantMessage(msg, 0, nil, nil, "", false, true, true, false, 80, 0)

	if len(got) != 2 {
		t.Fatalf("want 2 uiMessages, got %d: %+v", len(got), got)
	}
	if got[0].messageType != thinkingMessageType {
		t.Errorf("first block = %v, want thinkingMessageType", got[0].messageType)
	}
	if got[1].messageType != assistantMessageType {
		t.Errorf("second block = %v, want assistantMessageType", got[1].messageType)
	}
	if got[1].position <= got[0].position {
		t.Errorf("answer position %d must come after thinking %d", got[1].position, got[0].position)
	}
}

func TestRenderAssistantMessageAnswerOnly(t *testing.T) {
	msg := assistantWith(message.TextContent{Text: "just the answer"})
	got := renderAssistantMessage(msg, 0, nil, nil, "", false, true, true, false, 80, 0)

	if len(got) != 1 {
		t.Fatalf("want 1 uiMessage, got %d: %+v", len(got), got)
	}
	if got[0].messageType != assistantMessageType {
		t.Errorf("messageType = %v, want assistantMessageType", got[0].messageType)
	}
}

func TestRenderAssistantMessageExpandsWhenAsked(t *testing.T) {
	msg := assistantWith(message.ReasoningContent{Thinking: "secret\nreasoning"})
	got := renderAssistantMessage(msg, 0, nil, nil, "", true, true, true, false, 80, 0)

	if len(got) != 1 {
		t.Fatalf("want 1 uiMessage, got %d", len(got))
	}
	if !strings.Contains(ansi.Strip(got[0].content), "secret") {
		t.Errorf("expanded block must show the reasoning, got %q", got[0].content)
	}
}

func TestRenderAssistantMessageThinkingHidden(t *testing.T) {
	msg := assistantWith(
		message.ReasoningContent{Thinking: "the reasoning"},
		message.TextContent{Text: "the answer"},
	)
	got := renderAssistantMessage(msg, 0, nil, nil, "", false, false, true, false, 80, 0)

	if len(got) != 1 {
		t.Fatalf("want 1 uiMessage with thinking hidden, got %d: %+v", len(got), got)
	}
	if got[0].messageType != assistantMessageType {
		t.Errorf("messageType = %v, want assistantMessageType", got[0].messageType)
	}
	out := ansi.Strip(got[0].content)
	if strings.Contains(out, "Thinking") || strings.Contains(out, "the reasoning") {
		t.Errorf("hidden thinking must not be rendered, got %q", out)
	}
	if !strings.Contains(out, "the answer") {
		t.Errorf("the answer must stay visible, got %q", out)
	}
}

func TestRenderAssistantMessageThinkingOnlyHidden(t *testing.T) {
	msg := assistantWith(message.ReasoningContent{Thinking: "pondering"})
	got := renderAssistantMessage(msg, 0, nil, nil, "", false, false, true, false, 80, 0)
	if len(got) != 0 {
		t.Fatalf("a hidden thinking-only message must render nothing, got %d: %+v", len(got), got)
	}
}

func TestRenderAssistantMessageToolDetailsToggle(t *testing.T) {
	msg := assistantWith(message.ToolCall{
		ID:       "call1",
		Name:     "read",
		Input:    `{"filePath":"main.go"}`,
		Finished: true,
	})
	results := []message.Message{{
		ID:   "r1",
		Role: message.User,
		Parts: []message.ContentPart{
			message.ToolResult{ToolCallID: "call1", Content: "SECRET RESPONSE BODY"},
		},
	}}
	join := func(msgs []uiMessage) string {
		var b strings.Builder
		for _, m := range msgs {
			b.WriteString(m.content)
			b.WriteString("\n")
		}
		return ansi.Strip(b.String())
	}

	shown := renderAssistantMessage(msg, 0, results, nil, "", false, true, true, false, 80, 0)
	if len(shown) == 0 {
		t.Fatal("want a rendered tool block")
	}
	if out := join(shown); !strings.Contains(out, "SECRET RESPONSE BODY") {
		t.Errorf("details shown must render the response, got %q", out)
	}

	hidden := renderAssistantMessage(msg, 0, results, nil, "", false, true, false, false, 80, 0)
	if len(hidden) == 0 {
		t.Fatal("want a rendered tool block")
	}
	out := join(hidden)
	if strings.Contains(out, "SECRET RESPONSE BODY") {
		t.Errorf("details hidden must drop the response, got %q", out)
	}
	if !strings.Contains(out, "main.go") {
		t.Errorf("details hidden must keep the params, got %q", out)
	}
}

func TestUiMessageZoneDefault(t *testing.T) {
	u := uiMessage{ID: "abc"}
	if got := u.zone(); got != "msg:abc" {
		t.Errorf("zone() = %q, want msg:abc", got)
	}
	u.zoneID = "think:abc"
	if got := u.zone(); got != "think:abc" {
		t.Errorf("zone() = %q, want think:abc", got)
	}
}
