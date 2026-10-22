package dialog

import (
	"github.com/charmbracelet/bubbles/key"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/opencode-ai/opencode/internal/message"
	utilComponents "github.com/opencode-ai/opencode/internal/tui/components/util"
	"github.com/opencode-ai/opencode/internal/tui/layout"
	"github.com/opencode-ai/opencode/internal/tui/styles"
	"github.com/opencode-ai/opencode/internal/tui/theme"
	"github.com/opencode-ai/opencode/internal/tui/util"
)

// MessageAction is the action chosen in the message actions dialog (Spec 09u).
type MessageAction string

const (
	// MessageActionJump views the message in the session (TS "Jump to").
	MessageActionJump MessageAction = "jump"
	// MessageActionRevert undoes the message and everything after it (TS
	// "Revert"; the file rollback of the TS implementation is deferred,
	// Spec 09k/09u).
	MessageActionRevert MessageAction = "revert"
	// MessageActionCopy puts the message text on the clipboard (TS "Copy").
	MessageActionCopy MessageAction = "copy"
	// MessageActionFork opens the fork picker from the message (TS "Fork").
	MessageActionFork MessageAction = "fork"
)

// OpenMessageActionsMsg asks the app to replace the timeline picker with the
// actions dialog for the chosen message (Spec 09u).
type OpenMessageActionsMsg struct {
	MessageID string
}

// MessageActionMsg is emitted when an action is chosen for the message
// (Spec 09u).
type MessageActionMsg struct {
	MessageID string
	Action    MessageAction
}

// CloseMessageActionsMsg is sent when the dialog is closed without choosing
// an action (Spec 09u).
type CloseMessageActionsMsg struct{}

// MessageActionsDialog interface for the message actions dialog (Spec 09u).
type MessageActionsDialog interface {
	tea.Model
	layout.Bindings
	SetMessage(msg *message.Message)
}

// messageActionItem is one row: the action title with the TS description as a
// muted footer (Spec 09u).
type messageActionItem struct {
	Action      MessageAction
	Title       string
	Description string
	Enabled     bool
}

func (i messageActionItem) Render(selected bool, width int) string {
	t := theme.CurrentTheme()
	baseStyle := styles.BaseStyle()

	descStyle := baseStyle.Width(width).Foreground(t.TextMuted())
	itemStyle := baseStyle.Width(width).
		Foreground(t.Text()).
		Background(t.Background())

	switch {
	case !i.Enabled:
		itemStyle = itemStyle.Foreground(t.TextMuted())
	case selected:
		itemStyle = itemStyle.
			Background(t.Primary()).
			Foreground(t.Background()).
			Bold(true)
		descStyle = descStyle.
			Background(t.Primary()).
			Foreground(t.Background())
	}

	return lipgloss.JoinVertical(
		lipgloss.Left,
		itemStyle.Padding(0, 1).Render(i.Title),
		descStyle.Padding(0, 1).Render(i.Description),
	)
}

// messageActions returns the rows for a message: Jump to / Revert / Copy
// always, Fork only for user messages — the same option set as the TS
// DialogMessage (Spec 09u).
func messageActions(msg *message.Message) []messageActionItem {
	items := []messageActionItem{
		{
			Action:      MessageActionJump,
			Title:       "Jump to",
			Description: "view message in session",
			Enabled:     true,
		},
		{
			Action:      MessageActionRevert,
			Title:       "Revert",
			Description: "undo messages from this point",
			Enabled:     true,
		},
		{
			Action:      MessageActionCopy,
			Title:       "Copy",
			Description: "message text to clipboard",
			Enabled:     msg != nil,
		},
	}
	if msg != nil && msg.Role == message.User {
		items = append(items, messageActionItem{
			Action:      MessageActionFork,
			Title:       "Fork",
			Description: "create a new session",
			Enabled:     true,
		})
	}
	return items
}

type messageActionsDialogCmp struct {
	listView utilComponents.SimpleList[messageActionItem]
	all      []messageActionItem
	message  *message.Message
	width    int
	height   int
}

type messageActionsKeyMap struct {
	Enter  key.Binding
	Escape key.Binding
}

var messageActionsKeys = messageActionsKeyMap{
	Enter: key.NewBinding(
		key.WithKeys("enter"),
		key.WithHelp("enter", "run action"),
	),
	Escape: key.NewBinding(
		key.WithKeys("esc"),
		key.WithHelp("esc", "close"),
	),
}

func (c *messageActionsDialogCmp) Init() tea.Cmd {
	return c.listView.Init()
}

func (c *messageActionsDialogCmp) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmds []tea.Cmd
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch {
		case key.Matches(msg, messageActionsKeys.Enter):
			selected, idx := c.listView.GetSelectedItem()
			if idx != -1 && selected.Enabled && c.message != nil {
				return c, util.CmdHandler(MessageActionMsg{
					MessageID: c.message.ID,
					Action:    selected.Action,
				})
			}
			return c, nil
		case key.Matches(msg, messageActionsKeys.Escape):
			return c, util.CmdHandler(CloseMessageActionsMsg{})
		}
	case utilComponents.ListActivatedMsg:
		// A click already moved the selection; activate it like Enter
		// (Spec 09n/09u).
		selected, idx := c.listView.GetSelectedItem()
		if idx != -1 && selected.Enabled && c.message != nil {
			return c, util.CmdHandler(MessageActionMsg{
				MessageID: c.message.ID,
				Action:    selected.Action,
			})
		}
		return c, nil
	case tea.WindowSizeMsg:
		c.width = msg.Width
		c.height = msg.Height
	}

	u, cmd := c.listView.Update(msg)
	c.listView = u.(utilComponents.SimpleList[messageActionItem])
	cmds = append(cmds, cmd)

	return c, tea.Batch(cmds...)
}

func (c *messageActionsDialogCmp) View() string {
	t := theme.CurrentTheme()
	baseStyle := styles.BaseStyle()

	maxWidth := 44
	for _, item := range c.all {
		if w := lipgloss.Width(item.Title) + 12; w > maxWidth {
			maxWidth = min(w, 80)
		}
		if w := lipgloss.Width(item.Description) + 4; w > maxWidth {
			maxWidth = min(w, 80)
		}
	}

	c.listView.SetMaxWidth(maxWidth)

	title := baseStyle.
		Foreground(t.Primary()).
		Bold(true).
		Width(maxWidth).
		Padding(0, 1).
		Render("Message Actions")

	hint := baseStyle.
		Width(maxWidth).
		Foreground(t.TextMuted()).
		Render("enter run · esc close · ↑↓ jk navigate")

	body := lipgloss.JoinVertical(
		lipgloss.Left,
		title,
		baseStyle.Width(maxWidth).Render(""),
		baseStyle.Width(maxWidth).Render(c.listView.View()),
		baseStyle.Width(maxWidth).Render(""),
		hint,
	)

	return baseStyle.Padding(1, 2).
		Border(lipgloss.RoundedBorder()).
		BorderBackground(t.Background()).
		BorderForeground(t.TextMuted()).
		Width(lipgloss.Width(body) + 4).
		Render(body)
}

func (c *messageActionsDialogCmp) BindingKeys() []key.Binding {
	return layout.KeyMapToSlice(messageActionsKeys)
}

func (c *messageActionsDialogCmp) SetMessage(msg *message.Message) {
	// Opening must always start from the full option set with "Jump to"
	// selected, regardless of a previous opening (Spec 09u).
	c.message = msg
	c.all = messageActions(msg)
	c.listView.SetItems(c.all)
}

// NewMessageActionsDialogCmp creates the message actions dialog (Spec 09u).
func NewMessageActionsDialogCmp() MessageActionsDialog {
	listView := utilComponents.NewSimpleList[messageActionItem](
		[]messageActionItem{},
		10,
		"No actions",
		true,
	)
	return &messageActionsDialogCmp{
		listView: listView,
	}
}
