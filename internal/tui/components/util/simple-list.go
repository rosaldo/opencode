package utilComponents

import (
	"strconv"
	"strings"

	"github.com/charmbracelet/bubbles/key"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/opencode-ai/opencode/internal/tui/layout"
	"github.com/opencode-ai/opencode/internal/tui/styles"
	"github.com/opencode-ai/opencode/internal/tui/theme"
	"github.com/opencode-ai/opencode/internal/tui/util"
)

// ListActivatedMsg reports that the user clicked a row of a SimpleList
// (Spec 09n). The owner decides what "activate" means; the selection has
// already moved to the clicked row.
type ListActivatedMsg struct {
	Index int
}

// listZonePrefix namespaces the mouse zones of every SimpleList row.
const listZonePrefix = "slist:"

type SimpleListItem interface {
	Render(selected bool, width int) string
}

type SimpleList[T SimpleListItem] interface {
	tea.Model
	layout.Bindings
	SetMaxWidth(maxWidth int)
	GetSelectedItem() (item T, idx int)
	SetItems(items []T)
	GetItems() []T
	// SelectIndex moves the selection to idx (Spec 09u); out-of-range
	// indexes are ignored.
	SelectIndex(idx int)
	// SetGroupHeaders toggles the non-selectable group headers rendered above
	// the first row of each group (Spec 09s). Off by default.
	SetGroupHeaders(show bool)
}

type simpleListCmp[T SimpleListItem] struct {
	fallbackMsg         string
	items               []T
	selectedIdx         int
	maxWidth            int
	maxVisibleItems     int
	useAlphaNumericKeys bool
	width               int
	height              int
	zones               util.ZoneSet
	showGroupHeaders    bool
}

type simpleListKeyMap struct {
	Up        key.Binding
	Down      key.Binding
	UpAlpha   key.Binding
	DownAlpha key.Binding
}

var simpleListKeys = simpleListKeyMap{
	Up: key.NewBinding(
		key.WithKeys("up"),
		key.WithHelp("↑", "previous list item"),
	),
	Down: key.NewBinding(
		key.WithKeys("down"),
		key.WithHelp("↓", "next list item"),
	),
	UpAlpha: key.NewBinding(
		key.WithKeys("k"),
		key.WithHelp("k", "previous list item"),
	),
	DownAlpha: key.NewBinding(
		key.WithKeys("j"),
		key.WithHelp("j", "next list item"),
	),
}

func (c *simpleListCmp[T]) Init() tea.Cmd {
	return nil
}

func (c *simpleListCmp[T]) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch {
		case key.Matches(msg, simpleListKeys.Up) || (c.useAlphaNumericKeys && key.Matches(msg, simpleListKeys.UpAlpha)):
			if c.selectedIdx > 0 {
				c.selectedIdx--
			}
			return c, nil
		case key.Matches(msg, simpleListKeys.Down) || (c.useAlphaNumericKeys && key.Matches(msg, simpleListKeys.DownAlpha)):
			if c.selectedIdx < len(c.items)-1 {
				c.selectedIdx++
			}
			return c, nil
		}
	case tea.MouseMsg:
		// A left click selects the row and activates it (Spec 09n).
		if msg.Action == tea.MouseActionPress && msg.Button == tea.MouseButtonLeft {
			if idx, ok := ListClickedRow(&c.zones, listZonePrefix, msg); ok {
				return c, c.clickRow(idx)
			}
		}
	}

	return c, nil
}

// clickRow moves the selection to idx and reports the activation; out-of-range
// indexes are ignored (Spec 09n).
func (c *simpleListCmp[T]) clickRow(idx int) tea.Cmd {
	if idx < 0 || idx >= len(c.items) {
		return nil
	}
	c.selectedIdx = idx
	return util.CmdHandler(ListActivatedMsg{Index: idx})
}

func (c *simpleListCmp[T]) BindingKeys() []key.Binding {
	return layout.KeyMapToSlice(simpleListKeys)
}

func (c *simpleListCmp[T]) GetSelectedItem() (T, int) {
	if len(c.items) > 0 {
		return c.items[c.selectedIdx], c.selectedIdx
	}

	var zero T
	return zero, -1
}

func (c *simpleListCmp[T]) SetItems(items []T) {
	c.selectedIdx = 0
	c.items = items
}

func (c *simpleListCmp[T]) GetItems() []T {
	return c.items
}

func (c *simpleListCmp[T]) SelectIndex(idx int) {
	if idx < 0 || idx >= len(c.items) {
		return
	}
	c.selectedIdx = idx
}

func (c *simpleListCmp[T]) SetGroupHeaders(show bool) {
	c.showGroupHeaders = show
}

func (c *simpleListCmp[T]) SetMaxWidth(width int) {
	c.maxWidth = width
}

// groupTitle returns the header text shown above item i, or "" when no header
// starts there (headers belong to items whose group differs from the previous
// row; Spec 09s).
func (c *simpleListCmp[T]) groupTitle(i int) string {
	if !c.showGroupHeaders || i < 0 || i >= len(c.items) {
		return ""
	}
	grouper, ok := any(c.items[i]).(interface{ GroupTitle() string })
	if !ok {
		return ""
	}
	title := grouper.GroupTitle()
	if title == "" {
		return ""
	}
	if i > 0 {
		prev, ok := any(c.items[i-1]).(interface{ GroupTitle() string })
		if ok && prev.GroupTitle() == title {
			return ""
		}
	}
	return title
}

// lineCount is how many rendered lines item i occupies (row plus header).
func (c *simpleListCmp[T]) lineCount(i int) int {
	if c.groupTitle(i) != "" {
		return 2
	}
	return 1
}

func (c *simpleListCmp[T]) View() string {
	t := theme.CurrentTheme()
	baseStyle := styles.BaseStyle()

	items := c.items
	maxWidth := c.maxWidth

	if len(items) <= 0 {
		return baseStyle.
			Background(t.Background()).
			Padding(0, 1).
			Width(maxWidth).
			Render(c.fallbackMsg)
	}

	startIdx, endIdx := c.window()

	listItems := make([]string, 0, endIdx-startIdx)
	c.zones.Reset()

	for i := startIdx; i < endIdx; i++ {
		// Group headers are plain lines: they carry no zone, so a click on
		// one is swallowed instead of activating a row (Spec 09s).
		if title := c.groupTitle(i); title != "" {
			listItems = append(listItems, baseStyle.
				Width(maxWidth).
				Padding(0, 1).
				Foreground(t.Primary()).
				Bold(true).
				Render(title))
		}
		item := items[i]
		row := item.Render(i == c.selectedIdx, maxWidth)
		// The zone carries the absolute index so a click can be mapped back
		// even while the list is windowed (Spec 09n).
		listItems = append(listItems, c.zones.Mark(ListZoneID(listZonePrefix, i), row))
	}

	return lipgloss.JoinVertical(lipgloss.Left, listItems...)
}

// window computes the [start, end) slice of items to render. Without group
// headers it keeps the classic item-count window centered on the selection;
// with headers the budget counts rendered lines (row + header) so the list
// never grows past maxVisibleItems lines (Spec 09s).
func (c *simpleListCmp[T]) window() (int, int) {
	items := c.items
	maxVisibleItems := c.maxVisibleItems

	if !c.showGroupHeaders {
		startIdx := 0
		if len(items) > maxVisibleItems {
			halfVisible := maxVisibleItems / 2
			if c.selectedIdx >= halfVisible && c.selectedIdx < len(items)-halfVisible {
				startIdx = c.selectedIdx - halfVisible
			} else if c.selectedIdx >= len(items)-halfVisible {
				startIdx = len(items) - maxVisibleItems
			}
		}
		return startIdx, min(startIdx+maxVisibleItems, len(items))
	}

	total := 0
	for i := range items {
		total += c.lineCount(i)
	}
	if total <= maxVisibleItems {
		return 0, len(items)
	}

	// Grow a window around the selection, alternating sides, while the line
	// budget allows it.
	sel := c.selectedIdx
	start, end := sel, sel+1
	used := c.lineCount(sel)
	up, down := sel-1, sel+1
	growDown := false
	for used < maxVisibleItems && (up >= 0 || down < len(items)) {
		added := false
		if down < len(items) && (growDown || up < 0) {
			if n := c.lineCount(down); used+n <= maxVisibleItems {
				used += n
				end = down + 1
				down++
				added = true
			}
		}
		if !added && up >= 0 {
			if n := c.lineCount(up); used+n <= maxVisibleItems {
				used += n
				start = up
				up--
				added = true
			}
		}
		if !added {
			break
		}
		growDown = !growDown
	}
	return start, end
}

// ListZoneID builds the mouse zone id for the row at idx.
func ListZoneID(prefix string, idx int) string {
	return prefix + strconv.Itoa(idx)
}

// ListClickedRow resolves the absolute row index under the cursor. Shared by
// the SimpleList and the hand-rolled list dialogs (Spec 09n).
func ListClickedRow(zones *util.ZoneSet, prefix string, msg tea.MouseMsg) (int, bool) {
	id, ok := zones.At(msg)
	if !ok {
		return 0, false
	}
	raw, found := strings.CutPrefix(id, prefix)
	if !found {
		return 0, false
	}
	idx, err := strconv.Atoi(raw)
	if err != nil || idx < 0 {
		return 0, false
	}
	return idx, true
}

func NewSimpleList[T SimpleListItem](items []T, maxVisibleItems int, fallbackMsg string, useAlphaNumericKeys bool) SimpleList[T] {
	return &simpleListCmp[T]{
		fallbackMsg:         fallbackMsg,
		items:               items,
		maxVisibleItems:     maxVisibleItems,
		useAlphaNumericKeys: useAlphaNumericKeys,
		selectedIdx:         0,
	}
}
