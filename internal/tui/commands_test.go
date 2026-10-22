package tui

import (
	"testing"

	"github.com/opencode-ai/opencode/internal/tui/components/dialog"
)

func TestRegisterCommandDefaultsGroupToCustom(t *testing.T) {
	a := &appModel{}
	a.RegisterCommand(dialog.Command{ID: "custom", Title: "Custom"})
	a.RegisterCommand(dialog.Command{ID: "grouped", Title: "Grouped", Group: "Session"})

	if a.commands[0].Group != "Custom" {
		t.Errorf("group = %q, want Custom", a.commands[0].Group)
	}
	if a.commands[1].Group != "Session" {
		t.Errorf("group = %q, want Session", a.commands[1].Group)
	}
}

func TestFilterCommands(t *testing.T) {
	cmds := []dialog.Command{
		{ID: "new", Title: "New Session"},
		{ID: "compact", Title: "Compact Session"},
		{ID: "connect", Title: "Connect Provider"},
	}

	t.Run("empty name returns all", func(t *testing.T) {
		got, ok := filterCommands(cmds, "")
		if !ok || len(got) != len(cmds) {
			t.Fatalf("got %d commands ok=%v, want %d", len(got), ok, len(cmds))
		}
	})

	t.Run("exact match", func(t *testing.T) {
		got, ok := filterCommands(cmds, "compact")
		if !ok || len(got) != 1 || got[0].ID != "compact" {
			t.Fatalf("got %+v ok=%v", got, ok)
		}
	})

	t.Run("case insensitive", func(t *testing.T) {
		got, ok := filterCommands(cmds, "New")
		if !ok || len(got) != 1 || got[0].ID != "new" {
			t.Fatalf("got %+v ok=%v", got, ok)
		}
	})

	t.Run("unknown name", func(t *testing.T) {
		if _, ok := filterCommands(cmds, "nope"); ok {
			t.Fatal("unknown name should return ok=false")
		}
	})

	t.Run("alias match", func(t *testing.T) {
		withAliases := []dialog.Command{
			{ID: "exit", Title: "Exit", Aliases: []string{"quit", "q"}},
			{ID: "new", Title: "New Session", Aliases: []string{"clear"}},
		}
		for _, name := range []string{"exit", "Quit", "q", "clear"} {
			got, ok := filterCommands(withAliases, name)
			if !ok || len(got) != 1 {
				t.Fatalf("%q: got %+v ok=%v", name, got, ok)
			}
		}
		if _, ok := filterCommands(withAliases, "Q"); !ok {
			t.Error("alias lookup must be case insensitive")
		}
	})
}
