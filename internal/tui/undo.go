package tui

import (
	"context"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/opencode-ai/opencode/internal/message"
	"github.com/opencode-ai/opencode/internal/tui/util"
)

// undoMessageMsg stages a revert of the newest user message: it and everything
// after it are removed and the worktree is rolled back to the snapshot
// captured before the turn (Spec 09k: the /undo command and the messages_undo
// keybind).
type undoMessageMsg struct{}

// redoMessageMsg clears the staged revert, restoring the messages and the
// worktree (Spec 09k/09u, parity with the TS session.revert.clear).
type redoMessageMsg struct{}

// undoneBatch is one undo unit: a user message and every message that
// followed it, kept in order so redo can put them back. snapshotBefore is the
// worktree state before the turn (file rollback); snapshotAfter is captured on
// undo so redo can restore the post-turn state.
type undoneBatch struct {
	sessionID      string
	messages       []message.Message
	snapshotBefore string
	snapshotAfter  string
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

// undoMessage stages a revert on the newest user message: the transcript is
// truncated, the worktree rolls back to the snapshot captured before the turn
// and the prompt text goes back to the editor (Spec 09k). /redo (redoMessage)
// cancels the staged revert.
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
	return a.stageRevert(msgs, idx, true)
}

// redoMessage clears the staged revert: it re-creates the removed messages and
// restores the worktree to the post-turn snapshot (Spec 09k/09u). Without a
// staged revert there is nothing to redo — matching the TS /redo.
func (a appModel) redoMessage() (appModel, tea.Cmd) {
	if a.revert != nil {
		return a.clearRevert()
	}
	return a, util.ReportWarn("Nothing to redo")
}
