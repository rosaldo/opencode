package tui

import (
	"testing"

	"github.com/opencode-ai/opencode/internal/tui/components/dialog"
)

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
}
