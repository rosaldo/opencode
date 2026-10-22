package chat

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

func newHistoryEditor() *editorCmp {
	return &editorCmp{textarea: CreateTextArea(nil)}
}

func TestRecordHistoryAppendsAndCollapses(t *testing.T) {
	m := newHistoryEditor()
	m.recordHistory("")
	if len(m.history) != 0 {
		t.Fatalf("empty prompt must not be recorded, got %q", m.history)
	}
	m.recordHistory("first")
	m.recordHistory("first")
	m.recordHistory("second")
	if len(m.history) != 2 || m.history[0] != "first" || m.history[1] != "second" {
		t.Errorf("history = %q, want [first second]", m.history)
	}
	if m.historyIdx != len(m.history) {
		t.Errorf("historyIdx = %d, want %d (live draft)", m.historyIdx, len(m.history))
	}
}

func TestHistoryWalkBackAndForwardRestoresDraft(t *testing.T) {
	m := newHistoryEditor()
	m.recordHistory("one")
	m.recordHistory("two")
	m.textarea.SetValue("draft")

	if !m.canHistoryPrevious() {
		t.Fatal("up must be available on a single-line draft")
	}
	m.historyPrevious()
	if got := m.textarea.Value(); got != "two" {
		t.Errorf("first up = %q, want the newest prompt", got)
	}
	m.historyPrevious()
	if got := m.textarea.Value(); got != "one" {
		t.Errorf("second up = %q, want the oldest prompt", got)
	}
	m.historyPrevious()
	if m.historyIdx != 0 || m.textarea.Value() != "one" {
		t.Errorf("up at the oldest prompt must stay, got idx=%d value=%q", m.historyIdx, m.textarea.Value())
	}
	if m.historyDraft != "draft" {
		t.Errorf("historyDraft = %q, want the live draft", m.historyDraft)
	}

	if !m.canHistoryNext() {
		t.Fatal("down must be available while browsing")
	}
	m.historyNext()
	if got := m.textarea.Value(); got != "two" {
		t.Errorf("first down = %q, want the newest prompt", got)
	}
	m.historyNext()
	if got := m.textarea.Value(); got != "draft" {
		t.Errorf("down at the end = %q, want the live draft restored", got)
	}
	if m.canHistoryNext() {
		t.Error("down must stop once the live draft is back")
	}
}

func TestHistoryNextWithoutBrowsingIsNoop(t *testing.T) {
	m := newHistoryEditor()
	m.recordHistory("one")
	m.textarea.SetValue("draft")
	if m.canHistoryNext() {
		t.Fatal("down must not act while the live draft is shown")
	}
	m.historyNext()
	if got := m.textarea.Value(); got != "draft" {
		t.Errorf("value = %q, want the draft untouched", got)
	}
}

func TestHistoryGuards(t *testing.T) {
	m := newHistoryEditor()
	m.recordHistory("one")

	m.deleteMode = true
	if m.canHistoryPrevious() || m.canHistoryNext() {
		t.Error("history must not run in delete mode")
	}
	m.deleteMode = false
	m.shellMode = true
	if m.canHistoryPrevious() || m.canHistoryNext() {
		t.Error("history must not run in shell mode")
	}
	m.shellMode = false
	m.textarea.Blur()
	if m.canHistoryPrevious() || m.canHistoryNext() {
		t.Error("history must not run with a blurred prompt")
	}
	m.textarea.Focus()

	m.resetHistory()
	if len(m.history) != 0 || m.canHistoryPrevious() {
		t.Error("resetHistory must drop the history")
	}
}

func TestHistoryPreviousBlockedOnMultiLineDraft(t *testing.T) {
	m := newHistoryEditor()
	m.recordHistory("single")
	m.textarea.SetValue("line one\nline two")
	if m.canHistoryPrevious() {
		t.Fatal("up on a multi-line draft must keep moving the cursor")
	}

	// A multi-line *recorded* prompt must still be browsable: once the walk
	// started, up continues back regardless of the restored line count.
	m.resetHistory()
	m.recordHistory("x\ny")
	m.textarea.SetValue("draft")
	if !m.canHistoryPrevious() {
		t.Fatal("up must start the walk from a single-line draft")
	}
	m.historyPrevious()
	if got := m.textarea.Value(); got != "x\ny" {
		t.Fatalf("restored = %q, want the multi-line prompt", got)
	}
	if !m.canHistoryPrevious() {
		t.Error("up must continue while browsing a multi-line prompt")
	}
}

func TestHistoryKeyMessagesRouteThroughUpdate(t *testing.T) {
	m := newHistoryEditor()
	m.recordHistory("one")
	m.textarea.SetValue("draft")

	up := tea.KeyMsg{Type: tea.KeyUp}
	if _, cmd := m.Update(up); cmd != nil {
		t.Errorf("history up returned a command, want nil (got %v)", cmd)
	}
	if got := m.textarea.Value(); got != "one" {
		t.Errorf("up key restored %q, want the prompt", got)
	}

	down := tea.KeyMsg{Type: tea.KeyDown}
	m.Update(down)
	if got := m.textarea.Value(); got != "draft" {
		t.Errorf("down restored %q, want the draft", got)
	}
}

func TestRestorePromptMsg(t *testing.T) {
	m := newHistoryEditor()
	if _, cmd := m.Update(RestorePromptMsg{Text: "undone prompt"}); cmd != nil {
		t.Errorf("restore returned a command, want nil (got %v)", cmd)
	}
	if got := m.textarea.Value(); got != "undone prompt" {
		t.Errorf("value = %q, want the restored prompt", got)
	}

	// An existing draft wins over the restored text.
	m.Update(RestorePromptMsg{Text: "other"})
	if m.textarea.Value() != "undone prompt" {
		t.Errorf("value = %q, want the draft kept", m.textarea.Value())
	}
}
