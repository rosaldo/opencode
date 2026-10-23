package tui

import (
	"context"
	"fmt"
	"slices"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/opencode-ai/opencode/internal/logging"
	"github.com/opencode-ai/opencode/internal/message"
	"github.com/opencode-ai/opencode/internal/snapshot"
	"github.com/opencode-ai/opencode/internal/tui/components/chat"
	"github.com/opencode-ai/opencode/internal/tui/util"
)

// stagedRevert is the "N messages reverted" state shared by /undo and the
// timeline Revert action (Spec 09u, parity with the TS session.revert). It
// survives until the user clears it (redo restores the files and messages) or
// commits it (a new turn makes the truncation and file rollback permanent).
type stagedRevert struct {
	sessionID  string
	messageID  string
	batchIndex int
	count      int
	files      []snapshot.FileDiff

	snapshotBefore string
	snapshotAfter  string
}

// bannerText renders the staged-revert banner shown above the prompt.
func (r *stagedRevert) bannerText() string {
	label := "messages"
	if r.count == 1 {
		label = "message"
	}
	files := ""
	if len(r.files) == 1 {
		files = " · 1 file rolled back"
	} else if len(r.files) > 1 {
		files = fmt.Sprintf(" · %d files rolled back", len(r.files))
	}
	return fmt.Sprintf("◇ %d %s reverted%s · redo restores", r.count, label, files)
}

// boundarySnapshot returns the worktree snapshot captured before the turn that
// produced the message at idx. User messages carry their own snapshot; for any
// other role the snapshot of the nearest preceding user message applies.
func boundarySnapshot(msgs []message.Message, idx int) string {
	for i := idx; i >= 0; i-- {
		if msgs[i].Role != message.User {
			continue
		}
		if s := msgs[i].SnapshotContent(); s != nil {
			return s.SnapshotID
		}
	}
	return ""
}

// stageRevert truncates the transcript at idx (inclusive), stashes the removed
// messages for redo, rolls the worktree back to the snapshot captured before
// that turn and sets the staged-revert banner. restorePrompt gives the user
// message text back to the editor (used by /undo and by reverting on a user
// message).
func (a appModel) stageRevert(msgs []message.Message, idx int, restorePrompt bool) (appModel, tea.Cmd) {
	ctx := context.Background()
	removed := msgs[idx:]

	// Only one revert may be staged at a time (a global slot, unlike the TS
	// per-session session.revert): staging over the same session commits the
	// previous one; another session must be cleared first.
	if a.revert != nil {
		if a.revert.sessionID != a.selectedSession.ID {
			return a, util.ReportWarn("Another session has a staged revert; clear it first")
		}
		a = a.commitRevertSilently()
	}

	snapshotBefore := boundarySnapshot(msgs, idx)
	var snapshotAfter string
	var files []snapshot.FileDiff
	if a.app.Snapshots != nil && a.app.Snapshots.Enabled() {
		if snap, err := a.app.Snapshots.Capture(ctx); err == nil && snap != nil {
			snapshotAfter = snap.ID
		}
		if snapshotBefore != "" {
			if err := a.app.Snapshots.Restore(ctx, snapshotBefore, nil); err != nil {
				logging.Warn("Failed to roll back the worktree on revert", "error", err)
			} else if snapshotAfter != "" {
				// The rolled-back changes are the diff between the post-turn
				// state (captured above) and the restored pre-turn snapshot.
				if diffs, diffErr := a.app.Snapshots.Diff(ctx, snapshotAfter, snapshotBefore, nil); diffErr == nil {
					files = diffs
					// Files created during the turn are absent from the
					// pre-turn tree ("D" in after→before); remove them.
					var created []string
					for _, d := range diffs {
						if d.Status == "D" {
							created = append(created, d.File)
						}
					}
					if len(created) > 0 {
						if rmErr := a.app.Snapshots.RemoveFiles(ctx, created); rmErr != nil {
							logging.Warn("Failed to remove files created by the reverted turn", "error", rmErr)
						}
					}
				}
			}
		}
	}

	for _, m := range removed {
		if err := a.app.Messages.Delete(ctx, m.ID); err != nil {
			return a, util.ReportError(err)
		}
	}
	a.undone = append(a.undone, undoneBatch{
		sessionID:      a.selectedSession.ID,
		messages:       removed,
		snapshotBefore: snapshotBefore,
		snapshotAfter:  snapshotAfter,
	})

	userCount := 0
	for _, m := range removed {
		if m.Role == message.User {
			userCount++
		}
	}
	a.revert = &stagedRevert{
		sessionID:      a.selectedSession.ID,
		messageID:      removed[0].ID,
		batchIndex:     len(a.undone) - 1,
		count:          userCount,
		files:          files,
		snapshotBefore: snapshotBefore,
		snapshotAfter:  snapshotAfter,
	}

	cmds := []tea.Cmd{
		util.CmdHandler(chat.RevertBannerMsg{Text: a.revert.bannerText()}),
		util.ReportInfo(fmt.Sprintf("Reverted %d message%s", userCount, pluralS(userCount))),
	}
	// Give the prompt text back so the user can edit and resend it. An
	// existing draft is left alone.
	if restorePrompt && removed[0].Role == message.User {
		if text := chat.MessagePlainText(removed[0]); text != "" {
			cmds = append(cmds, util.CmdHandler(chat.RestorePromptMsg{Text: text}))
		}
	}
	return a, tea.Batch(cmds...)
}

func pluralS(n int) string {
	if n == 1 {
		return ""
	}
	return "s"
}

// commitRevertSilently drops the staged revert state (and its redo batch)
// without touching the transcript or the worktree: the truncation and the file
// rollback become permanent.
func (a appModel) commitRevertSilently() appModel {
	if a.revert == nil {
		return a
	}
	if a.revert.batchIndex >= 0 && a.revert.batchIndex < len(a.undone) {
		a.undone = slices.Delete(a.undone, a.revert.batchIndex, a.revert.batchIndex+1)
	}
	a.revert = nil
	return a
}

// commitRevert is the CommitRevertMsg handler: it fires when a new turn starts
// while a revert is staged in the active session (Spec 09u).
func (a appModel) commitRevert() (appModel, tea.Cmd) {
	if a.revert == nil || a.revert.sessionID != a.selectedSession.ID {
		return a, nil
	}
	a = a.commitRevertSilently()
	return a, util.CmdHandler(chat.RevertBannerMsg{Text: ""})
}

// clearRevert cancels the staged revert: files go back to the post-turn
// snapshot and the removed messages are re-created (Spec 09u, the TS
// session.revert.clear behind /redo).
func (a appModel) clearRevert() (appModel, tea.Cmd) {
	if a.app == nil {
		return a, util.ReportWarn("No active session to redo")
	}
	if a.revert == nil || a.revert.sessionID != a.selectedSession.ID {
		return a, util.ReportWarn("Nothing to redo in this session")
	}
	if a.app.Agents.IsSessionBusy(a.selectedSession.ID) {
		return a, util.ReportWarn("Agent is working, please wait...")
	}
	if a.revert.batchIndex < 0 || a.revert.batchIndex >= len(a.undone) {
		a.revert = nil
		return a, util.ReportWarn("Nothing to redo")
	}
	batch := a.undone[a.revert.batchIndex]
	ctx := context.Background()

	if batch.snapshotAfter != "" && a.app.Snapshots != nil && a.app.Snapshots.Enabled() {
		if err := a.app.Snapshots.Restore(ctx, batch.snapshotAfter, nil); err != nil {
			logging.Warn("Failed to restore the worktree on redo", "error", err)
		} else {
			// The revert restored files the agent had deleted ("A" in
			// after→before); clearing must delete them again.
			var restoredDeletions []string
			for _, d := range a.revert.files {
				if d.Status == "A" {
					restoredDeletions = append(restoredDeletions, d.File)
				}
			}
			if len(restoredDeletions) > 0 {
				if rmErr := a.app.Snapshots.RemoveFiles(ctx, restoredDeletions); rmErr != nil {
					logging.Warn("Failed to remove files restored by the reverted turn", "error", rmErr)
				}
			}
		}
	}

	restored := 0
	for _, m := range batch.messages {
		parts := m.Parts
		if m.Role != message.Assistant {
			// The service re-appends a Finish marker for these roles.
			parts = stripFinishParts(parts)
		}
		if _, err := a.app.Messages.Create(ctx, batch.sessionID, message.CreateMessageParams{
			Role:  m.Role,
			Parts: parts,
			Model: m.Model,
		}); err != nil {
			return a, util.ReportError(err)
		}
		restored++
	}

	a.undone = slices.Delete(a.undone, a.revert.batchIndex, a.revert.batchIndex+1)
	a.revert = nil
	return a, tea.Batch(
		util.CmdHandler(chat.RevertBannerMsg{Text: ""}),
		util.ReportInfo(fmt.Sprintf("Revert cleared (%d message%s restored)", restored, pluralS(restored))),
	)
}

// revertToMessage stages a revert at the chosen message (Spec 09u: the
// "Revert" action of the message actions dialog): the working tree rolls back
// to the snapshot captured before that turn and the banner shows the count.
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
	return a.stageRevert(msgs, idx, true)
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
