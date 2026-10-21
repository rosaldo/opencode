package dialog

import (
	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/opencode-ai/opencode/internal/llm/models"
	"github.com/opencode-ai/opencode/internal/tui/layout"
	"github.com/opencode-ai/opencode/internal/tui/styles"
	"github.com/opencode-ai/opencode/internal/tui/theme"
	"github.com/opencode-ai/opencode/internal/tui/util"
)

// SubmitConnectMsg is emitted when the user submits an API key via /connect.
type SubmitConnectMsg struct {
	Provider models.ModelProvider
	APIKey   string
}

// ShowConnectDialogMsg asks the app to open the connect dialog for a provider.
type ShowConnectDialogMsg struct {
	Provider models.ModelProvider
}

// CloseConnectDialogMsg is emitted when the connect dialog is dismissed.
type CloseConnectDialogMsg struct{}

// ConnectDialog lets the user paste an API key for a provider (Spec 12d),
// mirroring the TS /connect flow.
type ConnectDialog interface {
	tea.Model
	layout.Bindings
	SetProvider(provider models.ModelProvider)
}

type connectCmp struct {
	width    int
	height   int
	provider models.ModelProvider
	input    textinput.Model
}

var connectKeys = struct {
	Enter  key.Binding
	Escape key.Binding
}{
	Enter:  key.NewBinding(key.WithKeys("enter"), key.WithHelp("enter", "save")),
	Escape: key.NewBinding(key.WithKeys("esc", "q"), key.WithHelp("esc", "cancel")),
}

// NewConnectDialog creates the connect dialog, defaulting to OpenCode Zen.
func NewConnectDialog() ConnectDialog {
	ti := textinput.New()
	ti.Placeholder = "paste your API key"
	ti.EchoMode = textinput.EchoPassword
	ti.EchoCharacter = '•'
	ti.Focus()
	return &connectCmp{
		provider: models.ProviderOpencode,
		input:    ti,
	}
}

func (c *connectCmp) Init() tea.Cmd { return textinput.Blink }

func (c *connectCmp) SetProvider(provider models.ModelProvider) {
	if provider != "" {
		c.provider = provider
	}
	c.input.Focus()
}

func (c *connectCmp) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		c.width = msg.Width
		c.height = msg.Height
	case tea.KeyMsg:
		switch {
		case key.Matches(msg, connectKeys.Escape):
			return c, util.CmdHandler(CloseConnectDialogMsg{})
		case key.Matches(msg, connectKeys.Enter):
			value := c.input.Value()
			if value == "" {
				return c, nil
			}
			return c, util.CmdHandler(SubmitConnectMsg{Provider: c.provider, APIKey: value})
		}
	}
	var cmd tea.Cmd
	c.input, cmd = c.input.Update(msg)
	return c, cmd
}

func (c *connectCmp) View() string {
	t := theme.CurrentTheme()
	baseStyle := styles.BaseStyle()

	maxWidth := 60
	maxWidth = min(maxWidth, c.width-10)

	title := baseStyle.
		Foreground(t.Primary()).
		Bold(true).
		Width(maxWidth).
		Padding(0, 1).
		Render("Connect: " + providerLabel(c.provider))

	explanation := baseStyle.
		Foreground(t.TextMuted()).
		Width(maxWidth).
		Padding(0, 1).
		Render("Sign in at https://opencode.ai/console, copy your API key and paste it below. " +
			"The key is stored in your opencode config and can also be provided via OPENCODE_API_KEY.")

	label := baseStyle.
		Foreground(t.Text()).
		Width(maxWidth).
		Padding(1, 1).
		Render("API key")

	input := baseStyle.
		Width(maxWidth-2).
		Padding(0, 1).
		Render(c.input.View())

	content := lipgloss.JoinVertical(
		lipgloss.Left,
		title,
		baseStyle.Width(maxWidth).Render(""),
		explanation,
		label,
		input,
		baseStyle.Width(maxWidth).Render(""),
	)

	return baseStyle.Padding(1, 2).
		Border(lipgloss.RoundedBorder()).
		BorderBackground(t.Background()).
		BorderForeground(t.TextMuted()).
		Width(lipgloss.Width(content) + 4).
		Render(content)
}

func (c *connectCmp) BindingKeys() []key.Binding {
	return layout.KeyMapToSlice(connectKeys)
}

func providerLabel(provider models.ModelProvider) string {
	if provider == models.ProviderOpencode {
		return "OpenCode Zen"
	}
	return string(provider)
}
