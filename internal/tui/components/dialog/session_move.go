package dialog

import (
	"os"
	"path/filepath"
	"strings"

	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/opencode-ai/opencode/internal/tui/layout"
	"github.com/opencode-ai/opencode/internal/tui/styles"
	"github.com/opencode-ai/opencode/internal/tui/theme"
)

// SessionMoveMsg asks the app to move the current session to Directory
// (Spec 09v). An empty Directory resets it to the launch directory.
type SessionMoveMsg struct {
	Directory string
}

// CloseSessionMoveDialogMsg is sent when the dialog is dismissed (Spec 09v).
type CloseSessionMoveDialogMsg struct{}

// SessionMoveDialog is the working-directory prompt for session_move.
type SessionMoveDialog interface {
	tea.Model
	layout.Bindings
	// SetDirectory prefills the input with the session's current directory.
	SetDirectory(dir string)
}

type sessionMoveDialogCmp struct {
	width, height int
	input         textinput.Model
}

// NewSessionMoveDialogCmp creates the session_move dialog.
func NewSessionMoveDialogCmp() SessionMoveDialog {
	ti := textinput.New()
	ti.Placeholder = "/path/to/project"
	ti.CharLimit = 4096
	return &sessionMoveDialogCmp{input: ti}
}

func (c *sessionMoveDialogCmp) Init() tea.Cmd {
	return textinput.Blink
}

func (c *sessionMoveDialogCmp) BindingKeys() []key.Binding {
	return layout.KeyMapToSlice(struct {
		Move   key.Binding
		Cancel key.Binding
	}{
		Move:   key.NewBinding(key.WithKeys("enter"), key.WithHelp("enter", "move session")),
		Cancel: key.NewBinding(key.WithKeys("esc"), key.WithHelp("esc", "cancel")),
	})
}

func (c *sessionMoveDialogCmp) SetDirectory(dir string) {
	c.input.SetValue(dir)
	c.input.CursorEnd()
	c.input.Focus()
}

func (c *sessionMoveDialogCmp) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		c.width = msg.Width
		c.height = msg.Height
		return c, nil
	case tea.KeyMsg:
		switch msg.String() {
		case "esc":
			c.input.Blur()
			return c, func() tea.Msg { return CloseSessionMoveDialogMsg{} }
		case "enter":
			value := strings.TrimSpace(c.input.Value())
			c.input.Blur()
			return c, func() tea.Msg { return SessionMoveMsg{Directory: value} }
		}
	}
	var cmd tea.Cmd
	c.input, cmd = c.input.Update(msg)
	return c, cmd
}

func (c *sessionMoveDialogCmp) View() string {
	t := theme.CurrentTheme()
	baseStyle := styles.BaseStyle()

	width := 60
	if c.width > 0 && c.width < width+8 {
		width = max(24, c.width-8)
	}

	title := baseStyle.
		Foreground(t.Primary()).
		Bold(true).
		Width(width).
		Padding(0, 1).
		Render("Move session")

	hint := baseStyle.
		Foreground(t.TextMuted()).
		Width(width).
		Padding(0, 1).
		Render("Directory for this session's tools. Empty resets to the launch directory.")

	help := baseStyle.
		Foreground(t.TextMuted()).
		Width(width).
		Padding(0, 1).
		Render("enter move · esc cancel")

	return baseStyle.
		Border(lipgloss.RoundedBorder()).
		BorderForeground(t.BorderFocused()).
		Padding(0, 1).
		Render(lipgloss.JoinVertical(
			lipgloss.Left,
			title,
			c.input.View(),
			hint,
			help,
		))
}

// ResolveDirectoryInput turns the dialog input into an absolute path: "~" and
// "~/x" expand to the home directory and relative paths resolve against the
// session's current directory (Spec 09v). Empty input stays empty (reset).
func ResolveDirectoryInput(input, currentDir string) string {
	input = strings.TrimSpace(input)
	if input == "" {
		return ""
	}
	if input == "~" || strings.HasPrefix(input, "~/") {
		if home, err := os.UserHomeDir(); err == nil {
			input = filepath.Join(home, strings.TrimPrefix(input, "~"))
		}
	}
	if !filepath.IsAbs(input) {
		base := currentDir
		if base == "" {
			base, _ = os.Getwd()
		}
		input = filepath.Join(base, input)
	}
	if abs, err := filepath.Abs(input); err == nil {
		return abs
	}
	return input
}
