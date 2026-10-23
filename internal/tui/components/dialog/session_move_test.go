package dialog

import (
	"os"
	"path/filepath"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

func TestResolveDirectoryInput(t *testing.T) {
	current := t.TempDir()

	cases := []struct {
		name  string
		input string
		want  string
	}{
		{"empty resets", "", ""},
		{"absolute stays", "/tmp", "/tmp"},
		{"relative resolves against the session", "sub/dir", filepath.Join(current, "sub/dir")},
		{"trims spaces", "  sub  ", filepath.Join(current, "sub")},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := ResolveDirectoryInput(tc.input, current); got != tc.want {
				t.Errorf("ResolveDirectoryInput(%q) = %q, want %q", tc.input, got, tc.want)
			}
		})
	}

	home, err := os.UserHomeDir()
	if err != nil {
		t.Skip("no home directory")
	}
	if got := ResolveDirectoryInput("~", current); got != home {
		t.Errorf("~ = %q, want %q", got, home)
	}
	if got := ResolveDirectoryInput("~/projects", current); got != filepath.Join(home, "projects") {
		t.Errorf("~/projects = %q", got)
	}
}

func TestSessionMoveDialogPrefillAndMessages(t *testing.T) {
	d := NewSessionMoveDialogCmp()
	d.SetDirectory("/work")

	m, cmd := d.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if cmd == nil {
		t.Fatal("enter should emit a command")
	}
	msg := cmd()
	move, ok := msg.(SessionMoveMsg)
	if !ok {
		t.Fatalf("enter emitted %T, want SessionMoveMsg", msg)
	}
	if move.Directory != "/work" {
		t.Errorf("directory = %q, want /work", move.Directory)
	}

	_, cmd = m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	if cmd == nil {
		t.Fatal("esc should emit a command")
	}
	if _, ok := cmd().(CloseSessionMoveDialogMsg); !ok {
		t.Errorf("esc emitted %T, want CloseSessionMoveDialogMsg", cmd())
	}
}
