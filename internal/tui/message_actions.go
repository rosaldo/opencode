package tui

import (
	"context"
	"fmt"
	"slices"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/opencode-ai/opencode/internal/message"
	"github.com/opencode-ai/opencode/internal/tui/components/chat"
	"github.com/opencode-ai/opencode/internal/tui/util"
)

// revertToMessage deletes the chosen message and every message after it,
// stashing the batch for /redo and giving the prompt text back for a user
// message (Spec 09u: the "Revert" action of the message actions dialog). Like
// /undo, the working tree is not touched: the git rollback of the TS
// implementation is deferred (Spec 09k).
func (a appModel) revertToMessage(messageID string) (appModel, tea.Cmd) {
	if a.app == nil || a.selectedSession.ID == "" {
		return a, util.ReportWarn("No active session to revert")
	}
	if a.app.Agents.IsSessionBusy(a.selectedSession.ID) {
		return a, util.ReportWarn("Agent is working, please wait...")
	}
	ctx := context.Background()
	msgs, err := a.app.Messages.List(ctx, a.selectedSession.ID)
	if err != nil {
		return a, util.ReportError(err)
	}
	idx := slices.IndexFunc(msgs, func(m message.Message) bool {
		return m.ID == messageID
	})
	if idx < 0 {
		return a, util.ReportWarn("Message not found in this session")
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
		util.ReportInfo(fmt.Sprintf("Reverted to before the message (%d removed)", len(removed))),
	}
	// Give the prompt text back so the user can edit and resend it. An
	// existing draft is left alone (Spec 09u).
	if removed[0].Role == message.User {
		if text := chat.MessagePlainText(removed[0]); text != "" {
			cmds = append(cmds, util.CmdHandler(chat.RestorePromptMsg{Text: text}))
		}
	}
	return a, tea.Batch(cmds...)
}

// copyMessageByID puts the text of the chosen message on the clipboard
// (Spec 09u: the "Copy" action of the message actions dialog).
func (a appModel) copyMessageByID(messageID string) (appModel, tea.Cmd) {
	if a.app == nil {
		return a, util.ReportWarn("No active session")
	}
	ctx := context.Background()
	msg, err := a.app.Messages.Get(ctx, messageID)
	if err != nil {
		return a, util.ReportError(err)
	}
	text := chat.MessagePlainText(msg)
	if text == "" {
		return a, util.ReportWarn("That message has no text to copy")
	}
	if err := util.WriteClipboardText(text); err != nil {
		return a, util.ReportError(err)
	}
	return a, util.ReportInfo("Message copied to clipboard")
}
