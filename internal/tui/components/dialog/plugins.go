package dialog

import (
	"sort"
	"strings"

	"github.com/charmbracelet/bubbles/key"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/opencode-ai/opencode/internal/tui/layout"
	"github.com/opencode-ai/opencode/internal/tui/styles"
	"github.com/opencode-ai/opencode/internal/tui/theme"
	"github.com/opencode-ai/opencode/internal/tui/util"
)

// ClosePluginsDialogMsg is sent when the plugins dialog is closed.
type ClosePluginsDialogMsg struct{}

// ShowPluginsDialogMsg asks the app to open the plugins dialog.
type ShowPluginsDialogMsg struct {
	Plugins []PluginInfo
}

// PluginInfo is one row in the plugins dialog.
type PluginInfo struct {
	Name  string
	Hooks []string
	Tools []string
}

// PluginsDialog lists the loaded plugins and what they contribute (Spec 10c).
type PluginsDialog interface {
	tea.Model
	layout.Bindings
	SetPlugins(plugins []PluginInfo)
}

type pluginsCmp struct {
	plugins []PluginInfo
	width   int
	height  int
}

var pluginsKeys = struct {
	Escape key.Binding
}{
	Escape: key.NewBinding(
		key.WithKeys("esc", "q"),
		key.WithHelp("esc", "close"),
	),
}

func (p *pluginsCmp) Init() tea.Cmd { return nil }

func (p *pluginsCmp) SetPlugins(plugins []PluginInfo) {
	p.plugins = plugins
}

func (p *pluginsCmp) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		p.width = msg.Width
		p.height = msg.Height
	case tea.KeyMsg:
		if key.Matches(msg, pluginsKeys.Escape) {
			return p, util.CmdHandler(ClosePluginsDialogMsg{})
		}
	}
	return p, nil
}

func (p *pluginsCmp) View() string {
	t := theme.CurrentTheme()
	baseStyle := styles.BaseStyle()

	maxWidth := 40
	for _, pl := range p.plugins {
		if len(pl.Name)+4 > maxWidth {
			maxWidth = len(pl.Name) + 4
		}
	}
	maxWidth = max(30, min(maxWidth, p.width-15))

	rows := []string{
		baseStyle.Foreground(t.Primary()).Bold(true).Width(maxWidth).Padding(0, 1).Render("Plugins"),
		baseStyle.Width(maxWidth).Render(""),
	}

	if len(p.plugins) == 0 {
		rows = append(rows, baseStyle.Width(maxWidth).Padding(0, 1).Foreground(t.TextMuted()).Render("No plugins loaded"))
	} else {
		for _, pl := range p.plugins {
			rows = append(rows, baseStyle.Width(maxWidth).Padding(0, 1).Bold(true).Render(pl.Name))
			if s := summarize(pl.Hooks); s != "" {
				rows = append(rows, baseStyle.Width(maxWidth).Padding(0, 2).Foreground(t.TextMuted()).Render("hooks: "+s))
			}
			if s := summarize(pl.Tools); s != "" {
				rows = append(rows, baseStyle.Width(maxWidth).Padding(0, 2).Foreground(t.TextMuted()).Render("tools: "+s))
			}
		}
	}

	rows = append(rows, baseStyle.Width(maxWidth).Render(""))
	content := lipgloss.JoinVertical(lipgloss.Left, rows...)

	return baseStyle.Padding(1, 2).
		Border(lipgloss.RoundedBorder()).
		BorderBackground(t.Background()).
		BorderForeground(t.TextMuted()).
		Width(lipgloss.Width(content) + 4).
		Render(content)
}

func (p *pluginsCmp) BindingKeys() []key.Binding {
	return layout.KeyMapToSlice(pluginsKeys)
}

func summarize(items []string) string {
	if len(items) == 0 {
		return ""
	}
	sorted := append([]string(nil), items...)
	sort.Strings(sorted)
	return strings.Join(sorted, ", ")
}

// NewPluginsDialog creates a new plugins dialog.
func NewPluginsDialog() PluginsDialog {
	return &pluginsCmp{}
}
