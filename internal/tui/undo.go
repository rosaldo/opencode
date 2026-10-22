package tui

import (
	"context"
	"fmt"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/opencode-ai/opencode/internal/message"
	"github.com/opencode-ai/opencode/internal/tui/components/chat"
	"github.com/opencode-ai/opencode/internal/tui/util"
)

// undoMessageMsg removes the newest user message and everything after it
// (Spec 09k: the /undo command and the messages_undo keybind).
type undoMessageMsg struct{}

// redoMessageMsg restores the batch removed by the previous undo.
type redoMessageMsg struct{}

// undoneBatch is one undo unit: a user message and every message that
// followed it, kept in order so /redo can put them back.
type undoneBatch struct {
	sessionID string
	messages  []message.Message
}

// lastUserMessageIndex returns the index of the newest user message, or -1
// when the conversation has none.
func lastUserMessageIndex(msgs []message.Message) int {
	for i := len(msgs) - 1; i >= 0; i-- {
		if msgs[i].Role == message.User {
			return i
		}
	}
	return -1
}

// stripFinishParts drops the synthetic Finish markers the message service
// appends on create, so a restored message does not end up with two of them.
func stripFinishParts(parts []message.ContentPart) []message.ContentPart {
	stripped := make([]message.ContentPart, 0, len(parts))
	for _, part := range parts {
		if _, isFinish := part.(message.Finish); isFinish {
			continue
		}
		stripped = append(stripped, part)
	}
	return stripped
}

// undoMessage deletes the newest user message and every message after it,
// stashing the batch for redo and putting the prompt text back into the
// editor. The working tree is not touched: the git rollback of the TS
// implementation is deferred (Spec 09k).
func (a appModel) undoMessage() (appModel, tea.Cmd) {
	if a.app == nil || a.selectedSession.ID == "" {
		return a, util.ReportWarn("No active session to undo")
	}
	if a.app.Agents.IsSessionBusy(a.selectedSession.ID) {
		return a, util.ReportWarn("Agent is working, please wait...")
	}
	ctx := context.Background()
	msgs, err := a.app.Messages.List(ctx, a.selectedSession.ID)
	if err != nil {
		return a, util.ReportError(err)
	}
	idx := lastUserMessageIndex(msgs)
	if idx < 0 {
		return a, util.ReportWarn("Nothing to undo")
	}
	removed := msgs[idx:]
	for _, m := range removed {
		if err := a.app.Messages.Delete(ctx, m.ID); err != nil {
			return a, util.ReportError(err)
		}
	}
	a.undone = append(a.undone, undoneBatch{
		sessionID: a.selectedSession.ID,
		messages:  removed,
	})
	cmds := []tea.Cmd{
		util.ReportInfo(fmt.Sprintf("Undid the last message (%d removed)", len(removed))),
	}
	// Give the prompt text back so the user can edit and resend it. An
	// existing draft is left alone.
	if text := chat.MessagePlainText(removed[0]); text != "" {
		cmds = append(cmds, util.CmdHandler(chat.RestorePromptMsg{Text: text}))
	}
	return a, tea.Batch(cmds...)
}

// redoMessage re-creates the messages removed by the last undo in the same
// session. It restores the transcript only; the agent is not re-run and the
// working tree stays untouched (Spec 09k).
func (a appModel) redoMessage() (appModel, tea.Cmd) {
	if a.app == nil || a.selectedSession.ID == "" {
		return a, util.ReportWarn("No active session to redo")
	}
	if len(a.undone) == 0 {
		return a, util.ReportWarn("Nothing to redo")
	}
	last := a.undone[len(a.undone)-1]
	if last.sessionID != a.selectedSession.ID {
		return a, util.ReportWarn("Nothing to redo in this session")
	}
	if a.app.Agents.IsSessionBusy(a.selectedSession.ID) {
		return a, util.ReportWarn("Agent is working, please wait...")
	}
	ctx := context.Background()
	for _, m := range last.messages {
		parts := m.Parts
		if m.Role != message.Assistant {
			// The service re-appends a Finish marker for these roles.
			parts = stripFinishParts(parts)
		}
		if _, err := a.app.Messages.Create(ctx, last.sessionID, message.CreateMessageParams{
			Role:  m.Role,
			Parts: parts,
			Model: m.Model,
		}); err != nil {
			return a, util.ReportError(err)
		}
	}
	a.undone = a.undone[:len(a.undone)-1]
	return a, util.ReportInfo(fmt.Sprintf("Redid the last message (%d restored)", len(last.messages)))
}
