package chat

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/opencode-ai/opencode/internal/message"
	"github.com/opencode-ai/opencode/internal/promptstash"
	"github.com/opencode-ai/opencode/internal/tui/util"
)

func newStashEditor() *editorCmp {
	return &editorCmp{
		textarea: CreateTextArea(nil),
		stash:    promptstash.Open(""),
	}
}

func warnFrom(t *testing.T, cmd tea.Cmd) string {
	t.Helper()
	if cmd == nil {
		t.Fatal("expected a report command, got nil")
	}
	msg, ok := cmd().(util.InfoMsg)
	if !ok {
		t.Fatalf("msg = %T, want util.InfoMsg", cmd())
	}
	if msg.Type != util.InfoTypeWarn {
		t.Fatalf("InfoMsg.Type = %v, want warn", msg.Type)
	}
	return msg.Msg
}

func TestStashPushClearsComposer(t *testing.T) {
	m := newStashEditor()
	m.textarea.SetValue("park me")
	m.attachments = []message.Attachment{{FileName: "a.txt"}}
	m.shellMode = true

	_, _ = m.Update(StashPushMsg{})

	if got := m.textarea.Value(); got != "" {
		t.Fatalf("textarea = %q, want empty", got)
	}
	if len(m.attachments) != 0 {
		t.Fatalf("attachments = %d, want 0", len(m.attachments))
	}
	if m.shellMode || m.deleteMode {
		t.Fatal("modes must reset when stashing")
	}
	if m.stash.Len() != 1 {
		t.Fatalf("stash Len() = %d, want 1", m.stash.Len())
	}
	entry := m.stash.List()[0]
	if entry.Text != "park me" || len(entry.Attachments) != 1 {
		t.Fatalf("entry = %+v", entry)
	}
}

func TestStashPushGuardsEmptyPrompt(t *testing.T) {
	m := newStashEditor()
	_, cmd := m.Update(StashPushMsg{})
	if got := warnFrom(t, cmd); got != "Nothing to stash" {
		t.Fatalf("warn = %q", got)
	}
	m.textarea.SetValue("   ")
	_, cmd = m.Update(StashPushMsg{})
	if cmd == nil {
		t.Fatal("whitespace-only prompt must warn")
	}
	if m.stash.Len() != 0 {
		t.Fatalf("stash Len() = %d, want 0", m.stash.Len())
	}
}

func TestStashPopRestoresOverwrite(t *testing.T) {
	m := newStashEditor()
	m.stash.Push("!ls -la", []message.Attachment{{FileName: "b.txt"}})
	m.stash.Push("plain", nil)
	m.textarea.SetValue("current draft")
	m.attachments = []message.Attachment{{FileName: "keep.txt"}}

	_, _ = m.Update(StashPopMsg{})

	if got := m.textarea.Value(); got != "plain" {
		t.Fatalf("textarea = %q, want plain", got)
	}
	if m.shellMode {
		t.Fatal("plain text must not enter shell mode")
	}
	// LIFO: the second stash is popped first, overwriting the draft.
	if m.stash.Len() != 1 {
		t.Fatalf("stash Len() = %d, want 1", m.stash.Len())
	}

	_, _ = m.Update(StashPopMsg{})
	if got := m.textarea.Value(); got != "!ls -la" {
		t.Fatalf("textarea = %q, want !ls -la", got)
	}
	if !m.shellMode {
		t.Fatal("restored !command must re-enter shell mode")
	}
	if len(m.attachments) != 1 || m.attachments[0].FileName != "b.txt" {
		t.Fatalf("attachments = %+v", m.attachments)
	}
}

func TestStashPopGuardsEmptyStash(t *testing.T) {
	m := newStashEditor()
	_, cmd := m.Update(StashPopMsg{})
	if got := warnFrom(t, cmd); got != "No stashed prompts" {
		t.Fatalf("warn = %q", got)
	}
}

func TestStashListRepliesWithEntries(t *testing.T) {
	m := newStashEditor()
	m.stash.Push("one", nil)
	m.stash.Push("two", nil)

	_, cmd := m.Update(RequestStashListMsg{})
	if cmd == nil {
		t.Fatal("expected StashListMsg command")
	}
	msg, ok := cmd().(StashListMsg)
	if !ok {
		t.Fatalf("msg = %T, want StashListMsg", cmd())
	}
	if len(msg.Entries) != 2 || msg.Entries[0].Text != "one" || msg.Entries[1].Text != "two" {
		t.Fatalf("entries = %+v", msg.Entries)
	}
}

func TestStashListGuardsEmptyStash(t *testing.T) {
	m := newStashEditor()
	_, cmd := m.Update(RequestStashListMsg{})
	if got := warnFrom(t, cmd); got != "No stashed prompts" {
		t.Fatalf("warn = %q", got)
	}
}

func TestStashTakeRestoresAndRemoves(t *testing.T) {
	m := newStashEditor()
	m.stash.Push("oldest", nil)
	m.stash.Push("middle", nil)
	m.stash.Push("newest", nil)

	_, _ = m.Update(StashTakeMsg{Index: 1})

	if got := m.textarea.Value(); got != "middle" {
		t.Fatalf("textarea = %q, want middle", got)
	}
	if m.stash.Len() != 2 {
		t.Fatalf("stash Len() = %d, want 2", m.stash.Len())
	}
	if m.stash.List()[0].Text != "oldest" || m.stash.List()[1].Text != "newest" {
		t.Fatalf("stash = %+v", m.stash.List())
	}

	// Out-of-range take is a no-op.
	_, _ = m.Update(StashTakeMsg{Index: 9})
	if m.stash.Len() != 2 {
		t.Fatalf("stash Len() = %d, want 2", m.stash.Len())
	}
}

func TestStashRemoveDropsEntry(t *testing.T) {
	m := newStashEditor()
	m.stash.Push("one", nil)
	m.stash.Push("two", nil)

	_, _ = m.Update(StashRemoveMsg{Index: 0})

	if m.stash.Len() != 1 || m.stash.List()[0].Text != "two" {
		t.Fatalf("stash = %+v", m.stash.List())
	}
}

func TestStashMessagesAreSafeWithoutStore(t *testing.T) {
	m := &editorCmp{textarea: CreateTextArea(nil)}
	_, cmd := m.Update(StashPushMsg{})
	if cmd == nil {
		t.Fatal("push without store must warn")
	}
	_, _ = m.Update(StashPopMsg{})
	_, _ = m.Update(StashTakeMsg{Index: 0})
	_, _ = m.Update(StashRemoveMsg{Index: 0})
	_, cmd = m.Update(RequestStashListMsg{})
	if cmd == nil {
		t.Fatal("list without store must warn")
	}
}
