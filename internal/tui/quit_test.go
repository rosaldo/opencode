package tui

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

func TestToggleQuitSecondPressQuits(t *testing.T) {
	a := appModel{}
	a, _ = a.toggleQuit()
	if !a.showQuit {
		t.Fatal("first press should open the quit dialog")
	}
	a, cmd := a.toggleQuit()
	if cmd == nil {
		t.Fatal("second press should quit")
	}
	if _, ok := cmd().(tea.QuitMsg); !ok {
		t.Fatalf("second press returned %T, want tea.QuitMsg", cmd())
	}
}
