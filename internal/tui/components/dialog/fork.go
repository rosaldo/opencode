package dialog

import (
	"fmt"
	"slices"
	"strings"

	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	fuzzy "github.com/lithammer/fuzzysearch/fuzzy"
	"github.com/opencode-ai/opencode/internal/message"
	utilComponents "github.com/opencode-ai/opencode/internal/tui/components/util"
	"github.com/opencode-ai/opencode/internal/tui/layout"
	"github.com/opencode-ai/opencode/internal/tui/styles"
	"github.com/opencode-ai/opencode/internal/tui/theme"
	"github.com/opencode-ai/opencode/internal/tui/util"
)

// ForkItems builds the fork dialog rows: "Full session" first, then the user
// messages newest first — the same projection as the TS fork dialog (Spec 09p).
func ForkItems(msgs []message.Message) []TimelineItem {
	items := append([]TimelineItem{{
		Title: "Full session",
	}}, TimelineItems(msgs)...)
	return items
}

// ForkMsg asks the app to fork the current session up to (but excluding) the
// selected message; an empty MessageID forks the full session (Spec 09p).
type ForkMsg struct {
	MessageID string
}

// CloseForkDialogMsg is sent when the fork dialog is closed without forking
// (Spec 09p).
type CloseForkDialogMsg struct{}

// ForkDialog interface for the fork picker (Spec 09p).
type ForkDialog interface {
	tea.Model
	layout.Bindings
	SetItems(items []TimelineItem)
	SetPending(pending bool)
	// SelectMessage preselects the row for messageID (Spec 09u); an unknown
	// id leaves the selection unchanged.
	SelectMessage(messageID string)
}

type forkDialogCmp struct {
	listView utilComponents.SimpleList[TimelineItem]
	all      []TimelineItem
	mode     commandMode
	input    textinput.Model
	pending  bool
	width    int
	height   int
}

type forkKeyMap struct {
	Enter  key.Binding
	Escape key.Binding
	Search key.Binding
}

var forkKeys = forkKeyMap{
	Enter: key.NewBinding(
		key.WithKeys("enter"),
		key.WithHelp("enter", "fork session"),
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

func (c *forkDialogCmp) Init() tea.Cmd {
	return c.listView.Init()
}

func (c *forkDialogCmp) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmds []tea.Cmd
	switch msg := msg.(type) {
	case tea.KeyMsg:
		if c.pending {
			// The fork request is in flight: only a size change may arrive
			// until the app closes the dialog (Spec 09p).
			return c, nil
		}
		if c.mode == commandModeSearch {
			switch {
			case key.Matches(msg, forkKeys.Escape), key.Matches(msg, forkKeys.Enter):
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
		case key.Matches(msg, forkKeys.Search):
			c.mode = commandModeSearch
			c.input.SetValue("")
			c.input.Focus()
			return c, textinput.Blink
		case key.Matches(msg, forkKeys.Enter):
			return c, c.activate()
		case key.Matches(msg, forkKeys.Escape):
			return c, util.CmdHandler(CloseForkDialogMsg{})
		}
	case utilComponents.ListActivatedMsg:
		if c.pending {
			return c, nil
		}
		// A click already moved the selection; activate it like Enter
		// (Spec 09n).
		return c, c.activate()
	case tea.WindowSizeMsg:
		c.width = msg.Width
		c.height = msg.Height
	}

	u, cmd := c.listView.Update(msg)
	c.listView = u.(utilComponents.SimpleList[TimelineItem])
	cmds = append(cmds, cmd)

	return c, tea.Batch(cmds...)
}

// activate forks up to the selected row: the "Full session" row carries an
// empty MessageID (Spec 09p).
func (c *forkDialogCmp) activate() tea.Cmd {
	selectedItem, idx := c.listView.GetSelectedItem()
	if idx == -1 {
		return nil
	}
	return util.CmdHandler(ForkMsg{MessageID: selectedItem.MessageID})
}

// applyFilter narrows the list with a case-insensitive fuzzy match over the
// message text, ordered by Levenshtein distance (Spec 09p).
func (c *forkDialogCmp) applyFilter() {
	query := strings.TrimSpace(c.input.Value())
	if query == "" {
		c.listView.SetItems(c.all)
		return
	}
	targets := make([]string, len(c.all))
	for i := range c.all {
		targets[i] = c.all[i].Title
	}
	ranks := fuzzy.RankFindFold(query, targets)
	slices.SortStableFunc(ranks, func(a, b fuzzy.Rank) int {
		return a.Distance - b.Distance
	})
	items := make([]TimelineItem, 0, len(ranks))
	for _, r := range ranks {
		items = append(items, c.all[r.OriginalIndex])
	}
	c.listView.SetItems(items)
}

func (c *forkDialogCmp) View() string {
	t := theme.CurrentTheme()
	baseStyle := styles.BaseStyle()

	maxWidth := 40

	// Width follows every loaded message so the frame does not resize while
	// the query narrows the list.
	for _, item := range c.all {
		if w := lipgloss.Width(item.Title) + 12; w > maxWidth {
			maxWidth = min(w, 80)
		}
	}

	c.listView.SetMaxWidth(maxWidth)

	title := baseStyle.
		Foreground(t.Primary()).
		Bold(true).
		Width(maxWidth).
		Padding(0, 1).
		Render("Fork session")

	if c.pending {
		body := lipgloss.JoinVertical(
			lipgloss.Left,
			title,
			baseStyle.Width(maxWidth).Render(""),
			baseStyle.Width(maxWidth).Foreground(t.TextMuted()).Render("Forking session…"),
		)
		return baseStyle.Padding(1, 2).
			Border(lipgloss.RoundedBorder()).
			BorderBackground(t.Background()).
			BorderForeground(t.TextMuted()).
			Width(lipgloss.Width(body) + 4).
			Render(body)
	}

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
		Render("enter fork · / search · ↑↓ jk navigate")
	if c.mode == commandModeSearch {
		hint = baseStyle.
			Width(maxWidth).
			Foreground(t.TextMuted()).
			Render(fmt.Sprintf("esc to list · %d match(es)", count))
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

func (c *forkDialogCmp) BindingKeys() []key.Binding {
	return layout.KeyMapToSlice(forkKeys)
}

func (c *forkDialogCmp) SetItems(items []TimelineItem) {
	// A search left over from a previous opening must not shrink the freshly
	// loaded list (Spec 09p).
	c.mode = commandModeList
	c.input.SetValue("")
	c.input.Blur()
	c.pending = false
	c.all = items
	c.listView.SetItems(items)
}

func (c *forkDialogCmp) SetPending(pending bool) {
	c.pending = pending
}

func (c *forkDialogCmp) SelectMessage(messageID string) {
	idx := slices.IndexFunc(c.all, func(item TimelineItem) bool {
		return item.MessageID == messageID
	})
	if idx >= 0 {
		c.listView.SelectIndex(idx)
	}
}

// NewForkDialogCmp creates the fork picker (Spec 09p).
func NewForkDialogCmp() ForkDialog {
	listView := utilComponents.NewSimpleList[TimelineItem](
		[]TimelineItem{},
		10,
		"No messages to fork",
		true,
	)
	input := textinput.New()
	input.Prompt = ""
	input.Placeholder = "search messages..."
	input.CharLimit = 64
	return &forkDialogCmp{
		listView: listView,
		input:    input,
	}
}
