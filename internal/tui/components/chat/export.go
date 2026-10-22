package chat

import (
	"fmt"
	"strings"
	"time"

	"github.com/opencode-ai/opencode/internal/message"
)

// SessionToMarkdown renders a conversation as Markdown so /export (Spec 09g)
// can hand it to the user's $EDITOR. Only parts that carry meaning are kept:
// text, reasoning, tool calls and tool results; finish markers and images are
// dropped.
func SessionToMarkdown(title string, msgs []message.Message) string {
	var b strings.Builder

	heading := strings.TrimSpace(title)
	if heading == "" {
		heading = "Session"
	}
	fmt.Fprintf(&b, "# %s\n\n", heading)
	noun := "messages"
	if len(msgs) == 1 {
		noun = "message"
	}
	fmt.Fprintf(&b, "> %d %s · exported %s\n",
		len(msgs), noun, time.Now().UTC().Format("2006-01-02 15:04 UTC"))

	for _, msg := range msgs {
		role := "Assistant"
		switch msg.Role {
		case message.User:
			role = "User"
		case message.System:
			role = "System"
		}
		fmt.Fprintf(&b, "\n## %s\n\n", role)

		for _, part := range msg.Parts {
			switch p := part.(type) {
			case message.TextContent:
				if text := strings.TrimSpace(p.Text); text != "" {
					b.WriteString(text)
					b.WriteString("\n\n")
				}
			case message.ReasoningContent:
				if text := strings.TrimSpace(p.Thinking); text != "" {
					fmt.Fprintf(&b, "**Thinking:**\n\n```\n%s\n```\n\n", text)
				}
			case message.ToolCall:
				fmt.Fprintf(&b, "**Tool:** `%s`\n\n", p.Name)
				if input := strings.TrimSpace(p.Input); input != "" {
					fmt.Fprintf(&b, "```json\n%s\n```\n\n", input)
				}
			case message.ToolResult:
				label := p.Name
				if label == "" {
					label = "tool result"
				}
				marker := "Tool result"
				if p.IsError {
					marker = "Tool error"
				}
				fmt.Fprintf(&b, "**%s:** `%s`\n\n", marker, label)
				if content := strings.TrimSpace(p.Content); content != "" {
					fmt.Fprintf(&b, "```\n%s\n```\n\n", content)
				}
			}
		}
	}

	return strings.TrimRight(b.String(), "\n") + "\n"
}
