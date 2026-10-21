package dialog

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/opencode-ai/opencode/internal/llm/models"
)

func TestConnectDialogRenderAndSubmit(t *testing.T) {
	d := NewConnectDialog()
	d.SetProvider(models.ProviderOpencode)
	d.Update(tea.WindowSizeMsg{Width: 120, Height: 40})

	view := d.View()
	for _, want := range []string{"Connect", "OpenCode Zen", "API key"} {
		if !strings.Contains(view, want) {
			t.Fatalf("view missing %q:\n%s", want, view)
		}
	}

	// Type a key and submit.
	d.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("secret")})
	_, cmd := d.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if cmd == nil {
		t.Fatal("expected submit command")
	}
	msg, ok := cmd().(SubmitConnectMsg)
	if !ok {
		t.Fatalf("expected SubmitConnectMsg, got %T", cmd())
	}
	if msg.Provider != models.ProviderOpencode || msg.APIKey != "secret" {
		t.Fatalf("unexpected submit: %+v", msg)
	}
}

func TestConnectDialogEmptyKeyIgnored(t *testing.T) {
	d := NewConnectDialog()
	_, cmd := d.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if cmd != nil {
		t.Fatalf("empty key should not submit, got %T", cmd())
	}
}

func TestConnectDialogEscape(t *testing.T) {
	d := NewConnectDialog()
	_, cmd := d.Update(tea.KeyMsg{Type: tea.KeyEsc})
	if cmd == nil {
		t.Fatal("expected close command")
	}
	if _, ok := cmd().(CloseConnectDialogMsg); !ok {
		t.Fatalf("expected CloseConnectDialogMsg, got %T", cmd())
	}
}
