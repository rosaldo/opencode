package dialog

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/opencode-ai/opencode/internal/session"
)

func sampleSessions() []session.Session {
	return []session.Session{
		{ID: "1", Title: "alpha"},
		{ID: "2", Title: "beta"},
		{ID: "3", Title: "gamma"},
	}
}

func TestSessionSearchFilters(t *testing.T) {
	d := NewSessionDialogCmp().(*sessionDialogCmp)
	d.SetSessions(sampleSessions())

	// Enter search mode via the "/" key.
	model, _ := d.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("/")})
	d = model.(*sessionDialogCmp)
	if d.mode != sessionModeSearch {
		t.Fatalf("mode = %v, want search", d.mode)
	}
	d.input.SetValue("be")
	d.refreshFiltered()
	if len(d.sessions) != 1 || d.sessions[0].Title != "beta" {
		t.Fatalf("filtered = %+v", d.sessions)
	}
}

func TestSessionRenameIntent(t *testing.T) {
	d := NewSessionDialogCmp().(*sessionDialogCmp)
	d.SetSessions(sampleSessions())

	model, _ := d.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("r")})
	d = model.(*sessionDialogCmp)
	if d.mode != sessionModeRename {
		t.Fatalf("mode = %v, want rename", d.mode)
	}
	if d.input.Value() != "alpha" {
		t.Fatalf("rename input = %q", d.input.Value())
	}
	d.input.SetValue("renamed")
	_, cmd := d.Update(tea.KeyMsg{Type: tea.KeyEnter})
	msg := cmd()
	r, ok := msg.(SessionRenameRequestMsg)
	if !ok {
		t.Fatalf("msg = %T, want SessionRenameRequestMsg", msg)
	}
	if r.Title != "renamed" || r.Session.ID != "1" {
		t.Fatalf("rename = %+v", r)
	}
}

func TestSessionDeleteRequiresConfirm(t *testing.T) {
	d := NewSessionDialogCmp().(*sessionDialogCmp)
	d.SetSessions(sampleSessions())

	model, cmd := d.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("d")})
	d = model.(*sessionDialogCmp)
	if !d.pendingDelete || cmd != nil {
		t.Fatalf("delete should arm confirmation, pendingDelete=%v", d.pendingDelete)
	}

	_, cmd = d.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("y")})
	msg := cmd()
	del, ok := msg.(SessionDeleteRequestMsg)
	if !ok {
		t.Fatalf("msg = %T, want SessionDeleteRequestMsg", msg)
	}
	if del.Session.ID != "1" {
		t.Fatalf("deleted = %+v", del)
	}
}

func TestSessionDeleteCancelled(t *testing.T) {
	d := NewSessionDialogCmp().(*sessionDialogCmp)
	d.SetSessions(sampleSessions())
	model, _ := d.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("d")})
	d = model.(*sessionDialogCmp)
	model, cmd := d.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("n")})
	d = model.(*sessionDialogCmp)
	if d.pendingDelete {
		t.Fatal("pendingDelete should be cleared by non-confirm key")
	}
	if cmd != nil {
		t.Fatal("cancel should not emit a command")
	}
}
