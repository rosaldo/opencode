package dialog

import (
	"testing"

	"github.com/opencode-ai/opencode/internal/llm/models"
	"github.com/opencode-ai/opencode/internal/session"
	"github.com/opencode-ai/opencode/internal/tui/theme"
)

func TestSessionDialogClickRow(t *testing.T) {
	cmp, ok := NewSessionDialogCmp().(*sessionDialogCmp)
	if !ok {
		t.Fatal("NewSessionDialogCmp must return *sessionDialogCmp")
	}
	cmp.SetSessions([]session.Session{
		{ID: "s1", Title: "one"},
		{ID: "s2", Title: "two"},
	})

	_, cmd := cmp.clickRow(1)
	if cmd == nil {
		t.Fatal("clicking a valid row must select the session")
	}
	msg, ok := cmd().(SessionSelectedMsg)
	if !ok {
		t.Fatal("click must emit SessionSelectedMsg")
	}
	if msg.Session.ID != "s2" {
		t.Errorf("session = %q, want s2", msg.Session.ID)
	}
	if cmp.selectedIdx != 1 {
		t.Errorf("selectedIdx = %d, want 1", cmp.selectedIdx)
	}

	if _, cmd := cmp.clickRow(99); cmd != nil {
		t.Error("out-of-range clicks must be ignored")
	}
	if _, cmd := cmp.clickRow(-1); cmd != nil {
		t.Error("negative clicks must be ignored")
	}
	if cmp.selectedIdx != 1 {
		t.Errorf("selectedIdx = %d, want selection unchanged", cmp.selectedIdx)
	}
}

func TestModelDialogClickRow(t *testing.T) {
	cmp, ok := NewModelDialogCmp(nil).(*modelDialogCmp)
	if !ok {
		t.Fatal("NewModelDialogCmp must return *modelDialogCmp")
	}
	cmp.models = []models.Model{{Name: "fast"}, {Name: "smart"}}

	_, cmd := cmp.clickRow(1)
	if cmd == nil {
		t.Fatal("clicking a valid row must select the model")
	}
	msg, ok := cmd().(ModelSelectedMsg)
	if !ok {
		t.Fatal("click must emit ModelSelectedMsg")
	}
	if msg.Model.Name != "smart" {
		t.Errorf("model = %q, want smart", msg.Model.Name)
	}
	if cmp.selectedIdx != 1 {
		t.Errorf("selectedIdx = %d, want 1", cmp.selectedIdx)
	}

	if _, cmd := cmp.clickRow(99); cmd != nil {
		t.Error("out-of-range clicks must be ignored")
	}
	if _, cmd := cmp.clickRow(-1); cmd != nil {
		t.Error("negative clicks must be ignored")
	}
}

func TestThemeDialogClickRow(t *testing.T) {
	cmp, ok := NewThemeDialogCmp().(*themeDialogCmp)
	if !ok {
		t.Fatal("NewThemeDialogCmp must return *themeDialogCmp")
	}
	current := theme.CurrentThemeName()
	cmp.themes = []string{current, "opencode-dark"}

	// Clicking the already-active theme closes the dialog (Spec 09n), same as
	// Enter.
	_, cmd := cmp.clickRow(0)
	if cmd == nil {
		t.Fatal("clicking the current theme must close the dialog")
	}
	if _, ok := cmd().(CloseThemeDialogMsg); !ok {
		t.Fatalf("msg = %T, want CloseThemeDialogMsg", cmd())
	}

	if _, cmd := cmp.clickRow(99); cmd != nil {
		t.Error("out-of-range clicks must be ignored")
	}
	if _, cmd := cmp.clickRow(-1); cmd != nil {
		t.Error("negative clicks must be ignored")
	}
}
