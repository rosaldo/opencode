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
	"github.com/opencode-ai/opencode/internal/agentdef"
	"github.com/opencode-ai/opencode/internal/tui/layout"
	"github.com/opencode-ai/opencode/internal/tui/styles"
	"github.com/opencode-ai/opencode/internal/tui/theme"
	"github.com/opencode-ai/opencode/internal/tui/util"
)

const numVisibleAgents = 10

// AgentSelectedMsg is sent when the user picks an agent in the switcher.
type AgentSelectedMsg struct {
	Name string
}

// ShowAgentsDialogMsg opens the agent switcher.
type ShowAgentsDialogMsg struct {
	Show bool
}

// CloseAgentsDialogMsg closes the agent switcher without selecting.
type CloseAgentsDialogMsg struct{}

// AgentDialog is the interface for the agent selection dialog.
type AgentDialog interface {
	tea.Model
	layout.Bindings
}

type agentDialogCmp struct {
	agents      []agentdef.Definition
	filteredIdx []int

	input        textinput.Model
	selectedIdx  int
	scrollOffset int
	activeAgent  func() string
}

type agentKeyMap struct {
	Up     key.Binding
	Down   key.Binding
	Enter  key.Binding
	Escape key.Binding
}

var agentKeys = agentKeyMap{
	Up: key.NewBinding(
		key.WithKeys("up", "ctrl+k"),
		key.WithHelp("↑", "previous agent"),
	),
	Down: key.NewBinding(
		key.WithKeys("down", "ctrl+j"),
		key.WithHelp("↓", "next agent"),
	),
	Enter: key.NewBinding(
		key.WithKeys("enter"),
		key.WithHelp("enter", "select agent"),
	),
	Escape: key.NewBinding(
		key.WithKeys("esc"),
		key.WithHelp("esc", "close"),
	),
}

// NewAgentDialogCmp creates the agent switcher. activeAgent returns the name
// of the agent currently selected in the AgentManager and is used to
// pre-highlight it in the list.
func NewAgentDialogCmp(activeAgent func() string) AgentDialog {
	input := textinput.New()
	input.Prompt = ""
	input.Placeholder = "search agents..."
	input.CharLimit = 64

	return &agentDialogCmp{
		input:       input,
		activeAgent: activeAgent,
	}
}

func (m *agentDialogCmp) Init() tea.Cmd {
	m.reload()
	return textinput.Blink
}

// reload re-reads the registry and re-applies the current filter. Called on
// every open so newly added agent files appear without a restart.
func (m *agentDialogCmp) reload() {
	defs := agentdef.Get().ListByMode(agentdef.ModePrimary, false)
	m.agents = make([]agentdef.Definition, 0, len(defs))
	for _, d := range defs {
		m.agents = append(m.agents, *d)
	}
	m.input.SetValue("")
	m.input.CursorEnd()
	m.applyFilter()
}

func (m *agentDialogCmp) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch {
		case key.Matches(msg, agentKeys.Escape):
			return m, util.CmdHandler(CloseAgentsDialogMsg{})
		case key.Matches(msg, agentKeys.Enter):
			if len(m.filteredIdx) == 0 {
				return m, nil
			}
			name := m.agents[m.filteredIdx[m.selectedIdx]].Name
			return m, util.CmdHandler(AgentSelectedMsg{Name: name})
		case key.Matches(msg, agentKeys.Up):
			m.moveSelection(-1)
			return m, nil
		case key.Matches(msg, agentKeys.Down):
			m.moveSelection(1)
			return m, nil
		}
		var cmd tea.Cmd
		m.input, cmd = m.input.Update(msg)
		m.applyFilter()
		return m, cmd
	case tea.WindowSizeMsg:
		return m, nil
	}
	return m, nil
}

func (m *agentDialogCmp) moveSelection(delta int) {
	if len(m.filteredIdx) == 0 {
		return
	}
	m.selectedIdx += delta
	if m.selectedIdx < 0 {
		m.selectedIdx = len(m.filteredIdx) - 1
		m.scrollOffset = max(0, len(m.filteredIdx)-numVisibleAgents)
	} else if m.selectedIdx >= len(m.filteredIdx) {
		m.selectedIdx = 0
		m.scrollOffset = 0
	}
	if m.selectedIdx < m.scrollOffset {
		m.scrollOffset = m.selectedIdx
	} else if m.selectedIdx >= m.scrollOffset+numVisibleAgents {
		m.scrollOffset = m.selectedIdx - (numVisibleAgents - 1)
	}
}

// applyFilter narrows the list with a case-insensitive fuzzy (subsequence)
// match over "name description". An empty query keeps registration order;
// otherwise matches keep the rank order returned by the matcher.
func (m *agentDialogCmp) applyFilter() {
	query := strings.TrimSpace(m.input.Value())
	m.filteredIdx = make([]int, 0, len(m.agents))
	if query == "" {
		for i := range m.agents {
			m.filteredIdx = append(m.filteredIdx, i)
		}
	} else {
		targets := make([]string, len(m.agents))
		for i := range m.agents {
			targets[i] = m.agents[i].Name + " " + m.agents[i].Description
		}
		ranks := fuzzy.RankFindFold(query, targets)
		// rankFind leaves matches in registry order; sort by distance so the
		// closest names surface first.
		slices.SortStableFunc(ranks, func(a, b fuzzy.Rank) int {
			return a.Distance - b.Distance
		})
		for _, r := range ranks {
			m.filteredIdx = append(m.filteredIdx, r.OriginalIndex)
		}
	}
	if m.selectedIdx >= len(m.filteredIdx) {
		m.selectedIdx = max(0, len(m.filteredIdx)-1)
	}
	m.scrollOffset = 0
	if m.selectedIdx >= numVisibleAgents {
		m.scrollOffset = m.selectedIdx - (numVisibleAgents - 1)
	}
}

func (m *agentDialogCmp) View() string {
	t := theme.CurrentTheme()
	baseStyle := styles.BaseStyle()

	title := baseStyle.
		Foreground(t.Primary()).
		Bold(true).
		Width(maxDialogWidth).
		Padding(0, 0, 1).
		Render("Select Agent")

	search := baseStyle.
		Width(maxDialogWidth).
		Render(m.input.View())

	var active string
	if m.activeAgent != nil {
		active = m.activeAgent()
	}

	endIdx := min(m.scrollOffset+numVisibleAgents, len(m.filteredIdx))
	items := make([]string, 0, endIdx-m.scrollOffset)
	for row := m.scrollOffset; row < endIdx; row++ {
		def := m.agents[m.filteredIdx[row]]
		label := def.Name
		if def.Description != "" {
			label = def.Name + " — " + def.Description
		}
		if runeLen := len(label); runeLen > maxDialogWidth {
			label = label[:maxDialogWidth-1] + "…"
		}
		itemStyle := baseStyle.Width(maxDialogWidth)
		if def.Name == active {
			label += " ●"
		}
		if row == m.selectedIdx {
			itemStyle = itemStyle.Background(t.Primary()).
				Foreground(t.Background()).Bold(true)
		}
		items = append(items, itemStyle.Render(label))
	}

	var indicator string
	if len(m.filteredIdx) > numVisibleAgents {
		if m.scrollOffset > 0 {
			indicator += "↑ "
		}
		if m.scrollOffset+numVisibleAgents < len(m.filteredIdx) {
			indicator += "↓"
		}
	}

	counts := baseStyle.
		Foreground(t.TextMuted()).
		Width(maxDialogWidth).
		Render(fmt.Sprintf("%d/%d agents", len(m.filteredIdx), len(m.agents)))

	body := lipgloss.JoinVertical(
		lipgloss.Left,
		title,
		search,
		baseStyle.Width(maxDialogWidth).Render(lipgloss.JoinVertical(lipgloss.Left, items...)),
		indicator,
		counts,
	)

	return baseStyle.Padding(1, 2).
		Border(lipgloss.RoundedBorder()).
		BorderBackground(t.Background()).
		BorderForeground(t.TextMuted()).
		Width(lipgloss.Width(body) + 4).
		Render(body)
}

func (m *agentDialogCmp) BindingKeys() []key.Binding {
	return layout.KeyMapToSlice(agentKeys)
}
