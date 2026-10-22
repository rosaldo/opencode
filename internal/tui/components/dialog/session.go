package dialog

import (
	"strings"

	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/opencode-ai/opencode/internal/session"
	"github.com/opencode-ai/opencode/internal/tui/layout"
	"github.com/opencode-ai/opencode/internal/tui/styles"
	"github.com/opencode-ai/opencode/internal/tui/theme"
	"github.com/opencode-ai/opencode/internal/tui/util"
)

// SessionSelectedMsg is sent when a session is selected
type SessionSelectedMsg struct {
	Session session.Session
}

// SessionRenameRequestMsg asks the app to rename a session (Spec 09).
type SessionRenameRequestMsg struct {
	Session session.Session
	Title   string
}

// SessionDeleteRequestMsg asks the app to delete a session (Spec 09).
type SessionDeleteRequestMsg struct {
	Session session.Session
}

// CloseSessionDialogMsg is sent when the session dialog is closed
type CloseSessionDialogMsg struct{}

// SessionDialog interface for the session switching dialog
type SessionDialog interface {
	tea.Model
	layout.Bindings
	SetSessions(sessions []session.Session)
	SetSelectedSession(sessionID string)
}

type sessionMode int

const (
	sessionModeList sessionMode = iota
	sessionModeSearch
	sessionModeRename
)

type sessionDialogCmp struct {
	allSessions       []session.Session
	sessions          []session.Session
	selectedIdx       int
	width             int
	height            int
	selectedSessionID string
	mode              sessionMode
	input             textinput.Model
	pendingDelete     bool
}

type sessionKeyMap struct {
	Up      key.Binding
	Down    key.Binding
	Enter   key.Binding
	Escape  key.Binding
	J       key.Binding
	K       key.Binding
	Search  key.Binding
	Rename  key.Binding
	Delete  key.Binding
	Confirm key.Binding
}

var sessionKeys = sessionKeyMap{
	Up: key.NewBinding(
		key.WithKeys("up"),
		key.WithHelp("↑", "previous session"),
	),
	Down: key.NewBinding(
		key.WithKeys("down"),
		key.WithHelp("↓", "next session"),
	),
	Enter: key.NewBinding(
		key.WithKeys("enter"),
		key.WithHelp("enter", "select session"),
	),
	Escape: key.NewBinding(
		key.WithKeys("esc"),
		key.WithHelp("esc", "close"),
	),
	J: key.NewBinding(
		key.WithKeys("j"),
		key.WithHelp("j", "next session"),
	),
	K: key.NewBinding(
		key.WithKeys("k"),
		key.WithHelp("k", "previous session"),
	),
	Search: key.NewBinding(
		key.WithKeys("/"),
		key.WithHelp("/", "search"),
	),
	Rename: key.NewBinding(
		key.WithKeys("r"),
		key.WithHelp("r", "rename"),
	),
	Delete: key.NewBinding(
		key.WithKeys("d"),
		key.WithHelp("d", "delete"),
	),
	Confirm: key.NewBinding(
		key.WithKeys("y"),
		key.WithHelp("y", "confirm delete"),
	),
}

func (s *sessionDialogCmp) Init() tea.Cmd {
	return nil
}

// filtered returns the sessions matching the current search query. With an empty
// query it returns every session in order. This is pure logic used by Update and
// covered by tests.
func (s *sessionDialogCmp) filtered() []session.Session {
	if s.mode != sessionModeSearch {
		return s.allSessions
	}
	q := strings.ToLower(strings.TrimSpace(s.input.Value()))
	if q == "" {
		return s.allSessions
	}
	out := make([]session.Session, 0, len(s.allSessions))
	for _, sess := range s.allSessions {
		if strings.Contains(strings.ToLower(sess.Title), q) || strings.Contains(strings.ToLower(sess.ID), q) {
			out = append(out, sess)
		}
	}
	return out
}

func (s *sessionDialogCmp) current() (session.Session, bool) {
	if s.selectedIdx < 0 || s.selectedIdx >= len(s.sessions) {
		return session.Session{}, false
	}
	return s.sessions[s.selectedIdx], true
}

func (s *sessionDialogCmp) refreshFiltered() {
	s.sessions = s.filtered()
	if s.selectedIdx >= len(s.sessions) {
		s.selectedIdx = max(0, len(s.sessions)-1)
	}
}

func (s *sessionDialogCmp) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		s.width = msg.Width
		s.height = msg.Height
	case tea.KeyMsg:
		if s.pendingDelete {
			switch {
			case key.Matches(msg, sessionKeys.Confirm):
				sess, ok := s.current()
				s.pendingDelete = false
				if ok {
					return s, util.CmdHandler(SessionDeleteRequestMsg{Session: sess})
				}
			default:
				s.pendingDelete = false
			}
			return s, nil
		}
		switch s.mode {
		case sessionModeSearch:
			switch {
			case key.Matches(msg, sessionKeys.Escape), key.Matches(msg, sessionKeys.Enter):
				s.mode = sessionModeList
				s.refreshFiltered()
				return s, nil
			}
			var cmd tea.Cmd
			s.input, cmd = s.input.Update(msg)
			s.refreshFiltered()
			return s, cmd
		case sessionModeRename:
			switch {
			case key.Matches(msg, sessionKeys.Escape):
				s.mode = sessionModeList
				return s, nil
			case key.Matches(msg, sessionKeys.Enter):
				title := strings.TrimSpace(s.input.Value())
				sess, ok := s.current()
				s.mode = sessionModeList
				if ok && title != "" {
					return s, util.CmdHandler(SessionRenameRequestMsg{Session: sess, Title: title})
				}
				return s, nil
			}
			var cmd tea.Cmd
			s.input, cmd = s.input.Update(msg)
			return s, cmd
		}
		switch {
		case key.Matches(msg, sessionKeys.Up) || key.Matches(msg, sessionKeys.K):
			if s.selectedIdx > 0 {
				s.selectedIdx--
			}
			return s, nil
		case key.Matches(msg, sessionKeys.Down) || key.Matches(msg, sessionKeys.J):
			if s.selectedIdx < len(s.sessions)-1 {
				s.selectedIdx++
			}
			return s, nil
		case key.Matches(msg, sessionKeys.Search):
			s.mode = sessionModeSearch
			s.input.SetValue("")
			s.input.Focus()
			return s, textinput.Blink
		case key.Matches(msg, sessionKeys.Rename):
			if sess, ok := s.current(); ok {
				s.mode = sessionModeRename
				s.input.SetValue(sess.Title)
				s.input.CursorEnd()
				s.input.Focus()
				return s, textinput.Blink
			}
			return s, nil
		case key.Matches(msg, sessionKeys.Delete):
			if _, ok := s.current(); ok {
				s.pendingDelete = true
			}
			return s, nil
		case key.Matches(msg, sessionKeys.Enter):
			if sess, ok := s.current(); ok {
				return s, util.CmdHandler(SessionSelectedMsg{Session: sess})
			}
		case key.Matches(msg, sessionKeys.Escape):
			return s, util.CmdHandler(CloseSessionDialogMsg{})
		}
	}
	return s, nil
}

func (s *sessionDialogCmp) View() string {
	t := theme.CurrentTheme()
	baseStyle := styles.BaseStyle()

	if len(s.sessions) == 0 && len(s.allSessions) == 0 {
		return baseStyle.Padding(1, 2).
			Border(lipgloss.RoundedBorder()).
			BorderBackground(t.Background()).
			BorderForeground(t.TextMuted()).
			Width(40).
			Render("No sessions available")
	}

	maxWidth := 40
	for _, sess := range s.allSessions {
		if len(sess.Title) > maxWidth-4 {
			maxWidth = len(sess.Title) + 4
		}
	}
	maxWidth = max(30, min(maxWidth, s.width-15))

	maxVisibleSessions := min(10, len(s.sessions))
	sessionItems := make([]string, 0, maxVisibleSessions)
	startIdx := 0
	if len(s.sessions) > maxVisibleSessions {
		halfVisible := maxVisibleSessions / 2
		if s.selectedIdx >= halfVisible && s.selectedIdx < len(s.sessions)-halfVisible {
			startIdx = s.selectedIdx - halfVisible
		} else if s.selectedIdx >= len(s.sessions)-halfVisible {
			startIdx = len(s.sessions) - maxVisibleSessions
		}
	}
	endIdx := min(startIdx+maxVisibleSessions, len(s.sessions))

	for i := startIdx; i < endIdx; i++ {
		sess := s.sessions[i]
		itemStyle := baseStyle.Width(maxWidth)
		if i == s.selectedIdx {
			itemStyle = itemStyle.Background(t.Primary()).Foreground(t.Background()).Bold(true)
		}
		sessionItems = append(sessionItems, itemStyle.Padding(0, 1).Render(sess.Title))
	}

	title := baseStyle.Foreground(t.Primary()).Bold(true).Width(maxWidth).Padding(0, 1).Render("Switch Session")

	rows := []string{title, baseStyle.Width(maxWidth).Render("")}

	if s.mode == sessionModeSearch {
		rows = append(rows, baseStyle.Width(maxWidth).Padding(0, 1).Render("Search: "+s.input.View()))
		rows = append(rows, baseStyle.Width(maxWidth).Render(""))
	} else if s.mode == sessionModeRename {
		rows = append(rows, baseStyle.Width(maxWidth).Padding(0, 1).Render("Rename: "+s.input.View()))
		rows = append(rows, baseStyle.Width(maxWidth).Render(""))
	}

	if len(s.sessions) == 0 {
		rows = append(rows, baseStyle.Width(maxWidth).Render("(no matches)"))
	} else {
		rows = append(rows, baseStyle.Width(maxWidth).Render(lipgloss.JoinVertical(lipgloss.Left, sessionItems...)))
	}

	if s.pendingDelete {
		rows = append(rows, baseStyle.Width(maxWidth).Foreground(t.Error()).Render("Delete? press y to confirm"))
	} else {
		rows = append(rows, baseStyle.Width(maxWidth).Foreground(t.TextMuted()).Render("/ search · r rename · d delete"))
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

func (s *sessionDialogCmp) BindingKeys() []key.Binding {
	return layout.KeyMapToSlice(sessionKeys)
}

func (s *sessionDialogCmp) SetSessions(sessions []session.Session) {
	s.allSessions = sessions
	// The mode must be reset before filtering: a search left over from a
	// previous opening would otherwise shrink the freshly loaded list while
	// the dialog renders in list mode.
	s.mode = sessionModeList
	s.sessions = s.filtered()
	s.pendingDelete = false

	if s.selectedSessionID != "" {
		for i, sess := range s.sessions {
			if sess.ID == s.selectedSessionID {
				s.selectedIdx = i
				return
			}
		}
	}
	s.selectedIdx = 0
}

func (s *sessionDialogCmp) SetSelectedSession(sessionID string) {
	s.selectedSessionID = sessionID
	if len(s.sessions) > 0 {
		for i, sess := range s.sessions {
			if sess.ID == sessionID {
				s.selectedIdx = i
				return
			}
		}
	}
}

// NewSessionDialogCmp creates a new session switching dialog
func NewSessionDialogCmp() SessionDialog {
	ti := textinput.New()
	ti.Placeholder = ""
	ti.CharLimit = 200
	return &sessionDialogCmp{
		allSessions:       []session.Session{},
		sessions:          []session.Session{},
		selectedIdx:       0,
		selectedSessionID: "",
		input:             ti,
	}
}
