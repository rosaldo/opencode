package chat

import (
	"fmt"
	"os"
	"os/exec"
	"slices"
	"strings"
	"unicode"

	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/bubbles/textarea"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/opencode-ai/opencode/internal/app"
	"github.com/opencode-ai/opencode/internal/config"
	"github.com/opencode-ai/opencode/internal/logging"
	"github.com/opencode-ai/opencode/internal/message"
	"github.com/opencode-ai/opencode/internal/promptstash"
	"github.com/opencode-ai/opencode/internal/session"
	"github.com/opencode-ai/opencode/internal/tui/components/dialog"
	"github.com/opencode-ai/opencode/internal/tui/layout"
	"github.com/opencode-ai/opencode/internal/tui/styles"
	"github.com/opencode-ai/opencode/internal/tui/theme"
	"github.com/opencode-ai/opencode/internal/tui/util"
)

type editorCmp struct {
	width       int
	height      int
	app         *app.App
	session     session.Session
	textarea    textarea.Model
	attachments []message.Attachment
	deleteMode  bool
	// shellMode is true while the prompt is composing a "!cmd" shell passthrough.
	shellMode bool
	// stash holds prompts parked by the prompt_stash bindings (Spec 09q).
	stash *promptstash.Store
	// history holds the prompts sent in this session (oldest first) for the
	// history_previous/history_next bindings (Spec 09f). historyIdx == len(history)
	// means the live draft is in the textarea; historyDraft keeps the draft the
	// user was typing when they started walking back.
	history      []string
	historyIdx   int
	historyDraft string
}

type EditorKeyMaps struct {
	Send            key.Binding
	OpenEditor      key.Binding
	Paste           key.Binding
	HistoryPrevious key.Binding
	HistoryNext     key.Binding
}

type bluredEditorKeyMaps struct {
	Send       key.Binding
	Focus      key.Binding
	OpenEditor key.Binding
}
type DeleteAttachmentKeyMaps struct {
	AttachmentDeleteMode key.Binding
	Escape               key.Binding
	DeleteAllAttachments key.Binding
}

var editorMaps = EditorKeyMaps{
	Send: key.NewBinding(
		key.WithKeys("enter", "ctrl+s"),
		key.WithHelp("enter", "send message"),
	),
	OpenEditor: key.NewBinding(
		key.WithKeys("ctrl+e"),
		key.WithHelp("ctrl+e", "open editor"),
	),
	Paste: key.NewBinding(
		key.WithKeys("ctrl+v"),
		key.WithHelp("ctrl+v", "paste image"),
	),
	HistoryPrevious: key.NewBinding(
		key.WithKeys("up"),
		key.WithHelp("up", "previous prompt"),
	),
	HistoryNext: key.NewBinding(
		key.WithKeys("down"),
		key.WithHelp("down", "next prompt"),
	),
}

var DeleteKeyMaps = DeleteAttachmentKeyMaps{
	AttachmentDeleteMode: key.NewBinding(
		key.WithKeys("ctrl+r"),
		key.WithHelp("ctrl+r+{i}", "delete attachment at index i"),
	),
	Escape: key.NewBinding(
		key.WithKeys("esc"),
		key.WithHelp("esc", "cancel delete mode"),
	),
	DeleteAllAttachments: key.NewBinding(
		key.WithKeys("r"),
		key.WithHelp("ctrl+r+r", "delete all attchments"),
	),
}

const (
	maxAttachments = 5
)

// SetPromptPasteKeybind replaces the paste binding so the key comes from
// `tui.keybinds.prompt_paste` instead of the hardcoded ctrl+v (Spec 09d).
func SetPromptPasteKeybind(bound []string) {
	if len(bound) == 0 {
		return
	}
	editorMaps.Paste = key.NewBinding(
		key.WithKeys(bound...),
		key.WithHelp(bound[0], "paste image"),
	)
}

// SetHistoryKeybinds replaces the prompt history bindings so the keys come from
// `tui.keybinds.history_previous`/`history_next` instead of the hardcoded
// up/down (Spec 09f). A disabled binding (empty list) is left unbound.
func SetHistoryKeybinds(previous, next []string) {
	if len(previous) > 0 {
		editorMaps.HistoryPrevious = key.NewBinding(
			key.WithKeys(previous...),
			key.WithHelp(previous[0], "previous prompt"),
		)
	} else {
		editorMaps.HistoryPrevious = key.NewBinding()
	}
	if len(next) > 0 {
		editorMaps.HistoryNext = key.NewBinding(
			key.WithKeys(next...),
			key.WithHelp(next[0], "next prompt"),
		)
	} else {
		editorMaps.HistoryNext = key.NewBinding()
	}
}

// recordHistory appends a sent prompt to this session's history, collapsing a
// repeat of the newest entry, and returns the prompt index to the live draft.
func (m *editorCmp) recordHistory(text string) {
	if text == "" {
		return
	}
	if len(m.history) > 0 && m.history[len(m.history)-1] == text {
		m.historyIdx = len(m.history)
		m.historyDraft = ""
		return
	}
	m.history = append(m.history, text)
	m.historyIdx = len(m.history)
	m.historyDraft = ""
}

// resetHistory drops the prompt history, e.g. when the session changes.
func (m *editorCmp) resetHistory() {
	m.history = nil
	m.historyIdx = 0
	m.historyDraft = ""
}

// canHistoryPrevious reports whether the history_previous key should walk back.
// While the live draft is shown it only applies to a single-line prompt: the
// bubbles textarea does not expose the cursor row, so on a multi-line prompt up
// must keep moving the cursor. Once browsing, further up always continues back
// even when the restored prompt is multi-line.
func (m *editorCmp) canHistoryPrevious() bool {
	if m.deleteMode || m.shellMode || len(m.history) == 0 || !m.textarea.Focused() {
		return false
	}
	if m.historyIdx < len(m.history) {
		return true
	}
	return m.textarea.LineCount() <= 1
}

// canHistoryNext reports whether the history_next key should walk forward.
func (m *editorCmp) canHistoryNext() bool {
	return !m.deleteMode && !m.shellMode && m.textarea.Focused() &&
		len(m.history) > 0 && m.historyIdx < len(m.history)
}

// historyPrevious shows the prompt sent before the one on screen, remembering
// the live draft the first time it moves away from it.
func (m *editorCmp) historyPrevious() tea.Cmd {
	if m.historyIdx >= len(m.history) {
		m.historyDraft = m.textarea.Value()
	}
	if m.historyIdx > 0 {
		m.historyIdx--
	}
	m.textarea.SetValue(m.history[m.historyIdx])
	m.textarea.CursorEnd()
	return nil
}

// historyNext walks towards the newest prompt and restores the live draft at
// the end of the history.
func (m *editorCmp) historyNext() tea.Cmd {
	if m.historyIdx >= len(m.history) {
		return nil
	}
	m.historyIdx++
	if m.historyIdx == len(m.history) {
		m.textarea.SetValue(m.historyDraft)
	} else {
		m.textarea.SetValue(m.history[m.historyIdx])
	}
	m.textarea.CursorEnd()
	return nil
}

// OpenEditorMsg asks the editor to open $EDITOR (the /editor command,
// Spec 09h).
type OpenEditorMsg struct{}

// RestorePromptMsg puts text back into the prompt textarea (Spec 09k: /undo
// returns the undone user message so it can be edited and resent). An
// existing draft is left untouched.
type RestorePromptMsg struct {
	Text string
}

func (m *editorCmp) openEditor() tea.Cmd {
	tmpfile, err := os.CreateTemp("", "msg_*.md")
	if err != nil {
		return util.ReportError(err)
	}
	tmpfile.Close()
	c := EditorCommand(tmpfile.Name())
	return tea.ExecProcess(c, func(err error) tea.Msg {
		if err != nil {
			return util.ReportError(err)
		}
		content, err := os.ReadFile(tmpfile.Name())
		if err != nil {
			return util.ReportError(err)
		}
		if len(content) == 0 {
			return util.ReportWarn("Message is empty")
		}
		os.Remove(tmpfile.Name())
		attachments := m.attachments
		m.attachments = nil
		clean, mentioned := m.mention(string(content))
		return SendMsg{
			Text:           clean,
			Attachments:    attachments,
			MentionedAgent: mentioned,
		}
	})
}

func (m *editorCmp) Init() tea.Cmd {
	return textarea.Blink
}

func (m *editorCmp) send() tea.Cmd {
	if m.app.Agents.IsSessionBusy(m.session.ID) {
		return util.ReportWarn("Agent is working, please wait...")
	}

	value := m.textarea.Value()
	// A shell passthrough ("!cmd") runs without clearing the busy guard above,
	// but still needs a session to attach its output to.
	parsed := ParseInput(value)
	if parsed.Kind == InputShell {
		m.textarea.Reset()
		m.shellMode = false
		if parsed.Command == "" {
			return nil
		}
		return util.CmdHandler(RunShellMsg{Command: parsed.Command})
	}

	m.textarea.Reset()
	attachments := m.attachments

	m.attachments = nil
	if value == "" {
		return nil
	}
	m.recordHistory(value)
	clean, mentioned := m.mention(value)
	return tea.Batch(
		util.CmdHandler(SendMsg{
			Text:           clean,
			Attachments:    attachments,
			MentionedAgent: mentioned,
		}),
	)
}

// mention extracts a leading "@name" token when it refers to a registered,
// enabled agent, returning the cleaned text and the matched name. A mention of
// an unknown name is left in the text untouched so it is sent as ordinary
// prose instead of being silently dropped.
func (m *editorCmp) mention(text string) (string, string) {
	clean, name := splitMention(text)
	if name == "" {
		return text, ""
	}
	if _, ok := m.app.Agents.SubagentLookup(name); !ok {
		return text, ""
	}
	return clean, name
}

// pasteClipboard attaches an image from the system clipboard, degrading to a
// text paste when no image is available (Spec 09).
func (m *editorCmp) pasteClipboard() tea.Msg {
	if content, mime, name, err := util.ClipboardImage(); err == nil && len(content) > 0 {
		if len(m.attachments) >= maxAttachments {
			return util.InfoMsg{Type: util.InfoTypeError, Msg: fmt.Sprintf("cannot add more than %d images", maxAttachments)}
		}
		m.attachments = append(m.attachments, message.Attachment{
			FilePath: name,
			FileName: name,
			MimeType: mime,
			Content:  content,
		})
		return nil
	}
	if text, err := util.ClipboardText(); err == nil && text != "" {
		m.textarea.SetValue(m.textarea.Value() + text)
		return nil
	}
	return util.InfoMsg{Type: util.InfoTypeWarn, Msg: util.ErrClipboardUnavailable.Error()}
}

func (m *editorCmp) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmd tea.Cmd
	switch msg := msg.(type) {
	case dialog.ThemeChangedMsg:
		m.textarea = CreateTextArea(&m.textarea)
	case dialog.CompletionSelectedMsg:
		existingValue := m.textarea.Value()
		modifiedValue := strings.Replace(existingValue, msg.SearchString, msg.CompletionValue, 1)

		m.textarea.SetValue(modifiedValue)
		return m, nil
	case SessionSelectedMsg:
		if msg.ID != m.session.ID {
			m.session = msg
			m.resetHistory()
		}
		return m, nil
	case SessionClearedMsg:
		m.resetHistory()
		return m, nil
	case OpenEditorMsg:
		if m.app != nil && m.app.Agents.IsSessionBusy(m.session.ID) {
			return m, util.ReportWarn("Agent is working, please wait...")
		}
		return m, m.openEditor()
	case RestorePromptMsg:
		if m.textarea.Value() != "" {
			return m, nil
		}
		m.textarea.SetValue(msg.Text)
		m.textarea.CursorEnd()
		return m, nil
	case StashPushMsg:
		return m.stashPush()
	case StashPopMsg:
		return m.stashPop()
	case RequestStashListMsg:
		return m.stashList()
	case StashTakeMsg:
		return m.stashTake(msg.Index)
	case StashRemoveMsg:
		m.stash.Remove(msg.Index)
		return m, nil
	case dialog.AttachmentAddedMsg:
		if len(m.attachments) >= maxAttachments {
			logging.ErrorPersist(fmt.Sprintf("cannot add more than %d images", maxAttachments))
			return m, cmd
		}
		m.attachments = append(m.attachments, msg.Attachment)
	case tea.KeyMsg:
		if key.Matches(msg, DeleteKeyMaps.AttachmentDeleteMode) {
			m.deleteMode = true
			return m, nil
		}
		if key.Matches(msg, DeleteKeyMaps.DeleteAllAttachments) && m.deleteMode {
			m.deleteMode = false
			m.attachments = nil
			return m, nil
		}
		if m.deleteMode && len(msg.Runes) > 0 && unicode.IsDigit(msg.Runes[0]) {
			num := int(msg.Runes[0] - '0')
			m.deleteMode = false
			if num < 10 && len(m.attachments) > num {
				if num == 0 {
					m.attachments = m.attachments[num+1:]
				} else {
					m.attachments = slices.Delete(m.attachments, num, num+1)
				}
				return m, nil
			}
		}
		if key.Matches(msg, messageKeys.PageUp) || key.Matches(msg, messageKeys.PageDown) ||
			key.Matches(msg, messageKeys.HalfPageUp) || key.Matches(msg, messageKeys.HalfPageDown) {
			return m, nil
		}
		if key.Matches(msg, editorMaps.OpenEditor) {
			if m.app.Agents.IsSessionBusy(m.session.ID) {
				return m, util.ReportWarn("Agent is working, please wait...")
			}
			return m, m.openEditor()
		}
		if key.Matches(msg, editorMaps.Paste) {
			return m, util.CmdHandler(m.pasteClipboard())
		}
		// Prompt history (Spec 09f): walk the prompts sent in this session.
		if m.canHistoryPrevious() && key.Matches(msg, editorMaps.HistoryPrevious) {
			return m, m.historyPrevious()
		}
		if m.canHistoryNext() && key.Matches(msg, editorMaps.HistoryNext) {
			return m, m.historyNext()
		}
		if key.Matches(msg, DeleteKeyMaps.Escape) {
			m.deleteMode = false
			if m.shellMode {
				m.shellMode = false
				return m, nil
			}
			return m, nil
		}
		// Trigger commands at the prompt position: "/" opens the command dialog,
		// "!" enters shell mode (Spec 09c). Only honored on an empty prompt so a
		// slash/bang typed mid-sentence stays ordinary text.
		if m.textarea.Focused() && m.textarea.Value() == "" && !m.deleteMode {
			if msg.Type == tea.KeyRunes && len(msg.Runes) == 1 {
				switch msg.Runes[0] {
				case '/':
					return m, util.CmdHandler(RunSlashCommandMsg{})
				case '!':
					m.shellMode = true
					return m, nil
				}
			}
		}
		// Handle Enter key
		if m.textarea.Focused() && key.Matches(msg, editorMaps.Send) {
			value := m.textarea.Value()
			if len(value) > 0 && value[len(value)-1] == '\\' {
				// If the last character is a backslash, remove it and add a newline
				m.textarea.SetValue(value[:len(value)-1] + "\n")
				return m, nil
			} else {
				// Otherwise, send the message
				return m, m.send()
			}
		}

	}
	m.textarea, cmd = m.textarea.Update(msg)
	return m, cmd
}

func (m *editorCmp) View() string {
	t := theme.CurrentTheme()

	// Style the prompt with theme colors
	style := lipgloss.NewStyle().
		Padding(0, 0, 0, 1).
		Bold(true).
		Foreground(t.Primary())

	prompt := ">"
	if m.shellMode {
		prompt = "!"
		style = style.Foreground(t.Warning())
	}

	if len(m.attachments) == 0 {
		return lipgloss.JoinHorizontal(lipgloss.Top, style.Render(prompt), m.textarea.View())
	}
	m.textarea.SetHeight(m.height - 1)
	return lipgloss.JoinVertical(lipgloss.Top,
		m.attachmentsContent(),
		lipgloss.JoinHorizontal(lipgloss.Top, style.Render(prompt),
			m.textarea.View()),
	)
}

func (m *editorCmp) SetSize(width, height int) tea.Cmd {
	m.width = width
	m.height = height
	m.textarea.SetWidth(width - 3) // account for the prompt and padding right
	m.textarea.SetHeight(height)
	m.textarea.SetWidth(width)
	return nil
}

func (m *editorCmp) GetSize() (int, int) {
	return m.textarea.Width(), m.textarea.Height()
}

func (m *editorCmp) attachmentsContent() string {
	var styledAttachments []string
	t := theme.CurrentTheme()
	attachmentStyles := styles.BaseStyle().
		MarginLeft(1).
		Background(t.TextMuted()).
		Foreground(t.Text())
	for i, attachment := range m.attachments {
		var filename string
		if len(attachment.FileName) > 10 {
			filename = fmt.Sprintf(" %s %s...", styles.DocumentIcon, attachment.FileName[0:7])
		} else {
			filename = fmt.Sprintf(" %s %s", styles.DocumentIcon, attachment.FileName)
		}
		if m.deleteMode {
			filename = fmt.Sprintf("%d%s", i, filename)
		}
		styledAttachments = append(styledAttachments, attachmentStyles.Render(filename))
	}
	content := lipgloss.JoinHorizontal(lipgloss.Left, styledAttachments...)
	return content
}

func (m *editorCmp) BindingKeys() []key.Binding {
	bindings := []key.Binding{}
	bindings = append(bindings, layout.KeyMapToSlice(editorMaps)...)
	bindings = append(bindings, layout.KeyMapToSlice(DeleteKeyMaps)...)
	return bindings
}

// EditorCommand builds the $EDITOR invocation for path, falling back to nvim,
// with the process attached to the terminal (Spec 09).
func EditorCommand(path string) *exec.Cmd {
	editor := os.Getenv("EDITOR")
	if editor == "" {
		editor = "nvim"
	}
	c := exec.Command(editor, path) //nolint:gosec
	c.Stdin = os.Stdin
	c.Stdout = os.Stdout
	c.Stderr = os.Stderr
	return c
}

func CreateTextArea(existing *textarea.Model) textarea.Model {
	t := theme.CurrentTheme()
	bgColor := t.Background()
	textColor := t.Text()
	textMutedColor := t.TextMuted()

	ta := textarea.New()
	ta.BlurredStyle.Base = styles.BaseStyle().Background(bgColor).Foreground(textColor)
	ta.BlurredStyle.CursorLine = styles.BaseStyle().Background(bgColor)
	ta.BlurredStyle.Placeholder = styles.BaseStyle().Background(bgColor).Foreground(textMutedColor)
	ta.BlurredStyle.Text = styles.BaseStyle().Background(bgColor).Foreground(textColor)
	ta.FocusedStyle.Base = styles.BaseStyle().Background(bgColor).Foreground(textColor)
	ta.FocusedStyle.CursorLine = styles.BaseStyle().Background(bgColor)
	ta.FocusedStyle.Placeholder = styles.BaseStyle().Background(bgColor).Foreground(textMutedColor)
	ta.FocusedStyle.Text = styles.BaseStyle().Background(bgColor).Foreground(textColor)

	ta.Prompt = " "
	ta.ShowLineNumbers = false
	ta.CharLimit = -1

	if existing != nil {
		ta.SetValue(existing.Value())
		ta.SetWidth(existing.Width())
		ta.SetHeight(existing.Height())
	}

	ta.Focus()
	return ta
}

func NewEditorCmp(app *app.App) tea.Model {
	ta := CreateTextArea(nil)
	// The stash persists next to the other state files; without a loaded
	// config (tests) it stays memory-only.
	var dir string
	if cfg := config.Get(); cfg != nil {
		dir = cfg.Data.Directory
	}
	return &editorCmp{
		app:      app,
		textarea: ta,
		stash:    promptstash.Open(dir),
	}
}
