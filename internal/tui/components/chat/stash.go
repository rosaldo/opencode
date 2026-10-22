package chat

import (
	"slices"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/opencode-ai/opencode/internal/promptstash"
	"github.com/opencode-ai/opencode/internal/tui/util"
)

type (
	// StashPushMsg saves the current prompt into the stash and clears the
	// composer (Spec 09q).
	StashPushMsg struct{}
	// StashPopMsg restores the most recently stashed prompt (LIFO).
	StashPopMsg struct{}
	// RequestStashListMsg asks the editor for the stashed prompts; the editor
	// replies with StashListMsg (or warns when the stash is empty).
	RequestStashListMsg struct{}
	// StashListMsg carries a snapshot of the stash for the stash dialog.
	StashListMsg struct {
		Entries []promptstash.Entry
	}
	// StashTakeMsg restores the entry at Index into the composer and removes
	// it from the stash (overwrite, unlike RestorePromptMsg).
	StashTakeMsg struct {
		Index int
	}
	// StashRemoveMsg drops the entry at Index from the stash.
	StashRemoveMsg struct {
		Index int
	}
)

func (m *editorCmp) stashPush() (tea.Model, tea.Cmd) {
	text := m.textarea.Value()
	if strings.TrimSpace(text) == "" {
		return m, util.ReportWarn("Nothing to stash")
	}
	m.stash.Push(text, m.attachments)
	m.textarea.SetValue("")
	m.textarea.CursorEnd()
	m.attachments = nil
	m.shellMode = false
	m.deleteMode = false
	return m, nil
}

func (m *editorCmp) stashPop() (tea.Model, tea.Cmd) {
	entry, ok := m.stash.Pop()
	if !ok {
		return m, util.ReportWarn("No stashed prompts")
	}
	m.restoreStashEntry(entry)
	return m, nil
}

func (m *editorCmp) stashList() (tea.Model, tea.Cmd) {
	entries := m.stash.List()
	if len(entries) == 0 {
		return m, util.ReportWarn("No stashed prompts")
	}
	return m, util.CmdHandler(StashListMsg{Entries: entries})
}

func (m *editorCmp) stashTake(index int) (tea.Model, tea.Cmd) {
	entry, ok := m.stash.Take(index)
	if !ok {
		return m, nil
	}
	m.restoreStashEntry(entry)
	return m, nil
}

// restoreStashEntry overwrites the composer with a stashed prompt: text,
// attachments and shell mode (a leading "!" was typed as one when stashed).
func (m *editorCmp) restoreStashEntry(entry promptstash.Entry) {
	m.textarea.SetValue(entry.Text)
	m.textarea.CursorEnd()
	m.attachments = slices.Clone(entry.Attachments)
	m.shellMode = strings.HasPrefix(entry.Text, "!")
	m.deleteMode = false
}
