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
	"github.com/opencode-ai/opencode/internal/promptstash"
	utilComponents "github.com/opencode-ai/opencode/internal/tui/components/util"
	"github.com/opencode-ai/opencode/internal/tui/layout"
	"github.com/opencode-ai/opencode/internal/tui/styles"
	"github.com/opencode-ai/opencode/internal/tui/theme"
	"github.com/opencode-ai/opencode/internal/tui/util"
)

// stashPreviewLimit is the maximum preview width of a stashed prompt title.
const stashPreviewLimit = 50

// StashItem is one row of the stash dialog: a prompt parked by the
// prompt_stash bindings (Spec 09q).
type StashItem struct {
	// Index is the position in the stash store (oldest = 0); it survives the
	// newest-first projection and is what select/delete report back.
	Index     int
	Title     string
	CreatedAt time.Time
	Lines     int
}

// StashItems projects the stash into dialog rows, newest first — the same
// order as the TS stash dialog (Spec 09q).
func StashItems(entries []promptstash.Entry) []StashItem {
	items := make([]StashItem, 0, len(entries))
	for i, entry := range entries {
		text := entry.Text
		first, _, _ := strings.Cut(text, "\n")
		title := first
		if r := []rune(title); len(r) > stashPreviewLimit {
			title = string(r[:stashPreviewLimit]) + "..."
		}
		items = append(items, StashItem{
			Index:     i,
			Title:     title,
			CreatedAt: time.UnixMilli(entry.Timestamp),
			Lines:     1 + strings.Count(text, "\n"),
		})
	}
	slices.Reverse(items)
	return items
}

// Render shows the prompt preview with a muted footer: the relative age of
// the entry and, for multiline prompts, an approximate line count (Spec 09q).
func (i StashItem) Render(selected bool, width int) string {
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
	rows := []string{
		itemStyle.Padding(0, 1).Render(title),
		descStyle.Padding(0, 1).Render(stashRelativeTime(i.CreatedAt, time.Now())),
	}
	if i.Lines > 1 {
		rows = append(rows, descStyle.Padding(0, 1).Render(fmt.Sprintf("~%d lines", i.Lines)))
	}
	return lipgloss.JoinVertical(lipgloss.Left, rows...)
}

// stashRelativeTime formats the age of a stash entry the way the TS dialog
// does: relative up to a week, absolute after that.
func stashRelativeTime(created, now time.Time) string {
	age := now.Sub(created)
	switch {
	case age < time.Minute:
		return "just now"
	case age < time.Hour:
		return fmt.Sprintf("%dm ago", int(age.Minutes()))
	case age < 24*time.Hour:
		return fmt.Sprintf("%dh ago", int(age.Hours()))
	case age < 7*24*time.Hour:
		return fmt.Sprintf("%dd ago", int(age.Hours()/24))
	default:
		return created.Format("2006-01-02 15:04")
	}
}

// StashSelectMsg asks the app to restore the selected stashed prompt into the
// composer and drop it from the stash (Spec 09q).
type StashSelectMsg struct {
	Index int
}

// StashDeleteMsg asks the app to drop the selected stashed prompt without
// restoring it (second ctrl+d, Spec 09q).
type StashDeleteMsg struct {
	Index int
}

// CloseStashDialogMsg is sent when the stash dialog is closed without
// selecting (Spec 09q).
type CloseStashDialogMsg struct{}

// StashDialog interface for the stash picker (Spec 09q).
type StashDialog interface {
	tea.Model
	layout.Bindings
	SetItems(items []StashItem)
}

type stashDialogCmp struct {
	listView utilComponents.SimpleList[StashItem]
	all      []StashItem
	mode     commandMode
	input    textinput.Model
	// armedIdx marks the row whose next ctrl+d confirms the deletion
	// (Spec 09q).
	armed    bool
	armedIdx int
	width    int
	height   int
}

type stashKeyMap struct {
	Enter  key.Binding
	Escape key.Binding
	Search key.Binding
	Delete key.Binding
}

var stashKeys = stashKeyMap{
	Enter: key.NewBinding(
		key.WithKeys("enter"),
		key.WithHelp("enter", "restore prompt"),
	),
	Escape: key.NewBinding(
		key.WithKeys("esc"),
		key.WithHelp("esc", "close"),
	),
	Search: key.NewBinding(
		key.WithKeys("/"),
		key.WithHelp("/", "search"),
	),
	Delete: key.NewBinding(
		key.WithKeys("ctrl+d"),
		key.WithHelp("ctrl+d", "delete entry"),
	),
}

func (c *stashDialogCmp) Init() tea.Cmd {
	return c.listView.Init()
}

func (c *stashDialogCmp) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmds []tea.Cmd
	switch msg := msg.(type) {
	case tea.KeyMsg:
		if c.mode == commandModeSearch {
			switch {
			case key.Matches(msg, stashKeys.Escape), key.Matches(msg, stashKeys.Enter):
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
		case key.Matches(msg, stashKeys.Search):
			c.mode = commandModeSearch
			c.input.SetValue("")
			c.input.Focus()
			return c, textinput.Blink
		case key.Matches(msg, stashKeys.Enter):
			return c, c.activate()
		case key.Matches(msg, stashKeys.Delete):
			return c, c.deleteSelected()
		case key.Matches(msg, stashKeys.Escape):
			if c.armed {
				// First Esc disarms; a second one closes (Spec 09q).
				c.disarm()
				return c, nil
			}
			return c, util.CmdHandler(CloseStashDialogMsg{})
		}
	case utilComponents.ListActivatedMsg:
		// A click already moved the selection; activate it like Enter
		// (Spec 09n).
		return c, c.activate()
	case tea.WindowSizeMsg:
		c.width = msg.Width
		c.height = msg.Height
	}

	// Moving the selection away from the armed row cancels the pending
	// confirmation (Spec 09q).
	u, cmd := c.listView.Update(msg)
	c.listView = u.(utilComponents.SimpleList[StashItem])
	cmds = append(cmds, cmd)
	if c.armed {
		cur, idx := c.listView.GetSelectedItem()
		if idx == -1 || cur.Index != c.armedIdx {
			c.disarm()
		}
	}

	return c, tea.Batch(cmds...)
}

// activate restores the selected entry into the composer (Spec 09q).
func (c *stashDialogCmp) activate() tea.Cmd {
	selectedItem, idx := c.listView.GetSelectedItem()
	if idx == -1 {
		return nil
	}
	return util.CmdHandler(StashSelectMsg{Index: selectedItem.Index})
}

// deleteSelected arms the selected row, or — when it is already armed —
// confirms the deletion, drops the row and reports the original store index
// (Spec 09q).
func (c *stashDialogCmp) deleteSelected() tea.Cmd {
	selectedItem, idx := c.listView.GetSelectedItem()
	if idx == -1 {
		return nil
	}
	if c.armed && c.armedIdx == selectedItem.Index {
		index := selectedItem.Index
		c.removeRow(index)
		return util.CmdHandler(StashDeleteMsg{Index: index})
	}
	c.armed = true
	c.armedIdx = selectedItem.Index
	return nil
}

func (c *stashDialogCmp) disarm() {
	c.armed = false
	c.armedIdx = -1
}

// removeRow drops the entry from the loaded rows and renumbers the ones after
// it so indexes keep matching the store (Spec 09q).
func (c *stashDialogCmp) removeRow(storeIndex int) {
	c.all = removeStashItems(c.all, storeIndex)
	c.disarm()
	c.applyFilter()
}

func removeStashItems(items []StashItem, storeIndex int) []StashItem {
	kept := make([]StashItem, 0, len(items))
	for _, item := range items {
		if item.Index == storeIndex {
			continue
		}
		if item.Index > storeIndex {
			item.Index--
		}
		kept = append(kept, item)
	}
	return kept
}

// applyFilter narrows the list with a case-insensitive fuzzy match over the
// prompt preview, ordered by Levenshtein distance. Editing the query cancels a
// pending deletion confirmation (Spec 09q).
func (c *stashDialogCmp) applyFilter() {
	c.disarm()
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
	items := make([]StashItem, 0, len(ranks))
	for _, r := range ranks {
		items = append(items, c.all[r.OriginalIndex])
	}
	c.listView.SetItems(items)
}

func (c *stashDialogCmp) View() string {
	t := theme.CurrentTheme()
	baseStyle := styles.BaseStyle()

	maxWidth := 40

	// Width follows every loaded entry so the frame does not resize while
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
		Render("Stash")

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
		Render("enter restore · ctrl+d delete · / search · ↑↓ jk navigate")
	switch {
	case c.armed:
		// Destructive confirmation stands out like the TS dialog (Spec 09q).
		hint = baseStyle.
			Width(maxWidth).
			Foreground(t.Error()).
			Render("press ctrl+d again to delete · esc disarm")
	case c.mode == commandModeSearch:
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

func (c *stashDialogCmp) BindingKeys() []key.Binding {
	return layout.KeyMapToSlice(stashKeys)
}

func (c *stashDialogCmp) SetItems(items []StashItem) {
	// A search or arm left over from a previous opening must not affect the
	// freshly loaded list (Spec 09q).
	c.mode = commandModeList
	c.input.SetValue("")
	c.input.Blur()
	c.disarm()
	c.all = items
	c.listView.SetItems(items)
}

// NewStashDialogCmp creates the stash picker (Spec 09q).
func NewStashDialogCmp() StashDialog {
	listView := utilComponents.NewSimpleList[StashItem](
		[]StashItem{},
		10,
		"No stashed prompts",
		true,
	)
	input := textinput.New()
	input.Prompt = ""
	input.Placeholder = "search stash..."
	input.CharLimit = 64
	return &stashDialogCmp{
		listView: listView,
		input:    input,
		armedIdx: -1,
	}
}
