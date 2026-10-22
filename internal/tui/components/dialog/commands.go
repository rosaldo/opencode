package dialog

import (
	"slices"
	"strconv"
	"strings"

	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	fuzzy "github.com/lithammer/fuzzysearch/fuzzy"
	utilComponents "github.com/opencode-ai/opencode/internal/tui/components/util"
	"github.com/opencode-ai/opencode/internal/tui/layout"
	"github.com/opencode-ai/opencode/internal/tui/styles"
	"github.com/opencode-ai/opencode/internal/tui/theme"
	"github.com/opencode-ai/opencode/internal/tui/util"
)

// Command represents a command that can be executed
type Command struct {
	ID          string
	Title       string
	Description string
	// Aliases are extra slash names accepted for this command (Spec 09h).
	Aliases []string
	Handler func(cmd Command) tea.Cmd
}

func (ci Command) Render(selected bool, width int) string {
	t := theme.CurrentTheme()
	baseStyle := styles.BaseStyle()

	descStyle := baseStyle.Width(width).Foreground(t.TextMuted())
	itemStyle := baseStyle.Width(width).
		Foreground(t.Text()).
		Background(t.Background())

	if selected {
		itemStyle = itemStyle.
			Background(t.Primary()).
			Foreground(t.Background()).
			Bold(true)
		descStyle = descStyle.
			Background(t.Primary()).
			Foreground(t.Background())
	}

	title := itemStyle.Padding(0, 1).Render(ci.Title)
	if ci.Description != "" {
		description := descStyle.Padding(0, 1).Render(ci.Description)
		return lipgloss.JoinVertical(lipgloss.Left, title, description)
	}
	return title
}

// CommandSelectedMsg is sent when a command is selected
type CommandSelectedMsg struct {
	Command Command
}

// CloseCommandDialogMsg is sent when the command dialog is closed
type CloseCommandDialogMsg struct{}

// CommandDialog interface for the command selection dialog
type CommandDialog interface {
	tea.Model
	layout.Bindings
	SetCommands(commands []Command)
}

// commandMode selects what the dialog does with printable keys: browsing the
// list or narrowing it with the search input (Spec 09d).
type commandMode int

const (
	commandModeList commandMode = iota
	commandModeSearch
)

type commandDialogCmp struct {
	listView utilComponents.SimpleList[Command]
	all      []Command
	mode     commandMode
	input    textinput.Model
	width    int
	height   int
}

type commandKeyMap struct {
	Enter  key.Binding
	Escape key.Binding
	Search key.Binding
}

var commandKeys = commandKeyMap{
	Enter: key.NewBinding(
		key.WithKeys("enter"),
		key.WithHelp("enter", "select command"),
	),
	Escape: key.NewBinding(
		key.WithKeys("esc"),
		key.WithHelp("esc", "close"),
	),
	Search: key.NewBinding(
		key.WithKeys("/"),
		key.WithHelp("/", "search"),
	),
}

func (c *commandDialogCmp) Init() tea.Cmd {
	return c.listView.Init()
}

func (c *commandDialogCmp) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmds []tea.Cmd
	switch msg := msg.(type) {
	case tea.KeyMsg:
		if c.mode == commandModeSearch {
			switch {
			case key.Matches(msg, commandKeys.Escape), key.Matches(msg, commandKeys.Enter):
				// Keep the narrowed list: leave search, then Enter picks an item.
				c.mode = commandModeList
				return c, nil
			}
			var cmd tea.Cmd
			c.input, cmd = c.input.Update(msg)
			c.applyFilter()
			return c, cmd
		}
		switch {
		case key.Matches(msg, commandKeys.Search):
			c.mode = commandModeSearch
			c.input.SetValue("")
			c.input.Focus()
			return c, textinput.Blink
		case key.Matches(msg, commandKeys.Enter):
			selectedItem, idx := c.listView.GetSelectedItem()
			if idx != -1 {
				return c, util.CmdHandler(CommandSelectedMsg{
					Command: selectedItem,
				})
			}
		case key.Matches(msg, commandKeys.Escape):
			return c, util.CmdHandler(CloseCommandDialogMsg{})
		}
	case utilComponents.ListActivatedMsg:
		// A click already moved the selection; activate it like Enter
		// (Spec 09n).
		selectedItem, idx := c.listView.GetSelectedItem()
		if idx != -1 {
			return c, util.CmdHandler(CommandSelectedMsg{
				Command: selectedItem,
			})
		}
		return c, nil
	case tea.WindowSizeMsg:
		c.width = msg.Width
		c.height = msg.Height
	}

	u, cmd := c.listView.Update(msg)
	c.listView = u.(utilComponents.SimpleList[Command])
	cmds = append(cmds, cmd)

	return c, tea.Batch(cmds...)
}

// applyFilter narrows the list with a case-insensitive fuzzy (subsequence)
// match over "title description", ordered by Levenshtein distance — so shorter,
// more exact titles surface first. An empty query restores the order the
// commands were registered with.
func (c *commandDialogCmp) applyFilter() {
	query := strings.TrimSpace(c.input.Value())
	if query == "" {
		c.listView.SetItems(c.all)
		return
	}
	targets := make([]string, len(c.all))
	for i := range c.all {
		targets[i] = c.all[i].Title + " " + c.all[i].Description
	}
	ranks := fuzzy.RankFindFold(query, targets)
	slices.SortStableFunc(ranks, func(a, b fuzzy.Rank) int {
		return a.Distance - b.Distance
	})
	items := make([]Command, 0, len(ranks))
	for _, r := range ranks {
		items = append(items, c.all[r.OriginalIndex])
	}
	c.listView.SetItems(items)
}

func (c *commandDialogCmp) View() string {
	t := theme.CurrentTheme()
	baseStyle := styles.BaseStyle()

	maxWidth := 40

	// Width follows every registered command so the frame does not resize while
	// the query narrows the list.
	commands := c.all

	for _, cmd := range commands {
		if len(cmd.Title) > maxWidth-4 {
			maxWidth = len(cmd.Title) + 4
		}
		if cmd.Description != "" {
			if len(cmd.Description) > maxWidth-4 {
				maxWidth = len(cmd.Description) + 4
			}
		}
	}

	c.listView.SetMaxWidth(maxWidth)

	title := baseStyle.
		Foreground(t.Primary()).
		Bold(true).
		Width(maxWidth).
		Padding(0, 1).
		Render("Commands")

	rows := []string{title, baseStyle.Width(maxWidth).Render("")}
	if c.mode == commandModeSearch {
		rows = append(rows,
			baseStyle.Width(maxWidth).Padding(0, 1).Render("Search: "+c.input.View()),
			baseStyle.Width(maxWidth).Render(""),
		)
	}

	count := len(c.listView.GetItems())
	hint := baseStyle.
		Width(maxWidth).
		Foreground(t.TextMuted()).
		Render("/ search · ↑↓ jk navigate")
	if c.mode == commandModeSearch {
		hint = baseStyle.
			Width(maxWidth).
			Foreground(t.TextMuted()).
			Render("esc to list · " + strconv.Itoa(count) + " match(es)")
	}

	body := lipgloss.JoinVertical(
		lipgloss.Left,
		append(rows,
			baseStyle.Width(maxWidth).Render(c.listView.View()),
			baseStyle.Width(maxWidth).Render(""),
			hint,
		)...,
	)

	return baseStyle.Padding(1, 2).
		Border(lipgloss.RoundedBorder()).
		BorderBackground(t.Background()).
		BorderForeground(t.TextMuted()).
		Width(lipgloss.Width(body) + 4).
		Render(body)
}

func (c *commandDialogCmp) BindingKeys() []key.Binding {
	return layout.KeyMapToSlice(commandKeys)
}

func (c *commandDialogCmp) SetCommands(commands []Command) {
	// A search left over from a previous opening must not shrink the freshly
	// loaded list (Spec 09d).
	c.mode = commandModeList
	c.input.SetValue("")
	c.input.Blur()
	c.all = commands
	c.listView.SetItems(commands)
}

// NewCommandDialogCmp creates a new command selection dialog
func NewCommandDialogCmp() CommandDialog {
	listView := utilComponents.NewSimpleList[Command](
		[]Command{},
		10,
		"No commands available",
		true,
	)
	input := textinput.New()
	input.Prompt = ""
	input.Placeholder = "search commands..."
	input.CharLimit = 64
	return &commandDialogCmp{
		listView: listView,
		input:    input,
	}
}
