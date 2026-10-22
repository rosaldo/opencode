package dialog

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/opencode-ai/opencode/internal/agentdef"
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

func newAgentClickFixture(t *testing.T) *agentDialogCmp {
	t.Helper()
	cmp, ok := NewAgentDialogCmp(nil).(*agentDialogCmp)
	if !ok {
		t.Fatal("NewAgentDialogCmp must return *agentDialogCmp")
	}
	cmp.agents = []*agentdef.Definition{
		{Name: "coder", Description: "general purpose coding agent"},
		{Name: "security-audit", Description: "reviews code for vulnerabilities"},
	}
	cmp.applyFilter()
	return cmp
}

func TestAgentDialogClickRow(t *testing.T) {
	cmp := newAgentClickFixture(t)

	_, cmd := cmp.selectAgent(1)
	if cmd == nil {
		t.Fatal("clicking a valid row must select the agent")
	}
	msg, ok := cmd().(AgentSelectedMsg)
	if !ok {
		t.Fatal("click must emit AgentSelectedMsg")
	}
	if msg.Name != "security-audit" {
		t.Errorf("agent = %q, want security-audit", msg.Name)
	}
	if cmp.selectedIdx != 1 {
		t.Errorf("selectedIdx = %d, want 1", cmp.selectedIdx)
	}

	if _, cmd := cmp.selectAgent(99); cmd != nil {
		t.Error("out-of-range clicks must be ignored")
	}
	if _, cmd := cmp.selectAgent(-1); cmd != nil {
		t.Error("negative clicks must be ignored")
	}
	if cmp.selectedIdx != 1 {
		t.Errorf("selectedIdx = %d, want selection unchanged", cmp.selectedIdx)
	}
}

func TestAgentDialogClickOnEmptyListIsIgnored(t *testing.T) {
	cmp := newAgentClickFixture(t)
	cmp.agents = nil
	cmp.applyFilter()

	if _, cmd := cmp.selectAgent(0); cmd != nil {
		t.Error("clicking an empty list must not activate anything")
	}
}

func TestAgentDialogMouseUpdateSwallowsClicks(t *testing.T) {
	cmp := newAgentClickFixture(t)

	// No zone has been rendered yet, so a click cannot resolve a row: it must
	// be swallowed (no command, selection unchanged) instead of falling
	// through (Spec 09n).
	_, cmd := cmp.Update(tea.MouseMsg{
		Action: tea.MouseActionPress,
		Button: tea.MouseButtonLeft,
	})
	if cmd != nil {
		t.Error("click outside a row must be swallowed")
	}
	if cmp.selectedIdx != 0 {
		t.Errorf("selectedIdx = %d, want 0", cmp.selectedIdx)
	}

	// Non-press mouse events never reach the list from the app, but must not
	// select or close anything either.
	_, cmd = cmp.Update(tea.MouseMsg{Action: tea.MouseActionMotion})
	if cmd != nil {
		t.Error("mouse motion must be swallowed")
	}
}

func TestPluginsDialogConsumesMouse(t *testing.T) {
	cmp := NewPluginsDialog()
	cmp.SetPlugins([]PluginInfo{{Name: "example", Hooks: []string{"tool.execute.before"}}})

	for _, action := range []tea.MouseAction{
		tea.MouseActionPress,
		tea.MouseActionRelease,
		tea.MouseActionMotion,
	} {
		_, cmd := cmp.Update(tea.MouseMsg{
			Action: action,
			Button: tea.MouseButtonLeft,
		})
		if cmd != nil {
			t.Errorf("action %v: informational dialog must consume the click (no fall-through)", action)
		}
	}

	if _, cmd := cmp.Update(tea.KeyMsg{Type: tea.KeyEsc}); cmd == nil {
		t.Error("escape must still close the plugins dialog")
	}
}
