package dialog

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

func TestPluginsDialogRenderAndClose(t *testing.T) {
	d := NewPluginsDialog()
	d.SetPlugins([]PluginInfo{
		{Name: "helper", Hooks: []string{"session.start", "tool.before_execute"}, Tools: []string{"echo"}},
	})
	d.Update(tea.WindowSizeMsg{Width: 120, Height: 40})

	view := d.View()
	for _, want := range []string{"Plugins", "helper", "session.start", "echo"} {
		if !strings.Contains(view, want) {
			t.Fatalf("view missing %q:\n%s", want, view)
		}
	}

	_, cmd := d.Update(tea.KeyMsg{Type: tea.KeyEsc})
	if cmd == nil {
		t.Fatal("expected close command")
	}
	if _, ok := cmd().(ClosePluginsDialogMsg); !ok {
		t.Fatalf("expected ClosePluginsDialogMsg, got %T", cmd())
	}
}

func TestPluginsDialogEmpty(t *testing.T) {
	d := NewPluginsDialog()
	d.Update(tea.WindowSizeMsg{Width: 100, Height: 30})
	if !strings.Contains(d.View(), "No plugins loaded") {
		t.Fatalf("expected empty state, got:\n%s", d.View())
	}
}
