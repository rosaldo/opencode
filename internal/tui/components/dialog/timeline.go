package dialog

import (
	"fmt"
	"slices"
	"strings"
	"time"

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

// TimelineItem is one row of the timeline dialog: a user message of the
// current session (Spec 09o).
type TimelineItem struct {
	MessageID string
	Title     string
	CreatedAt time.Time
}

// Render shows the message text with the creation time as a muted footer
// (Spec 09o).
func (i TimelineItem) Render(selected bool, width int) string {
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

	title := i.Title
	if lipgloss.Width(title) > width-4 {
		title = strings.TrimSpace(title[:max(0, width-7)]) + "..."
	}
	return lipgloss.JoinVertical(
		lipgloss.Left,
		itemStyle.Padding(0, 1).Render(title),
		descStyle.Padding(0, 1).Render(i.CreatedAt.Format("15:04:05")),
	)
}

// TimelineItems builds the timeline rows from the session messages: user
// messages only, newest first — the same projection as the TS timeline
// (Spec 09o).
func TimelineItems(msgs []message.Message) []TimelineItem {
	items := make([]TimelineItem, 0, len(msgs))
	for _, msg := range msgs {
		if msg.Role != message.User {
			continue
		}
		text := timelineText(msg)
		if text == "" {
			continue
		}
		items = append(items, TimelineItem{
			MessageID: msg.ID,
			Title:     text,
			CreatedAt: time.Unix(msg.CreatedAt, 0),
		})
	}
	slices.Reverse(items)
	return items
}

// timelineText flattens the text parts of a user message into a single line.
func timelineText(msg message.Message) string {
	var parts []string
	for _, part := range msg.Parts {
		if text, ok := part.(message.TextContent); ok {
			if trimmed := strings.TrimSpace(text.Text); trimmed != "" {
				parts = append(parts, strings.Join(strings.Fields(trimmed), " "))
			}
		}
	}
	return strings.Join(parts, " ")
}

// TimelineJumpMsg asks the app to close the dialog and scroll the transcript to
// the selected message (Spec 09o).
type TimelineJumpMsg struct {
	MessageID string
}

// TimelinePreviewMsg scrolls the transcript behind the open dialog while the
// selection moves (Spec 09o).
type TimelinePreviewMsg struct {
	MessageID string
}

// CloseTimelineDialogMsg is sent when the timeline dialog is closed without
// jumping (Spec 09o).
type CloseTimelineDialogMsg struct{}

// TimelineDialog interface for the timeline picker (Spec 09o).
type TimelineDialog interface {
	tea.Model
	layout.Bindings
	SetItems(items []TimelineItem)
}

type timelineDialogCmp struct {
	listView utilComponents.SimpleList[TimelineItem]
	all      []TimelineItem
	mode     commandMode
	input    textinput.Model
	width    int
	height   int
}

type timelineKeyMap struct {
	Enter  key.Binding
	Escape key.Binding
	Search key.Binding
}

var timelineKeys = timelineKeyMap{
	Enter: key.NewBinding(
		key.WithKeys("enter"),
		key.WithHelp("enter", "jump to message"),
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

func (c *timelineDialogCmp) Init() tea.Cmd {
	return c.listView.Init()
}

func (c *timelineDialogCmp) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmds []tea.Cmd
	switch msg := msg.(type) {
	case tea.KeyMsg:
		if c.mode == commandModeSearch {
			switch {
			case key.Matches(msg, timelineKeys.Escape), key.Matches(msg, timelineKeys.Enter):
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
		case key.Matches(msg, timelineKeys.Search):
			c.mode = commandModeSearch
			c.input.SetValue("")
			c.input.Focus()
			return c, textinput.Blink
		case key.Matches(msg, timelineKeys.Enter):
			selectedItem, idx := c.listView.GetSelectedItem()
			if idx != -1 {
				return c, util.CmdHandler(TimelineJumpMsg{
					MessageID: selectedItem.MessageID,
				})
			}
		case key.Matches(msg, timelineKeys.Escape):
			return c, util.CmdHandler(CloseTimelineDialogMsg{})
		}
	case utilComponents.ListActivatedMsg:
		// A click already moved the selection; activate it like Enter
		// (Spec 09n).
		selectedItem, idx := c.listView.GetSelectedItem()
		if idx != -1 {
			return c, util.CmdHandler(TimelineJumpMsg{
				MessageID: selectedItem.MessageID,
			})
		}
		return c, nil
	case tea.WindowSizeMsg:
		c.width = msg.Width
		c.height = msg.Height
	}

	// Moving the selection previews the message by scrolling the transcript
	// behind the dialog (Spec 09o).
	prev, _ := c.listView.GetSelectedItem()
	u, cmd := c.listView.Update(msg)
	c.listView = u.(utilComponents.SimpleList[TimelineItem])
	cmds = append(cmds, cmd)
	if cur, idx := c.listView.GetSelectedItem(); idx != -1 && cur.MessageID != prev.MessageID {
		cmds = append(cmds, util.CmdHandler(TimelinePreviewMsg{MessageID: cur.MessageID}))
	}

	return c, tea.Batch(cmds...)
}

// applyFilter narrows the list with a case-insensitive fuzzy match over the
// message text, ordered by Levenshtein distance (Spec 09o).
func (c *timelineDialogCmp) applyFilter() {
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

func (c *timelineDialogCmp) View() string {
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
		Render("Timeline")

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

func (c *timelineDialogCmp) BindingKeys() []key.Binding {
	return layout.KeyMapToSlice(timelineKeys)
}

func (c *timelineDialogCmp) SetItems(items []TimelineItem) {
	// A search left over from a previous opening must not shrink the freshly
	// loaded list (Spec 09o).
	c.mode = commandModeList
	c.input.SetValue("")
	c.input.Blur()
	c.all = items
	c.listView.SetItems(items)
}

// NewTimelineDialogCmp creates the timeline picker (Spec 09o).
func NewTimelineDialogCmp() TimelineDialog {
	listView := utilComponents.NewSimpleList[TimelineItem](
		[]TimelineItem{},
		10,
		"No user messages yet",
		true,
	)
	input := textinput.New()
	input.Prompt = ""
	input.Placeholder = "search timeline..."
	input.CharLimit = 64
	return &timelineDialogCmp{
		listView: listView,
		input:    input,
	}
}
