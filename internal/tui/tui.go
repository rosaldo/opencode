package tui

import (
	"context"
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/key"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/opencode-ai/opencode/internal/app"
	"github.com/opencode-ai/opencode/internal/config"
	"github.com/opencode-ai/opencode/internal/llm/agent"
	"github.com/opencode-ai/opencode/internal/llm/models"
	"github.com/opencode-ai/opencode/internal/logging"
	"github.com/opencode-ai/opencode/internal/permission"
	"github.com/opencode-ai/opencode/internal/plugin"
	"github.com/opencode-ai/opencode/internal/promptstash"
	"github.com/opencode-ai/opencode/internal/pubsub"
	"github.com/opencode-ai/opencode/internal/session"
	"github.com/opencode-ai/opencode/internal/tui/components/chat"
	"github.com/opencode-ai/opencode/internal/tui/components/core"
	"github.com/opencode-ai/opencode/internal/tui/components/dialog"
	utilComponents "github.com/opencode-ai/opencode/internal/tui/components/util"
	"github.com/opencode-ai/opencode/internal/tui/layout"
	"github.com/opencode-ai/opencode/internal/tui/page"
	"github.com/opencode-ai/opencode/internal/tui/theme"
	"github.com/opencode-ai/opencode/internal/tui/util"
)

type keyMap struct {
	Leader         key.Binding
	Help           key.Binding
	Logs           key.Binding
	Quit           key.Binding
	SwitchSession  key.Binding
	Commands       key.Binding
	CommandPalette key.Binding
	SwitchAgent    key.Binding
	Models         key.Binding
	Filepicker     key.Binding
	SwitchTheme    key.Binding
}

type startCompactSessionMsg struct{}

// Built-in slash command messages (Spec 09h): a command handler cannot reach
// the app model, so it raises one of these instead and the model reacts.
type (
	showSessionListMsg struct{}
	showModelListMsg   struct{}
	showThemeListMsg   struct{}
	showHelpMsg        struct{}
	showQuitMsg        struct{}
	showTimelineMsg    struct{}
	showForkMsg        struct{}
)

// forkCompletedMsg / forkFailedMsg carry the async outcome of a fork request
// back into the model (Spec 09p).
type (
	forkCompletedMsg struct {
		session session.Session
		prompt  string
	}
	forkFailedMsg struct {
		err error
	}
)

// leaderTimeoutMsg fires when the leader sequence was not completed in time
// (Spec 09d). seq tags the tick so a stale one cannot cancel a newer sequence.
type leaderTimeoutMsg struct {
	seq uint64
}

const (
	quitKey = "q"

	// defaultLeaderTimeout is how long the TUI waits for the key that follows
	// the leader; `tui.leader_timeout` overrides it (Spec 09d).
	defaultLeaderTimeout = 2 * time.Second
)

var keys = keyMap{
	Leader: key.NewBinding(
		key.WithKeys(defaultLeader),
		key.WithHelp(defaultLeader, "leader"),
	),
	Logs: key.NewBinding(
		key.WithKeys("ctrl+l"),
		key.WithHelp("ctrl+l", "logs"),
	),

	Quit: key.NewBinding(
		key.WithKeys("ctrl+c"),
		key.WithHelp("ctrl+c", "quit"),
	),
	Help: key.NewBinding(
		key.WithKeys("ctrl+_", "ctrl+h"),
		key.WithHelp("ctrl+?", "toggle help"),
	),

	SwitchSession: key.NewBinding(
		key.WithKeys("ctrl+s"),
		key.WithHelp("ctrl+s", "switch session"),
	),

	SwitchAgent: key.NewBinding(
		key.WithKeys("tab", "ctrl+a"),
		key.WithHelp("tab", "switch agent"),
	),

	Commands: key.NewBinding(
		key.WithKeys("ctrl+k"),
		key.WithHelp("ctrl+k", "commands"),
	),
	Filepicker: key.NewBinding(
		key.WithKeys("ctrl+f"),
		key.WithHelp("ctrl+f", "select files to upload"),
	),
	Models: key.NewBinding(
		key.WithKeys("ctrl+o"),
		key.WithHelp("ctrl+o", "model selection"),
	),

	SwitchTheme: key.NewBinding(
		key.WithKeys("ctrl+t"),
		key.WithHelp("ctrl+t", "switch theme"),
	),
}

var helpEsc = key.NewBinding(
	key.WithKeys("?"),
	key.WithHelp("?", "toggle help"),
)

// resolvedKeybinds holds the keybinds resolved from config (Spec 09). It is set
// once by applyKeybinds and read by dialogs that need the extra session actions.
var resolvedKeybinds Keybinds

// disabledKey is a never-matching key used when a binding is disabled.
const disabledKey = "__disabled__"

// binding builds a key.Binding from the resolved keys of id, falling back to
// fallback when the id is unknown. A disabled binding uses disabledKey so it can
// never match.
func binding(id KeybindID, help string, fallback []string) key.Binding {
	keys := resolvedKeybinds.Keys(id)
	if keys == nil && fallback != nil {
		keys = fallback
	}
	helpKey := fallbackHelp(fallback, help)
	if len(keys) == 0 {
		keys = []string{disabledKey}
		helpKey = "none"
	} else {
		helpKey = keys[0]
	}
	return key.NewBinding(
		key.WithKeys(keys...),
		key.WithHelp(helpKey, help),
	)
}

func fallbackHelp(fallback []string, def string) string {
	if len(fallback) > 0 {
		return fallback[0]
	}
	return def
}

// applyKeybinds resolves the configured keybinds over the defaults and updates
// the package keymap. Unknown ids are ignored with a warning (Spec 09).
func applyKeybinds() {
	var configured map[string]string
	if cfg := config.Get(); cfg != nil {
		configured = cfg.TUI.Keybinds
	}
	kb, warnings := ResolveKeybinds(configured)
	for _, w := range warnings {
		logging.Warn(w)
	}
	resolvedKeybinds = kb
	keys = keyMap{
		Logs:           binding(KeybindLogsToggle, "logs", []string{"ctrl+l"}),
		Quit:           binding(KeybindAppExit, "quit", []string{"ctrl+c"}),
		Help:           binding(KeybindHelpToggle, "toggle help", []string{"ctrl+_", "ctrl+h"}),
		SwitchSession:  binding(KeybindSessionList, "switch session", []string{"ctrl+s"}),
		SwitchAgent:    binding(KeybindAgentList, "switch agent", []string{"tab", "ctrl+a"}),
		Commands:       binding(KeybindCommandList, "commands", []string{"ctrl+k"}),
		CommandPalette: binding(KeybindCommandPalette, "command palette", []string{"ctrl+p"}),
		Filepicker:     binding(KeybindFilePicker, "select files to upload", []string{"ctrl+f"}),
		Models:         binding(KeybindModelList, "model selection", []string{"ctrl+o"}),
		SwitchTheme:    binding(KeybindThemeList, "switch theme", []string{"ctrl+t"}),
	}
	// The leader prefix always comes from the configured "leader" keybind, and
	// the actions it prefixes are resolved from their own keybinds (Spec 09d).
	if leader := resolvedKeybinds.Leader; leader != "" {
		keys.Leader = key.NewBinding(key.WithKeys(leader), key.WithHelp(leader, "leader"))
	}
	applyLeaderBindings()
	// The editor owns the paste key and cannot see the package keymap.
	chat.SetPromptPasteKeybind(resolvedKeybinds.Keys(KeybindPromptPaste))
	// Same for the prompt history keys (Spec 09f).
	chat.SetHistoryKeybinds(resolvedKeybinds.Keys(KeybindHistoryPrev),
		resolvedKeybinds.Keys(KeybindHistoryNext))
}

// leaderAction is a TUI action addressable through the <leader> sequence
// (Spec 09d).
type leaderAction int

const (
	leaderNewSession leaderAction = iota
	leaderRenameSession
	leaderDeleteSession
	leaderCompactSession
	leaderToggleSidebar
	leaderExportSession
	leaderToggleThinking
	leaderCopyMessage
	leaderUndoMessage
	leaderRedoMessage
	leaderOpenEditor
	leaderToggleDetails
	leaderTimeline
	leaderCopySession
	leaderForkSession
	leaderMoveSession
	leaderStash
	leaderStashPop
	leaderStashList
)

// leaderSuffixes maps the keystroke pressed after the leader to its action;
// leaderDirect holds the same actions when they are bound without the leader.
// Both are rebuilt from the configured keybinds by applyLeaderBindings.
var (
	leaderSuffixes = map[string]leaderAction{}
	leaderDirect   = map[leaderAction]key.Binding{}
)

// applyLeaderBindings resolves the leader actions: a binding shaped
// ["<leader>x"] becomes a suffix, anything else becomes a direct key. Ids
// disabled with "none" leave both maps without an entry.
func applyLeaderBindings() {
	// The slice keeps the wiring deterministic: on a suffix conflict the
	// first action listed here wins and the loser is logged (Spec 09d).
	type actionID struct {
		action leaderAction
		id     KeybindID
	}
	pairs := []actionID{
		{leaderNewSession, KeybindSessionNew},
		{leaderRenameSession, KeybindSessionRename},
		{leaderDeleteSession, KeybindSessionDelete},
		{leaderCompactSession, KeybindSessionCompact},
		{leaderToggleSidebar, KeybindSidebarToggle},
		{leaderExportSession, KeybindSessionExport},
		{leaderToggleThinking, KeybindDisplayThink},
		{leaderCopyMessage, KeybindMessagesCopy},
		{leaderUndoMessage, KeybindMessagesUndo},
		{leaderRedoMessage, KeybindMessagesRedo},
		{leaderOpenEditor, KeybindEditorOpen},
		{leaderToggleDetails, KeybindToolDetails},
		{leaderTimeline, KeybindSessionTimeline},
		{leaderCopySession, KeybindSessionCopy},
		{leaderForkSession, KeybindSessionFork},
		{leaderMoveSession, KeybindSessionMove},
		{leaderStash, KeybindPromptStash},
		{leaderStashPop, KeybindPromptStashPop},
		{leaderStashList, KeybindPromptStashList},
	}
	leaderSuffixes = make(map[string]leaderAction, len(pairs))
	leaderDirect = make(map[leaderAction]key.Binding, len(pairs))
	for _, pair := range pairs {
		bound := resolvedKeybinds.Keys(pair.id)
		if len(bound) == 0 {
			continue
		}
		if len(bound) == 2 && bound[0] == resolvedKeybinds.Leader {
			if taken, dup := leaderSuffixes[bound[1]]; dup {
				logging.Warn(fmt.Sprintf("keybinds.%s: suffix %q is already used by another leader action, keeping the other one", pair.id, bound[1]),
					"taken_by", leaderHints[taken])
				continue
			}
			leaderSuffixes[bound[1]] = pair.action
			continue
		}
		leaderDirect[pair.action] = key.NewBinding(
			key.WithKeys(bound...),
			key.WithHelp(bound[0], string(pair.id)),
		)
	}
}

// leaderDirectAction reports the action bound directly, i.e. without the leader.
func leaderDirectAction(msg tea.KeyMsg) (leaderAction, bool) {
	for action, binding := range leaderDirect {
		if key.Matches(msg, binding) {
			return action, true
		}
	}
	return leaderNewSession, false
}

// beginLeader arms a leader sequence and invalidates any tick left over from a
// previous one (Spec 09d).
func (a *appModel) beginLeader() {
	a.leaderSeq++
	a.leaderPending = true
}

// cancelLeader drops a pending sequence, so its timeout tick stops mattering.
func (a *appModel) cancelLeader() {
	if a.leaderPending {
		a.leaderSeq++
	}
	a.leaderPending = false
}

// leaderTimeoutCmd fires leaderTimeoutMsg after tui.leader_timeout (default 2s).
func (a appModel) leaderTimeoutCmd() tea.Cmd {
	seq := a.leaderSeq
	return tea.Tick(leaderTimeoutFromConfig(), func(time.Time) tea.Msg {
		return leaderTimeoutMsg{seq: seq}
	})
}

// leaderTimeoutFromConfig resolves tui.leader_timeout in milliseconds, falling
// back to defaultLeaderTimeout when unset or invalid (Spec 09d).
func leaderTimeoutFromConfig() time.Duration {
	if cfg := config.Get(); cfg != nil && cfg.TUI.LeaderTimeout > 0 {
		return time.Duration(cfg.TUI.LeaderTimeout) * time.Millisecond
	}
	return defaultLeaderTimeout
}

// leaderHints labels the suffix each leader action reacts to (Spec 09d).
var leaderHints = map[leaderAction]string{
	leaderNewSession:     "new session",
	leaderRenameSession:  "rename session",
	leaderDeleteSession:  "delete session",
	leaderCompactSession: "compact session",
	leaderToggleSidebar:  "toggle sidebar",
	leaderExportSession:  "export session",
	leaderToggleThinking: "toggle thinking",
	leaderCopyMessage:    "copy message",
	leaderUndoMessage:    "undo message",
	leaderRedoMessage:    "redo message",
	leaderOpenEditor:     "open editor",
	leaderToggleDetails:  "toggle details",
	leaderTimeline:       "jump to message",
	leaderCopySession:    "copy transcript",
	leaderForkSession:    "fork session",
	leaderMoveSession:    "move session",
	leaderStash:          "stash prompt",
	leaderStashPop:       "pop stash",
	leaderStashList:      "list stash",
}

// leaderHintRows lists "suffix  label" pairs sorted by suffix: the which-key
// popup must not jump around, and map iteration is randomised.
func leaderHintRows() []string {
	suffixes := make([]string, 0, len(leaderSuffixes))
	for suffix := range leaderSuffixes {
		suffixes = append(suffixes, suffix)
	}
	slices.Sort(suffixes)

	rows := make([]string, 0, len(suffixes))
	for _, suffix := range suffixes {
		rows = append(rows, fmt.Sprintf("%-4s %s", suffix, leaderHints[leaderSuffixes[suffix]]))
	}
	if len(rows) == 0 {
		rows = append(rows, "no leader actions bound")
	}
	return rows
}

// inChatContext reports whether the chat keybinds own the keyboard: the chat
// page is up and no dialog is modal over it (Spec 09d).
func (a appModel) inChatContext() bool {
	return a.currentPage == page.ChatPage &&
		!a.showQuit && !a.showPermissions && !a.showCommandDialog &&
		!a.showSessionDialog && !a.showModelDialog && !a.showAgentDialog &&
		!a.showThemeDialog && !a.showTimelineDialog && !a.showFilepicker &&
		!a.showHelp && !a.showPluginsDialog && !a.showConnectDialog &&
		!a.showMultiArgumentsDialog && !a.showForkDialog && !a.showStashDialog &&
		!a.showMessageDialog && !a.showSessionMoveDialog
}

// runLeaderAction executes one leader action (Spec 09d).
func (a appModel) runLeaderAction(action leaderAction) (appModel, tea.Cmd) {
	switch action {
	case leaderNewSession:
		return a.updateChatPage(page.NewSessionMsg{})
	case leaderToggleSidebar:
		return a.updateChatPage(page.ToggleSidebarMsg{})
	case leaderRenameSession, leaderDeleteSession:
		// The session dialog owns search, rename (r) and delete (d + y); it
		// opens with the current session preselected.
		return a.openSessionDialogFor(a.selectedSession)
	case leaderCompactSession:
		return a, util.CmdHandler(startCompactSessionMsg{})
	case leaderExportSession:
		return a.exportSession()
	case leaderToggleThinking:
		return a.updateChatPage(chat.ToggleThinkingMsg{})
	case leaderCopyMessage:
		return a.updateChatPage(chat.CopyMessageMsg{})
	case leaderUndoMessage:
		return a.undoMessage()
	case leaderRedoMessage:
		return a.redoMessage()
	case leaderOpenEditor:
		return a.updateChatPage(chat.OpenEditorMsg{})
	case leaderToggleDetails:
		return a.updateChatPage(chat.ToggleDetailsMsg{})
	case leaderTimeline:
		return a.openTimelineDialog()
	case leaderCopySession:
		return a.copySession()
	case leaderForkSession:
		return a.openForkDialog()
	case leaderMoveSession:
		return a.openSessionMoveDialog()
	case leaderStash:
		return a.updateChatPage(chat.StashPushMsg{})
	case leaderStashPop:
		return a.updateChatPage(chat.StashPopMsg{})
	case leaderStashList:
		return a.updateChatPage(chat.RequestStashListMsg{})
	}
	return a, nil
}

// forwardMouseClick routes a left click to the open dialog, else to the
// current page (Spec 09n).
func (a *appModel) forwardMouseClick(msg tea.MouseMsg) tea.Cmd {
	if a.showSessionDialog {
		d, cmd := a.sessionDialog.Update(msg)
		a.sessionDialog = d.(dialog.SessionDialog)
		return cmd
	}
	if a.showCommandDialog {
		d, cmd := a.commandDialog.Update(msg)
		a.commandDialog = d.(dialog.CommandDialog)
		return cmd
	}
	if a.showModelDialog {
		d, cmd := a.modelDialog.Update(msg)
		a.modelDialog = d.(dialog.ModelDialog)
		return cmd
	}
	if a.showThemeDialog {
		d, cmd := a.themeDialog.Update(msg)
		a.themeDialog = d.(dialog.ThemeDialog)
		return cmd
	}
	if a.showTimelineDialog {
		d, cmd := a.timelineDialog.Update(msg)
		a.timelineDialog = d.(dialog.TimelineDialog)
		return cmd
	}
	if a.showMessageDialog {
		d, cmd := a.messageDialog.Update(msg)
		a.messageDialog = d.(dialog.MessageActionsDialog)
		return cmd
	}
	if a.showForkDialog {
		d, cmd := a.forkDialog.Update(msg)
		a.forkDialog = d.(dialog.ForkDialog)
		return cmd
	}
	if a.showSessionMoveDialog {
		d, cmd := a.sessionMoveDialog.Update(msg)
		a.sessionMoveDialog = d.(dialog.SessionMoveDialog)
		return cmd
	}
	if a.showStashDialog {
		d, cmd := a.stashDialog.Update(msg)
		a.stashDialog = d.(dialog.StashDialog)
		return cmd
	}
	if a.showAgentDialog {
		d, cmd := a.agentDialog.Update(msg)
		a.agentDialog = d.(dialog.AgentDialog)
		return cmd
	}
	if a.showPluginsDialog {
		d, cmd := a.pluginsDialog.Update(msg)
		a.pluginsDialog = d.(dialog.PluginsDialog)
		return cmd
	}
	var cmd tea.Cmd
	a.pages[a.currentPage], cmd = a.pages[a.currentPage].Update(msg)
	return cmd
}

// openSessionList loads every session and shows the switcher (ctrl+s and
// /sessions, Spec 09h).
func (a appModel) openSessionList() (appModel, tea.Cmd) {
	if a.currentPage != page.ChatPage || a.showQuit || a.showPermissions || a.showCommandDialog {
		return a, nil
	}
	sessions, err := a.app.Sessions.List(context.Background())
	if err != nil {
		return a, util.ReportError(err)
	}
	if len(sessions) == 0 {
		return a, util.ReportWarn("No sessions available")
	}
	a.sessionDialog.SetSessions(sessions)
	a.showSessionDialog = true
	return a, nil
}

// openModelDialog shows the model picker (ctrl+o and /models, Spec 09h).
func (a appModel) openModelDialog() (appModel, tea.Cmd) {
	if a.currentPage != page.ChatPage || a.showQuit || a.showPermissions ||
		a.showSessionDialog || a.showCommandDialog {
		return a, nil
	}
	a.showModelDialog = true
	// Re-init so the dialog reflects the active agent's model.
	return a, a.modelDialog.Init()
}

// openThemeDialog shows the theme switcher (ctrl+t and /themes, Spec 09h).
func (a appModel) openThemeDialog() (appModel, tea.Cmd) {
	if a.showQuit || a.showPermissions || a.showSessionDialog || a.showCommandDialog {
		return a, nil
	}
	a.showThemeDialog = true
	// The theme list is dynamically loaded by the dialog component.
	return a, a.themeDialog.Init()
}

// openTimelineDialog loads the user messages of the current session and shows
// the timeline picker (<leader>g and /timeline, Spec 09o).
func (a appModel) openTimelineDialog() (appModel, tea.Cmd) {
	if a.app == nil || a.selectedSession.ID == "" {
		return a, util.ReportWarn("No active session")
	}
	if a.showQuit || a.showPermissions || a.showSessionDialog || a.showCommandDialog {
		return a, nil
	}
	msgs, err := a.app.Messages.List(context.Background(), a.selectedSession.ID)
	if err != nil {
		return a, util.ReportError(err)
	}
	items := dialog.TimelineItems(msgs)
	if len(items) == 0 {
		return a, util.ReportWarn("No user messages in this session")
	}
	a.timelineDialog.SetItems(items)
	a.showTimelineDialog = true
	return a, a.timelineDialog.Init()
}

// openMessageActions loads the chosen message and replaces the timeline
// picker with the actions dialog (Spec 09u).
func (a appModel) openMessageActions(messageID string) (appModel, tea.Cmd) {
	if a.app == nil || a.selectedSession.ID == "" {
		return a, util.ReportWarn("No active session")
	}
	if a.showQuit || a.showPermissions || a.showSessionDialog || a.showCommandDialog {
		return a, nil
	}
	msg, err := a.app.Messages.Get(context.Background(), messageID)
	if err != nil {
		return a, util.ReportError(err)
	}
	a.showTimelineDialog = false
	a.messageDialog.SetMessage(&msg)
	a.showMessageDialog = true
	return a, a.messageDialog.Init()
}

// openForkDialog loads the current session messages and shows the fork picker
// (/fork, Spec 09p). The first row forks the full session; the remaining rows
// fork up to (excluding) the chosen user message.
// openSessionMoveDialog asks for a new working directory for the session
// (Spec 09v). Disabled while the agent is running to avoid moving the tree
// under an in-flight tool call.
func (a appModel) openSessionMoveDialog() (appModel, tea.Cmd) {
	if a.app == nil || a.selectedSession.ID == "" {
		return a, util.ReportWarn("No active session")
	}
	if a.app.Agents.IsSessionBusy(a.selectedSession.ID) {
		return a, util.ReportWarn("Agent is working, please wait...")
	}
	dir := a.selectedSession.WorkingDirectory
	if dir == "" {
		dir = config.WorkingDirectory()
	}
	a.sessionMoveDialog.SetDirectory(dir)
	a.showSessionMoveDialog = true
	return a, a.sessionMoveDialog.Init()
}

func (a appModel) openForkDialog() (appModel, tea.Cmd) {
	if a.app == nil || a.selectedSession.ID == "" {
		return a, util.ReportWarn("No active session")
	}
	if a.showQuit || a.showPermissions || a.showSessionDialog || a.showCommandDialog {
		return a, nil
	}
	msgs, err := a.app.Messages.List(context.Background(), a.selectedSession.ID)
	if err != nil {
		return a, util.ReportError(err)
	}
	if len(msgs) == 0 {
		return a, util.ReportWarn("Session has no messages to fork")
	}
	a.forkDialog.SetItems(dialog.ForkItems(msgs))
	a.showForkDialog = true
	return a, a.forkDialog.Init()
}

// openStashDialog shows the stash picker with the entries the editor reported
// (chat.StashListMsg, Spec 09q). The editor owns the stash, so the dialog is
// only opened from that reply.
func (a appModel) openStashDialog(entries []promptstash.Entry) (appModel, tea.Cmd) {
	if a.currentPage != page.ChatPage || a.showQuit || a.showPermissions ||
		a.showSessionDialog || a.showCommandDialog {
		return a, nil
	}
	a.stashDialog.SetItems(dialog.StashItems(entries))
	a.showStashDialog = true
	return a, a.stashDialog.Init()
}

// forkSession copies the current session up to (excluding) before into a new
// session and reports the result asynchronously (Spec 09p). The prompt text of
// the boundary message rides along so the composer can be prefilled with it.
func (a appModel) forkSession(before string) tea.Cmd {
	app := a.app
	parentID := a.selectedSession.ID
	return func() tea.Msg {
		ctx := context.Background()
		var prompt string
		if before != "" {
			msgs, err := app.Messages.List(ctx, parentID)
			if err != nil {
				return forkFailedMsg{err: err}
			}
			for _, m := range msgs {
				if m.ID == before {
					prompt = strings.TrimSpace(m.Content().Text)
					break
				}
			}
		}
		forked, err := app.Sessions.Fork(ctx, parentID, before)
		if err != nil {
			return forkFailedMsg{err: err}
		}
		return forkCompletedMsg{session: forked, prompt: prompt}
	}
}

// openHelp shows the help dialog (/help, Spec 09h).
func (a appModel) openHelp() (appModel, tea.Cmd) {
	a.showHelp = true
	return a, nil
}

// toggleQuit mirrors the quit key: it closes the other dialogs instead of
// quitting right away (ctrl+c and /exit, Spec 09h).
func (a appModel) toggleQuit() (appModel, tea.Cmd) {
	if a.showQuit {
		// A second press (ctrl+c twice) confirms the quit.
		return a, tea.Quit
	}
	a.showQuit = !a.showQuit
	a.showHelp = false
	a.showSessionDialog = false
	a.showCommandDialog = false
	a.showModelDialog = false
	a.showAgentDialog = false
	a.showTimelineDialog = false
	a.showMessageDialog = false
	a.showForkDialog = false
	a.showStashDialog = false
	a.showMultiArgumentsDialog = false
	if a.showFilepicker {
		a.showFilepicker = false
		a.filepicker.ToggleFilepicker(false)
	}
	return a, nil
}

// updateChatPage forwards a message to the chat page, keeping the returned model.
func (a appModel) updateChatPage(msg tea.Msg) (appModel, tea.Cmd) {
	m, cmd := a.pages[page.ChatPage].Update(msg)
	a.pages[page.ChatPage] = m
	return a, cmd
}

// openSessionDialogFor loads every session into the session dialog, preselecting
// current so <leader>r and <leader>d act on the session in view (Spec 09d).
func (a appModel) openSessionDialogFor(current session.Session) (appModel, tea.Cmd) {
	if a.app == nil {
		return a, util.ReportWarn("No sessions available")
	}
	sessions, err := a.app.Sessions.List(context.Background())
	if err != nil {
		return a, util.ReportError(err)
	}
	if len(sessions) == 0 {
		return a, util.ReportWarn("No sessions available")
	}
	a.sessionDialog.SetSessions(sessions)
	a.sessionDialog.SetSelectedSession(current.ID)
	a.showSessionDialog = true
	return a, nil
}

var returnKey = key.NewBinding(
	key.WithKeys("esc"),
	key.WithHelp("esc", "close"),
)

var logsKeyReturnKey = key.NewBinding(
	key.WithKeys("esc", "backspace", quitKey),
	key.WithHelp("esc/q", "go back"),
)

type appModel struct {
	width, height   int
	currentPage     page.PageID
	previousPage    page.PageID
	pages           map[page.PageID]tea.Model
	loadedPages     map[page.PageID]bool
	status          core.StatusCmp
	app             *app.App
	selectedSession session.Session

	showPermissions bool
	permissions     dialog.PermissionDialogCmp

	showHelp bool
	help     dialog.HelpCmp

	showQuit bool
	quit     dialog.QuitDialog

	showSessionDialog bool
	sessionDialog     dialog.SessionDialog

	showCommandDialog bool
	commandDialog     dialog.CommandDialog
	commands          []dialog.Command

	showModelDialog bool
	modelDialog     dialog.ModelDialog

	showAgentDialog bool
	agentDialog     dialog.AgentDialog

	showInitDialog bool
	initDialog     dialog.InitDialogCmp

	showFilepicker bool
	filepicker     dialog.FilepickerCmp

	showThemeDialog   bool
	themeDialog       dialog.ThemeDialog
	showPluginsDialog bool
	pluginsDialog     dialog.PluginsDialog

	showTimelineDialog bool
	timelineDialog     dialog.TimelineDialog

	showMessageDialog bool
	messageDialog     dialog.MessageActionsDialog

	showForkDialog bool
	forkDialog     dialog.ForkDialog

	showSessionMoveDialog bool
	sessionMoveDialog     dialog.SessionMoveDialog

	showStashDialog bool
	stashDialog     dialog.StashDialog

	showConnectDialog bool
	connectDialog     dialog.ConnectDialog

	showMultiArgumentsDialog bool
	multiArgumentsDialog     dialog.MultiArgumentsDialogCmp

	isCompacting      bool
	compactingMessage string
	leaderPending     bool
	// leaderSeq tags every leader transition so ticks from an abandoned
	// sequence cannot cancel the current one (Spec 09d).
	leaderSeq uint64
	// undone stacks the batches removed by /undo so /redo can restore them
	// (Spec 09k).
	undone []undoneBatch
	// revert is the staged revert (Spec 09u, parity with the TS
	// session.revert): set by /undo and by the timeline Revert action, cleared
	// by /redo (restores files and messages) or by the next turn (commits).
	revert *stagedRevert
}

func (a appModel) Init() tea.Cmd {
	var cmds []tea.Cmd
	cmd := a.pages[a.currentPage].Init()
	a.loadedPages[a.currentPage] = true
	cmds = append(cmds, cmd)
	cmd = a.status.Init()
	cmds = append(cmds, cmd)
	cmd = a.quit.Init()
	cmds = append(cmds, cmd)
	cmd = a.help.Init()
	cmds = append(cmds, cmd)
	cmd = a.sessionDialog.Init()
	cmds = append(cmds, cmd)
	cmd = a.commandDialog.Init()
	cmds = append(cmds, cmd)
	cmd = a.modelDialog.Init()
	cmds = append(cmds, cmd)
	cmd = a.initDialog.Init()
	cmds = append(cmds, cmd)
	cmd = a.filepicker.Init()
	cmds = append(cmds, cmd)
	cmd = a.themeDialog.Init()
	cmds = append(cmds, cmd)
	cmd = a.timelineDialog.Init()
	cmds = append(cmds, cmd)
	cmd = a.pluginsDialog.Init()
	cmds = append(cmds, cmd)
	cmd = a.connectDialog.Init()
	cmds = append(cmds, cmd)

	// Check if we should show the init dialog
	cmds = append(cmds, func() tea.Msg {
		shouldShow, err := config.ShouldShowInitDialog()
		if err != nil {
			return util.InfoMsg{
				Type: util.InfoTypeError,
				Msg:  "Failed to check init status: " + err.Error(),
			}
		}
		return dialog.ShowInitDialogMsg{Show: shouldShow}
	})

	return tea.Batch(cmds...)
}

func (a appModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmds []tea.Cmd
	var cmd tea.Cmd
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		msg.Height -= 1 // Make space for the status bar
		a.width, a.height = msg.Width, msg.Height

		s, _ := a.status.Update(msg)
		a.status = s.(core.StatusCmp)
		a.pages[a.currentPage], cmd = a.pages[a.currentPage].Update(msg)
		cmds = append(cmds, cmd)

		prm, permCmd := a.permissions.Update(msg)
		a.permissions = prm.(dialog.PermissionDialogCmp)
		cmds = append(cmds, permCmd)

		help, helpCmd := a.help.Update(msg)
		a.help = help.(dialog.HelpCmp)
		cmds = append(cmds, helpCmd)

		session, sessionCmd := a.sessionDialog.Update(msg)
		a.sessionDialog = session.(dialog.SessionDialog)
		cmds = append(cmds, sessionCmd)

		command, commandCmd := a.commandDialog.Update(msg)
		a.commandDialog = command.(dialog.CommandDialog)
		cmds = append(cmds, commandCmd)

		filepicker, filepickerCmd := a.filepicker.Update(msg)
		a.filepicker = filepicker.(dialog.FilepickerCmp)
		cmds = append(cmds, filepickerCmd)

		a.initDialog.SetSize(msg.Width, msg.Height)

		if a.showMultiArgumentsDialog {
			a.multiArgumentsDialog.SetSize(msg.Width, msg.Height)
			args, argsCmd := a.multiArgumentsDialog.Update(msg)
			a.multiArgumentsDialog = args.(dialog.MultiArgumentsDialogCmp)
			cmds = append(cmds, argsCmd, a.multiArgumentsDialog.Init())
		}

		return a, tea.Batch(cmds...)
	case tea.MouseMsg:
		if msg.Action != tea.MouseActionPress {
			return a, nil
		}
		// Wheel scroll is translated into up/down keys for the focused
		// component (Spec 09); left clicks are forwarded as-is so list
		// dialogs, the sidebar and the message zones can hit-test them
		// (Spec 09n).
		var keyMsg tea.KeyMsg
		switch msg.Button {
		case tea.MouseButtonWheelUp:
			keyMsg = tea.KeyMsg{Type: tea.KeyUp}
		case tea.MouseButtonWheelDown:
			keyMsg = tea.KeyMsg{Type: tea.KeyDown}
		case tea.MouseButtonLeft:
			return a, a.forwardMouseClick(msg)
		default:
			return a, nil
		}
		if a.showSessionDialog {
			d, cmd := a.sessionDialog.Update(keyMsg)
			a.sessionDialog = d.(dialog.SessionDialog)
			return a, cmd
		}
		if a.showCommandDialog {
			d, cmd := a.commandDialog.Update(keyMsg)
			a.commandDialog = d.(dialog.CommandDialog)
			return a, cmd
		}
		if a.showModelDialog {
			d, cmd := a.modelDialog.Update(keyMsg)
			a.modelDialog = d.(dialog.ModelDialog)
			return a, cmd
		}
		if a.showThemeDialog {
			d, cmd := a.themeDialog.Update(keyMsg)
			a.themeDialog = d.(dialog.ThemeDialog)
			return a, cmd
		}
		if a.showTimelineDialog {
			d, cmd := a.timelineDialog.Update(keyMsg)
			a.timelineDialog = d.(dialog.TimelineDialog)
			return a, cmd
		}
		if a.showMessageDialog {
			d, cmd := a.messageDialog.Update(keyMsg)
			a.messageDialog = d.(dialog.MessageActionsDialog)
			return a, cmd
		}
		if a.showForkDialog {
			d, cmd := a.forkDialog.Update(keyMsg)
			a.forkDialog = d.(dialog.ForkDialog)
			return a, cmd
		}
		if a.showSessionMoveDialog {
			d, cmd := a.sessionMoveDialog.Update(keyMsg)
			a.sessionMoveDialog = d.(dialog.SessionMoveDialog)
			return a, cmd
		}
		if a.showStashDialog {
			d, cmd := a.stashDialog.Update(keyMsg)
			a.stashDialog = d.(dialog.StashDialog)
			return a, cmd
		}
		if a.showAgentDialog {
			d, cmd := a.agentDialog.Update(keyMsg)
			a.agentDialog = d.(dialog.AgentDialog)
			return a, cmd
		}
		if a.showPluginsDialog {
			d, cmd := a.pluginsDialog.Update(keyMsg)
			a.pluginsDialog = d.(dialog.PluginsDialog)
			return a, cmd
		}
		a.pages[a.currentPage], cmd = a.pages[a.currentPage].Update(keyMsg)
		return a, cmd
	// Status
	case util.InfoMsg:
		s, cmd := a.status.Update(msg)
		a.status = s.(core.StatusCmp)
		cmds = append(cmds, cmd)
		return a, tea.Batch(cmds...)
	case pubsub.Event[logging.LogMessage]:
		if msg.Payload.Persist {
			switch msg.Payload.Level {
			case "error":
				s, cmd := a.status.Update(util.InfoMsg{
					Type: util.InfoTypeError,
					Msg:  msg.Payload.Message,
					TTL:  msg.Payload.PersistTime,
				})
				a.status = s.(core.StatusCmp)
				cmds = append(cmds, cmd)
			case "info":
				s, cmd := a.status.Update(util.InfoMsg{
					Type: util.InfoTypeInfo,
					Msg:  msg.Payload.Message,
					TTL:  msg.Payload.PersistTime,
				})
				a.status = s.(core.StatusCmp)
				cmds = append(cmds, cmd)

			case "warn":
				s, cmd := a.status.Update(util.InfoMsg{
					Type: util.InfoTypeWarn,
					Msg:  msg.Payload.Message,
					TTL:  msg.Payload.PersistTime,
				})

				a.status = s.(core.StatusCmp)
				cmds = append(cmds, cmd)
			default:
				s, cmd := a.status.Update(util.InfoMsg{
					Type: util.InfoTypeInfo,
					Msg:  msg.Payload.Message,
					TTL:  msg.Payload.PersistTime,
				})
				a.status = s.(core.StatusCmp)
				cmds = append(cmds, cmd)
			}
		}
	case util.ClearStatusMsg:
		s, _ := a.status.Update(msg)
		a.status = s.(core.StatusCmp)

	// Permission
	case pubsub.Event[permission.PermissionRequest]:
		a.showPermissions = true
		return a, a.permissions.SetPermissions(msg.Payload)
	case dialog.PermissionResponseMsg:
		var cmd tea.Cmd
		switch msg.Action {
		case dialog.PermissionAllow:
			a.app.Permissions.Grant(msg.Permission)
		case dialog.PermissionAllowForSession:
			a.app.Permissions.GrantPersistant(msg.Permission)
		case dialog.PermissionDeny:
			a.app.Permissions.Deny(msg.Permission)
		}
		a.showPermissions = false
		return a, cmd

	case page.PageChangeMsg:
		return a, a.moveToPage(msg.ID)

	case dialog.CloseQuitMsg:
		a.showQuit = false
		return a, nil

	case dialog.CloseSessionDialogMsg:
		a.showSessionDialog = false
		return a, nil

	case dialog.CloseCommandDialogMsg:
		a.showCommandDialog = false
		return a, nil

	case chat.RunSlashCommandMsg:
		return a, a.openCommandDialog(msg.Name)

	// A fresh session clears the app-level selection so <leader>c (compact)
	// reports "no active session" instead of summarising the previous one.
	case chat.SessionClearedMsg:
		a.selectedSession = session.Session{}

	case leaderTimeoutMsg:
		// A stale tick (seq mismatch) belongs to a sequence that already ended.
		if a.leaderPending && msg.seq == a.leaderSeq {
			a.leaderPending = false
		}
		return a, nil

	case exportSessionMsg:
		return a.exportSession()

	case copySessionMsg:
		return a.copySession()

	case openSessionMoveDialogMsg:
		return a.openSessionMoveDialog()

	case undoMessageMsg:
		return a.undoMessage()

	case redoMessageMsg:
		return a.redoMessage()

	case chat.CommitRevertMsg:
		// A new turn makes a staged revert permanent (Spec 09u).
		return a.commitRevert()

	case utilComponents.ListActivatedMsg:
		// SimpleList clicks are relayed back to the owning dialog (Spec 09n).
		if a.showCommandDialog {
			d, cmd := a.commandDialog.Update(msg)
			a.commandDialog = d.(dialog.CommandDialog)
			return a, cmd
		}
		if a.showTimelineDialog {
			d, cmd := a.timelineDialog.Update(msg)
			a.timelineDialog = d.(dialog.TimelineDialog)
			return a, cmd
		}
		if a.showMessageDialog {
			d, cmd := a.messageDialog.Update(msg)
			a.messageDialog = d.(dialog.MessageActionsDialog)
			return a, cmd
		}
		if a.showForkDialog {
			d, cmd := a.forkDialog.Update(msg)
			a.forkDialog = d.(dialog.ForkDialog)
			return a, cmd
		}
		if a.showSessionMoveDialog {
			d, cmd := a.sessionMoveDialog.Update(msg)
			a.sessionMoveDialog = d.(dialog.SessionMoveDialog)
			return a, cmd
		}
		if a.showStashDialog {
			d, cmd := a.stashDialog.Update(msg)
			a.stashDialog = d.(dialog.StashDialog)
			return a, cmd
		}
		return a, nil

	case chat.ShowSessionListMsg:
		// The sidebar's session title opens the switcher (Spec 09n).
		return a.openSessionList()

	// Built-in slash commands (Spec 09h).
	case showSessionListMsg:
		return a.openSessionList()
	case showModelListMsg:
		return a.openModelDialog()
	case showThemeListMsg:
		return a.openThemeDialog()
	case showHelpMsg:
		return a.openHelp()
	case showQuitMsg:
		return a.toggleQuit()
	case showTimelineMsg:
		return a.openTimelineDialog()
	case showForkMsg:
		return a.openForkDialog()

	case dialog.CloseForkDialogMsg:
		a.showForkDialog = false
		return a, nil
	case dialog.CloseSessionMoveDialogMsg:
		a.showSessionMoveDialog = false
		return a, nil

	case dialog.SessionMoveMsg:
		a.showSessionMoveDialog = false
		if a.app == nil || a.selectedSession.ID == "" {
			return a, util.ReportWarn("No active session")
		}
		current := a.selectedSession.WorkingDirectory
		if current == "" {
			current = config.WorkingDirectory()
		}
		dir := dialog.ResolveDirectoryInput(msg.Directory, current)
		updated, err := a.app.Sessions.SetWorkingDirectory(context.Background(), a.selectedSession.ID, dir)
		if err != nil {
			return a, util.ReportError(err)
		}
		a.selectedSession = updated
		where := updated.WorkingDirectory
		if where == "" {
			where = config.WorkingDirectory()
		}
		return a, tea.Batch(
			util.CmdHandler(chat.SessionSelectedMsg(updated)),
			util.ReportInfo("Session moved to "+where),
		)

	case dialog.ForkMsg:
		// Run the fork off-model; the dialog shows a pending state until the
		// outcome arrives (Spec 09p).
		if a.app == nil || a.selectedSession.ID == "" {
			a.showForkDialog = false
			return a, util.ReportWarn("No active session")
		}
		a.forkDialog.SetPending(true)
		return a, a.forkSession(msg.MessageID)
	case forkCompletedMsg:
		a.showForkDialog = false
		cmds := []tea.Cmd{
			util.CmdHandler(chat.SessionSelectedMsg(msg.session)),
			util.ReportInfo("Forked session"),
		}
		if msg.prompt != "" {
			cmds = append(cmds, util.CmdHandler(chat.RestorePromptMsg{Text: msg.prompt}))
		}
		return a, tea.Batch(cmds...)
	case forkFailedMsg:
		a.showForkDialog = false
		return a, util.ReportError(msg.err)

	case dialog.CloseTimelineDialogMsg:
		a.showTimelineDialog = false
		return a, nil
	case dialog.TimelinePreviewMsg:
		// Scroll behind the open dialog without closing it (Spec 09o).
		return a.updateChatPage(chat.JumpToMessageMsg{MessageID: msg.MessageID})

	// Message actions (Spec 09u).
	case dialog.OpenMessageActionsMsg:
		return a.openMessageActions(msg.MessageID)
	case dialog.CloseMessageActionsMsg:
		a.showMessageDialog = false
		return a, nil
	case dialog.MessageActionMsg:
		a.showMessageDialog = false
		switch msg.Action {
		case dialog.MessageActionJump:
			// Close the dialog and scroll the transcript to the message.
			return a.updateChatPage(chat.JumpToMessageMsg{MessageID: msg.MessageID})
		case dialog.MessageActionRevert:
			return a.revertToMessage(msg.MessageID)
		case dialog.MessageActionCopy:
			return a.copyMessageByID(msg.MessageID)
		case dialog.MessageActionFork:
			// Replace the actions dialog with the fork picker preselected
			// on the message (Spec 09u).
			forked, cmd := a.openForkDialog()
			forked.forkDialog.SelectMessage(msg.MessageID)
			return forked, cmd
		}
		return a, nil

	// Prompt stash (Spec 09q).
	case chat.StashListMsg:
		return a.openStashDialog(msg.Entries)
	case dialog.CloseStashDialogMsg:
		a.showStashDialog = false
		return a, nil
	case dialog.StashSelectMsg:
		// Restore overwrites the composer and drops the entry (Spec 09q).
		a.showStashDialog = false
		return a.updateChatPage(chat.StashTakeMsg{Index: msg.Index})
	case dialog.StashDeleteMsg:
		// Deleting keeps the dialog open (Spec 09q).
		return a.updateChatPage(chat.StashRemoveMsg{Index: msg.Index})

	case startCompactSessionMsg:
		// Start compacting the current session
		a.isCompacting = true
		a.compactingMessage = "Starting summarization..."

		if a.selectedSession.ID == "" {
			a.isCompacting = false
			return a, util.ReportWarn("No active session to summarize")
		}

		// Start the summarization process
		return a, func() tea.Msg {
			ctx := context.Background()
			a.app.Agents.Summarize(ctx, a.selectedSession.ID)
			return nil
		}

	case pubsub.Event[agent.AgentEvent]:
		payload := msg.Payload
		if payload.Error != nil {
			a.isCompacting = false
			return a, util.ReportError(payload.Error)
		}

		a.compactingMessage = payload.Progress

		if payload.Done && payload.Type == agent.AgentEventTypeSummarize {
			a.isCompacting = false
			return a, util.ReportInfo("Session summarization complete")
		} else if payload.Done && payload.Type == agent.AgentEventTypeResponse && a.selectedSession.ID != "" {
			model := a.app.Agents.Model()
			contextWindow := model.ContextWindow
			tokens := a.selectedSession.CompletionTokens + a.selectedSession.PromptTokens
			if (tokens >= int64(float64(contextWindow)*0.95)) && config.Get().AutoCompact {
				return a, util.CmdHandler(startCompactSessionMsg{})
			}
		}
		// Continue listening for events
		return a, nil

	case dialog.CloseThemeDialogMsg:
		a.showThemeDialog = false
		return a, nil

	case dialog.ShowPluginsDialogMsg:
		a.pluginsDialog.SetPlugins(msg.Plugins)
		a.showPluginsDialog = true
		return a, a.pluginsDialog.Init()

	case dialog.ClosePluginsDialogMsg:
		a.showPluginsDialog = false
		return a, nil

	case dialog.ShowConnectDialogMsg:
		a.connectDialog.SetProvider(msg.Provider)
		a.showConnectDialog = true
		return a, a.connectDialog.Init()

	case dialog.CloseConnectDialogMsg:
		a.showConnectDialog = false
		return a, nil

	case dialog.SubmitConnectMsg:
		a.showConnectDialog = false
		if err := config.SetProviderAPIKey(msg.Provider, msg.APIKey); err != nil {
			return a, util.ReportError(err)
		}
		return a, util.ReportInfo("Connected " + string(msg.Provider))

	case dialog.ThemeChangedMsg:
		a.pages[a.currentPage], cmd = a.pages[a.currentPage].Update(msg)
		a.showThemeDialog = false
		return a, tea.Batch(cmd, util.ReportInfo("Theme changed to: "+msg.ThemeName))

	case dialog.CloseModelDialogMsg:
		a.showModelDialog = false
		return a, nil

	case dialog.ModelSelectedMsg:
		a.showModelDialog = false

		model, err := a.app.Agents.Update("", msg.Model.ID)
		if err != nil {
			return a, util.ReportError(err)
		}

		return a, util.ReportInfo(fmt.Sprintf("Model changed to %s for agent %s", model.Name, a.app.Agents.Active()))

	case dialog.AgentSelectedMsg:
		a.showAgentDialog = false
		if err := a.app.Agents.Switch(msg.Name); err != nil {
			return a, util.ReportError(err)
		}
		return a, util.ReportInfo(fmt.Sprintf("Agent switched to %s", msg.Name))

	case dialog.CloseAgentsDialogMsg:
		a.showAgentDialog = false
		return a, nil

	case dialog.ShowInitDialogMsg:
		a.showInitDialog = msg.Show
		return a, nil

	case dialog.CloseInitDialogMsg:
		a.showInitDialog = false
		if msg.Initialize {
			// Run the initialization command
			for _, cmd := range a.commands {
				if cmd.ID == "init" {
					// Mark the project as initialized
					if err := config.MarkProjectInitialized(); err != nil {
						return a, util.ReportError(err)
					}
					return a, cmd.Handler(cmd)
				}
			}
		} else {
			// Mark the project as initialized without running the command
			if err := config.MarkProjectInitialized(); err != nil {
				return a, util.ReportError(err)
			}
		}
		return a, nil

	case chat.SessionSelectedMsg:
		a.selectedSession = msg
		a.sessionDialog.SetSelectedSession(msg.ID)
		// The chat page (and the message list) must also see the selection,
		// otherwise the transcript keeps showing the previous session.
		var cmds []tea.Cmd
		if a.currentPage == page.ChatPage {
			var cmd tea.Cmd
			a, cmd = a.updateChatPage(msg)
			cmds = append(cmds, cmd)
		}
		// Re-show the staged-revert banner when returning to its session
		// (Spec 09u); other sessions hide it.
		if a.revert != nil && a.revert.sessionID == msg.ID {
			cmds = append(cmds, util.CmdHandler(chat.RevertBannerMsg{Text: a.revert.bannerText()}))
		} else {
			cmds = append(cmds, util.CmdHandler(chat.RevertBannerMsg{Text: ""}))
		}
		return a, tea.Batch(cmds...)

	case pubsub.Event[session.Session]:
		if msg.Type == pubsub.UpdatedEvent && msg.Payload.ID == a.selectedSession.ID {
			a.selectedSession = msg.Payload
		}
	case dialog.SessionSelectedMsg:
		a.showSessionDialog = false
		if a.currentPage == page.ChatPage {
			return a, util.CmdHandler(chat.SessionSelectedMsg(msg.Session))
		}
		return a, nil

	case dialog.SessionRenameRequestMsg:
		sess := msg.Session
		sess.Title = msg.Title
		if _, err := a.app.Sessions.Save(context.Background(), sess); err != nil {
			return a, util.ReportError(err)
		}
		sessions, err := a.app.Sessions.List(context.Background())
		if err != nil {
			return a, util.ReportError(err)
		}
		a.sessionDialog.SetSessions(sessions)
		a.sessionDialog.SetSelectedSession(sess.ID)
		return a, nil

	case dialog.SessionDeleteRequestMsg:
		if err := a.app.Sessions.Delete(context.Background(), msg.Session.ID); err != nil {
			return a, util.ReportError(err)
		}
		sessions, err := a.app.Sessions.List(context.Background())
		if err != nil {
			return a, util.ReportError(err)
		}
		a.sessionDialog.SetSessions(sessions)
		return a, nil

	case dialog.CommandSelectedMsg:
		a.showCommandDialog = false
		// Execute the command handler if available
		if msg.Command.Handler != nil {
			return a, msg.Command.Handler(msg.Command)
		}
		return a, util.ReportInfo("Command selected: " + msg.Command.Title)

	case dialog.ShowMultiArgumentsDialogMsg:
		// Show multi-arguments dialog
		a.multiArgumentsDialog = dialog.NewMultiArgumentsDialogCmp(msg.CommandID, msg.Content, msg.ArgNames)
		a.showMultiArgumentsDialog = true
		return a, a.multiArgumentsDialog.Init()

	case dialog.CloseMultiArgumentsDialogMsg:
		// Close multi-arguments dialog
		a.showMultiArgumentsDialog = false

		// If submitted, replace all named arguments and run the command
		if msg.Submit {
			content := msg.Content

			// Replace each named argument with its value
			for name, value := range msg.Args {
				placeholder := "$" + name
				content = strings.ReplaceAll(content, placeholder, value)
			}

			// Execute the command with arguments
			return a, util.CmdHandler(dialog.CommandRunCustomMsg{
				Content: content,
				Args:    msg.Args,
			})
		}
		return a, nil

	case tea.KeyMsg:
		// If multi-arguments dialog is open, let it handle the key press first
		if a.showMultiArgumentsDialog {
			args, cmd := a.multiArgumentsDialog.Update(msg)
			a.multiArgumentsDialog = args.(dialog.MultiArgumentsDialogCmp)
			return a, cmd
		}

		// A leader sequence only starts while the chat page is key-owner and
		// no dialog is modal over it (Spec 09d).
		if a.leaderPending && !a.inChatContext() {
			a.cancelLeader()
		}
		if a.inChatContext() {
			if a.leaderPending {
				if key.Matches(msg, keys.Quit) {
					// ctrl+c must keep working while a sequence is pending.
					a.cancelLeader()
				} else {
					action, ok := leaderSuffixes[msg.String()]
					a.cancelLeader()
					if ok {
						return a.runLeaderAction(action)
					}
					// Any other key cancels the leader sequence but must not be swallowed.
				}
			}
			if key.Matches(msg, keys.Leader) {
				a.beginLeader()
				return a, a.leaderTimeoutCmd()
			}
			if action, ok := leaderDirectAction(msg); ok {
				return a.runLeaderAction(action)
			}
		}

		switch {

		case key.Matches(msg, keys.Quit):
			return a.toggleQuit()
		case key.Matches(msg, keys.SwitchSession):
			return a.openSessionList()
		case key.Matches(msg, keys.Commands):
			if a.currentPage == page.ChatPage && !a.showQuit && !a.showPermissions && !a.showSessionDialog && !a.showThemeDialog && !a.showFilepicker {
				return a, a.openCommandDialog("")
			}
			return a, nil
		case key.Matches(msg, keys.CommandPalette):
			if a.currentPage == page.ChatPage && !a.showQuit && !a.showPermissions && !a.showSessionDialog && !a.showThemeDialog && !a.showFilepicker {
				return a, a.openCommandDialog("")
			}
			return a, nil
		case key.Matches(msg, keys.Models):
			if a.showModelDialog {
				a.showModelDialog = false
				return a, nil
			}
			return a.openModelDialog()
		case key.Matches(msg, keys.SwitchAgent):
			if a.showAgentDialog {
				a.showAgentDialog = false
				return a, nil
			}
			if a.currentPage == page.ChatPage && !a.showQuit && !a.showPermissions && !a.showSessionDialog && !a.showCommandDialog && !a.showModelDialog && !a.showThemeDialog && !a.showFilepicker {
				a.showAgentDialog = true
				return a, a.agentDialog.Init()
			}
			return a, nil
		case key.Matches(msg, keys.SwitchTheme):
			return a.openThemeDialog()
		case key.Matches(msg, returnKey) || key.Matches(msg):
			if msg.String() == quitKey {
				if a.currentPage == page.LogsPage {
					return a, a.moveToPage(page.ChatPage)
				}
			} else if !a.filepicker.IsCWDFocused() {
				if a.showQuit {
					a.showQuit = !a.showQuit
					return a, nil
				}
				if a.showHelp {
					a.showHelp = !a.showHelp
					return a, nil
				}
				if a.showInitDialog {
					a.showInitDialog = false
					// Mark the project as initialized without running the command
					if err := config.MarkProjectInitialized(); err != nil {
						return a, util.ReportError(err)
					}
					return a, nil
				}
				if a.showFilepicker {
					a.showFilepicker = false
					a.filepicker.ToggleFilepicker(a.showFilepicker)
					return a, nil
				}
				if a.currentPage == page.LogsPage {
					return a, a.moveToPage(page.ChatPage)
				}
			}
		case key.Matches(msg, keys.Logs):
			return a, a.moveToPage(page.LogsPage)
		case key.Matches(msg, keys.Help):
			if a.showQuit {
				return a, nil
			}
			a.showHelp = !a.showHelp
			return a, nil
		case key.Matches(msg, helpEsc):
			if a.app.Agents.IsBusy() {
				if a.showQuit {
					return a, nil
				}
				a.showHelp = !a.showHelp
				return a, nil
			}
		case key.Matches(msg, keys.Filepicker):
			a.showFilepicker = !a.showFilepicker
			a.filepicker.ToggleFilepicker(a.showFilepicker)
			return a, nil
		}
	default:
		if a.showFilepicker {
			f, filepickerCmd := a.filepicker.Update(msg)
			a.filepicker = f.(dialog.FilepickerCmp)
			cmds = append(cmds, filepickerCmd)
			if a.filepicker.IsCWDFocused() {
				return a, tea.Batch(cmds...)
			}
		}

	}

	if a.showFilepicker {
		f, filepickerCmd := a.filepicker.Update(msg)
		a.filepicker = f.(dialog.FilepickerCmp)
		cmds = append(cmds, filepickerCmd)
		// Only block key messages send all other messages down
		if _, ok := msg.(tea.KeyMsg); ok {
			return a, tea.Batch(cmds...)
		}
		if a.filepicker.IsCWDFocused() {
			return a, tea.Batch(cmds...)
		}
	}

	if a.showQuit {
		q, quitCmd := a.quit.Update(msg)
		a.quit = q.(dialog.QuitDialog)
		cmds = append(cmds, quitCmd)
		// Only block key messages send all other messages down
		if _, ok := msg.(tea.KeyMsg); ok {
			return a, tea.Batch(cmds...)
		}
	}
	if a.showPermissions {
		d, permissionsCmd := a.permissions.Update(msg)
		a.permissions = d.(dialog.PermissionDialogCmp)
		cmds = append(cmds, permissionsCmd)
		// Only block key messages send all other messages down
		if _, ok := msg.(tea.KeyMsg); ok {
			return a, tea.Batch(cmds...)
		}
	}

	if a.showSessionDialog {
		d, sessionCmd := a.sessionDialog.Update(msg)
		a.sessionDialog = d.(dialog.SessionDialog)
		cmds = append(cmds, sessionCmd)
		// Only block key messages send all other messages down
		if _, ok := msg.(tea.KeyMsg); ok {
			return a, tea.Batch(cmds...)
		}
	}

	if a.showCommandDialog {
		d, commandCmd := a.commandDialog.Update(msg)
		a.commandDialog = d.(dialog.CommandDialog)
		cmds = append(cmds, commandCmd)
		// Only block key messages send all other messages down
		if _, ok := msg.(tea.KeyMsg); ok {
			return a, tea.Batch(cmds...)
		}
	}

	if a.showModelDialog {
		d, modelCmd := a.modelDialog.Update(msg)
		a.modelDialog = d.(dialog.ModelDialog)
		cmds = append(cmds, modelCmd)
		// Only block key messages send all other messages down
		if _, ok := msg.(tea.KeyMsg); ok {
			return a, tea.Batch(cmds...)
		}
	}

	if a.showAgentDialog {
		d, agentCmd := a.agentDialog.Update(msg)
		a.agentDialog = d.(dialog.AgentDialog)
		cmds = append(cmds, agentCmd)
		// Only block key messages send all other messages down
		if _, ok := msg.(tea.KeyMsg); ok {
			return a, tea.Batch(cmds...)
		}
	}

	if a.showInitDialog {
		d, initCmd := a.initDialog.Update(msg)
		a.initDialog = d.(dialog.InitDialogCmp)
		cmds = append(cmds, initCmd)
		// Only block key messages send all other messages down
		if _, ok := msg.(tea.KeyMsg); ok {
			return a, tea.Batch(cmds...)
		}
	}

	if a.showThemeDialog {
		d, themeCmd := a.themeDialog.Update(msg)
		a.themeDialog = d.(dialog.ThemeDialog)
		cmds = append(cmds, themeCmd)
		// Only block key messages send all other messages down
		if _, ok := msg.(tea.KeyMsg); ok {
			return a, tea.Batch(cmds...)
		}
	}

	if a.showTimelineDialog {
		d, timelineCmd := a.timelineDialog.Update(msg)
		a.timelineDialog = d.(dialog.TimelineDialog)
		cmds = append(cmds, timelineCmd)
		// Only block key messages send all other messages down
		if _, ok := msg.(tea.KeyMsg); ok {
			return a, tea.Batch(cmds...)
		}
	}

	if a.showMessageDialog {
		d, messageCmd := a.messageDialog.Update(msg)
		a.messageDialog = d.(dialog.MessageActionsDialog)
		cmds = append(cmds, messageCmd)
		// Only block key messages send all other messages down
		if _, ok := msg.(tea.KeyMsg); ok {
			return a, tea.Batch(cmds...)
		}
	}

	if a.showForkDialog {
		d, forkCmd := a.forkDialog.Update(msg)
		a.forkDialog = d.(dialog.ForkDialog)
		cmds = append(cmds, forkCmd)
		// Only block key messages send all other messages down
		if _, ok := msg.(tea.KeyMsg); ok {
			return a, tea.Batch(cmds...)
		}
	}

	if a.showSessionMoveDialog {
		d, moveCmd := a.sessionMoveDialog.Update(msg)
		a.sessionMoveDialog = d.(dialog.SessionMoveDialog)
		cmds = append(cmds, moveCmd)
		// Only block key messages send all other messages down
		if _, ok := msg.(tea.KeyMsg); ok {
			return a, tea.Batch(cmds...)
		}
	}

	if a.showSessionMoveDialog {
		d, moveCmd := a.sessionMoveDialog.Update(msg)
		a.sessionMoveDialog = d.(dialog.SessionMoveDialog)
		cmds = append(cmds, moveCmd)
		// Only block key messages send all other messages down
		if _, ok := msg.(tea.KeyMsg); ok {
			return a, tea.Batch(cmds...)
		}
	}

	if a.showStashDialog {
		d, stashCmd := a.stashDialog.Update(msg)
		a.stashDialog = d.(dialog.StashDialog)
		cmds = append(cmds, stashCmd)
		// Only block key messages send all other messages down
		if _, ok := msg.(tea.KeyMsg); ok {
			return a, tea.Batch(cmds...)
		}
	}

	if a.showPluginsDialog {
		d, pluginsCmd := a.pluginsDialog.Update(msg)
		a.pluginsDialog = d.(dialog.PluginsDialog)
		cmds = append(cmds, pluginsCmd)
		if _, ok := msg.(tea.KeyMsg); ok {
			return a, tea.Batch(cmds...)
		}
	}

	if a.showConnectDialog {
		d, connectCmd := a.connectDialog.Update(msg)
		a.connectDialog = d.(dialog.ConnectDialog)
		cmds = append(cmds, connectCmd)
		if _, ok := msg.(tea.KeyMsg); ok {
			return a, tea.Batch(cmds...)
		}
	}

	s, _ := a.status.Update(msg)
	a.status = s.(core.StatusCmp)
	a.pages[a.currentPage], cmd = a.pages[a.currentPage].Update(msg)
	cmds = append(cmds, cmd)
	return a, tea.Batch(cmds...)
}

// RegisterCommand adds a command to the command dialog. Commands without an
// explicit group land under "Custom" (Spec 09s).
func (a *appModel) RegisterCommand(cmd dialog.Command) {
	if cmd.Group == "" {
		cmd.Group = "Custom"
	}
	a.commands = append(a.commands, cmd)
}

// openCommandDialog shows the command dialog, optionally narrowed to the
// commands whose id matches a slash name (Spec 09c). An unknown or empty name
// shows the full list.
func (a *appModel) openCommandDialog(name string) tea.Cmd {
	cmds, ok := filterCommands(a.commands, name)
	if !ok {
		return util.ReportWarn(fmt.Sprintf("Unknown command: /%s", name))
	}
	if len(cmds) == 0 {
		return util.ReportWarn("No commands available")
	}
	a.commandDialog.SetCommands(cmds)
	a.showCommandDialog = true
	return a.commandDialog.Init()
}

// filterCommands narrows commands to the one matching a slash name. An alias
// of a command matches too (Spec 09h). An empty name returns the whole list. ok
// is false when a name was given but nothing matched (so the caller can warn
// about an unknown command).
func filterCommands(commands []dialog.Command, name string) ([]dialog.Command, bool) {
	if name == "" {
		return commands, true
	}
	matched := make([]dialog.Command, 0, len(commands))
	for _, c := range commands {
		if matchesCommandName(c, name) {
			matched = append(matched, c)
		}
	}
	if len(matched) == 0 {
		return nil, false
	}
	return matched, true
}

// matchesCommandName reports whether name is the id or an alias of c.
func matchesCommandName(c dialog.Command, name string) bool {
	if strings.EqualFold(c.ID, name) {
		return true
	}
	for _, alias := range c.Aliases {
		if strings.EqualFold(alias, name) {
			return true
		}
	}
	return false
}

// pluginInfos builds the view model for the plugins dialog from the loaded
// plugin manager.
func pluginInfos() []dialog.PluginInfo {
	mgr := plugin.GetManager()
	if mgr == nil {
		return nil
	}
	var out []dialog.PluginInfo
	for _, p := range mgr.Summaries() {
		info := dialog.PluginInfo{Name: p.Name}
		for _, h := range p.Hooks {
			info.Hooks = append(info.Hooks, string(h))
		}
		for _, t := range p.Tools {
			info.Tools = append(info.Tools, t)
		}
		out = append(out, info)
	}
	return out
}

func (a *appModel) findCommand(id string) (dialog.Command, bool) {
	for _, cmd := range a.commands {
		if cmd.ID == id {
			return cmd, true
		}
	}
	return dialog.Command{}, false
}

func (a *appModel) moveToPage(pageID page.PageID) tea.Cmd {
	if a.app.Agents.IsBusy() {
		// For now we don't move to any page if the agent is busy
		return util.ReportWarn("Agent is busy, please wait...")
	}

	var cmds []tea.Cmd
	if _, ok := a.loadedPages[pageID]; !ok {
		cmd := a.pages[pageID].Init()
		cmds = append(cmds, cmd)
		a.loadedPages[pageID] = true
	}
	a.previousPage = a.currentPage
	a.currentPage = pageID
	if sizable, ok := a.pages[a.currentPage].(layout.Sizeable); ok {
		cmd := sizable.SetSize(a.width, a.height)
		cmds = append(cmds, cmd)
	}

	return tea.Batch(cmds...)
}

// leaderHintView renders the which-key popup listing the suffixes a pending
// leader sequence accepts (Spec 09d).
func (a appModel) leaderHintView() string {
	t := theme.CurrentTheme()
	style := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(t.BorderFocused()).
		BorderBackground(t.Background()).
		Padding(0, 1).
		Background(t.Background()).
		Foreground(t.Text())

	label := "leader"
	if bound := keys.Leader.Keys(); len(bound) > 0 {
		label = "leader " + bound[0]
	}
	header := lipgloss.NewStyle().
		Foreground(t.Primary()).
		Bold(true).
		Render(label + " — press a key")

	lines := append([]string{header}, leaderHintRows()...)
	return style.Render(strings.Join(lines, "\n"))
}

func (a appModel) View() string {
	components := []string{
		a.pages[a.currentPage].View(),
	}

	components = append(components, a.status.View())

	appView := lipgloss.JoinVertical(lipgloss.Top, components...)

	if a.showPermissions {
		overlay := a.permissions.View()
		row := lipgloss.Height(appView) / 2
		row -= lipgloss.Height(overlay) / 2
		col := lipgloss.Width(appView) / 2
		col -= lipgloss.Width(overlay) / 2
		appView = layout.PlaceOverlay(
			col,
			row,
			overlay,
			appView,
			true,
		)
	}

	if a.showFilepicker {
		overlay := a.filepicker.View()
		row := lipgloss.Height(appView) / 2
		row -= lipgloss.Height(overlay) / 2
		col := lipgloss.Width(appView) / 2
		col -= lipgloss.Width(overlay) / 2
		appView = layout.PlaceOverlay(
			col,
			row,
			overlay,
			appView,
			true,
		)

	}

	// Show compacting status overlay
	if a.isCompacting {
		t := theme.CurrentTheme()
		style := lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(t.BorderFocused()).
			BorderBackground(t.Background()).
			Padding(1, 2).
			Background(t.Background()).
			Foreground(t.Text())

		overlay := style.Render("Summarizing\n" + a.compactingMessage)
		row := lipgloss.Height(appView) / 2
		row -= lipgloss.Height(overlay) / 2
		col := lipgloss.Width(appView) / 2
		col -= lipgloss.Width(overlay) / 2
		appView = layout.PlaceOverlay(
			col,
			row,
			overlay,
			appView,
			true,
		)
	}

	// Which-key popup: the leader prefix is waiting for its suffix (Spec 09d).
	if a.leaderPending {
		overlay := a.leaderHintView()
		row := lipgloss.Height(appView) - lipgloss.Height(overlay) - 1
		if row < 0 {
			row = 0
		}
		col := lipgloss.Width(appView)/2 - lipgloss.Width(overlay)/2
		if col < 0 {
			col = 0
		}
		appView = layout.PlaceOverlay(col, row, overlay, appView, true)
	}

	if a.showHelp {
		bindings := layout.KeyMapToSlice(keys)
		if p, ok := a.pages[a.currentPage].(layout.Bindings); ok {
			bindings = append(bindings, p.BindingKeys()...)
		}
		if a.showPermissions {
			bindings = append(bindings, a.permissions.BindingKeys()...)
		}
		if a.currentPage == page.LogsPage {
			bindings = append(bindings, logsKeyReturnKey)
		}
		if !a.app.Agents.IsBusy() {
			bindings = append(bindings, helpEsc)
		}
		a.help.SetBindings(bindings)

		overlay := a.help.View()
		row := lipgloss.Height(appView) / 2
		row -= lipgloss.Height(overlay) / 2
		col := lipgloss.Width(appView) / 2
		col -= lipgloss.Width(overlay) / 2
		appView = layout.PlaceOverlay(
			col,
			row,
			overlay,
			appView,
			true,
		)
	}

	if a.showQuit {
		overlay := a.quit.View()
		row := lipgloss.Height(appView) / 2
		row -= lipgloss.Height(overlay) / 2
		col := lipgloss.Width(appView) / 2
		col -= lipgloss.Width(overlay) / 2
		appView = layout.PlaceOverlay(
			col,
			row,
			overlay,
			appView,
			true,
		)
	}

	if a.showSessionDialog {
		overlay := a.sessionDialog.View()
		row := lipgloss.Height(appView) / 2
		row -= lipgloss.Height(overlay) / 2
		col := lipgloss.Width(appView) / 2
		col -= lipgloss.Width(overlay) / 2
		appView = layout.PlaceOverlay(
			col,
			row,
			overlay,
			appView,
			true,
		)
	}

	if a.showModelDialog {
		overlay := a.modelDialog.View()
		row := lipgloss.Height(appView) / 2
		row -= lipgloss.Height(overlay) / 2
		col := lipgloss.Width(appView) / 2
		col -= lipgloss.Width(overlay) / 2
		appView = layout.PlaceOverlay(
			col,
			row,
			overlay,
			appView,
			true,
		)
	}

	if a.showAgentDialog {
		overlay := a.agentDialog.View()
		row := lipgloss.Height(appView) / 2
		row -= lipgloss.Height(overlay) / 2
		col := lipgloss.Width(appView) / 2
		col -= lipgloss.Width(overlay) / 2
		appView = layout.PlaceOverlay(
			col,
			row,
			overlay,
			appView,
			true,
		)
	}

	if a.showCommandDialog {
		overlay := a.commandDialog.View()
		row := lipgloss.Height(appView) / 2
		row -= lipgloss.Height(overlay) / 2
		col := lipgloss.Width(appView) / 2
		col -= lipgloss.Width(overlay) / 2
		appView = layout.PlaceOverlay(
			col,
			row,
			overlay,
			appView,
			true,
		)
	}

	if a.showTimelineDialog {
		overlay := a.timelineDialog.View()
		row := lipgloss.Height(appView) / 2
		row -= lipgloss.Height(overlay) / 2
		col := lipgloss.Width(appView) / 2
		col -= lipgloss.Width(overlay) / 2
		appView = layout.PlaceOverlay(
			col,
			row,
			overlay,
			appView,
			true,
		)
	}

	if a.showMessageDialog {
		overlay := a.messageDialog.View()
		row := lipgloss.Height(appView) / 2
		row -= lipgloss.Height(overlay) / 2
		col := lipgloss.Width(appView) / 2
		col -= lipgloss.Width(overlay) / 2
		appView = layout.PlaceOverlay(
			col,
			row,
			overlay,
			appView,
			true,
		)
	}

	if a.showForkDialog {
		overlay := a.forkDialog.View()
		row := lipgloss.Height(appView) / 2
		row -= lipgloss.Height(overlay) / 2
		col := lipgloss.Width(appView) / 2
		col -= lipgloss.Width(overlay) / 2
		appView = layout.PlaceOverlay(
			col,
			row,
			overlay,
			appView,
			true,
		)
	}

	if a.showSessionMoveDialog {
		overlay := a.sessionMoveDialog.View()
		row := lipgloss.Height(appView) / 2
		row -= lipgloss.Height(overlay) / 2
		col := lipgloss.Width(appView) / 2
		col -= lipgloss.Width(overlay) / 2
		appView = layout.PlaceOverlay(
			col,
			row,
			overlay,
			appView,
			true,
		)
	}

	if a.showStashDialog {
		overlay := a.stashDialog.View()
		row := lipgloss.Height(appView) / 2
		row -= lipgloss.Height(overlay) / 2
		col := lipgloss.Width(appView) / 2
		col -= lipgloss.Width(overlay) / 2
		appView = layout.PlaceOverlay(
			col,
			row,
			overlay,
			appView,
			true,
		)
	}

	if a.showInitDialog {
		overlay := a.initDialog.View()
		appView = layout.PlaceOverlay(
			a.width/2-lipgloss.Width(overlay)/2,
			a.height/2-lipgloss.Height(overlay)/2,
			overlay,
			appView,
			true,
		)
	}

	if a.showThemeDialog {
		overlay := a.themeDialog.View()
		row := lipgloss.Height(appView) / 2
		row -= lipgloss.Height(overlay) / 2
		col := lipgloss.Width(appView) / 2
		col -= lipgloss.Width(overlay) / 2
		appView = layout.PlaceOverlay(
			col,
			row,
			overlay,
			appView,
			true,
		)
	}

	if a.showPluginsDialog {
		overlay := a.pluginsDialog.View()
		row := lipgloss.Height(appView) / 2
		row -= lipgloss.Height(overlay) / 2
		col := lipgloss.Width(appView) / 2
		col -= lipgloss.Width(overlay) / 2
		appView = layout.PlaceOverlay(col, row, overlay, appView, true)
	}

	if a.showConnectDialog {
		overlay := a.connectDialog.View()
		row := lipgloss.Height(appView) / 2
		row -= lipgloss.Height(overlay) / 2
		col := lipgloss.Width(appView) / 2
		col -= lipgloss.Width(overlay) / 2
		appView = layout.PlaceOverlay(col, row, overlay, appView, true)
	}

	if a.showMultiArgumentsDialog {
		overlay := a.multiArgumentsDialog.View()
		row := lipgloss.Height(appView) / 2
		row -= lipgloss.Height(overlay) / 2
		col := lipgloss.Width(appView) / 2
		col -= lipgloss.Width(overlay) / 2
		appView = layout.PlaceOverlay(
			col,
			row,
			overlay,
			appView,
			true,
		)
	}

	return appView
}

func New(app *app.App) tea.Model {
	applyKeybinds()
	startPage := page.ChatPage
	model := &appModel{
		currentPage:       startPage,
		loadedPages:       make(map[page.PageID]bool),
		status:            core.NewStatusCmp(app.LSPClients, app.Agents.Active),
		help:              dialog.NewHelpCmp(),
		quit:              dialog.NewQuitCmp(),
		sessionDialog:     dialog.NewSessionDialogCmp(),
		commandDialog:     dialog.NewCommandDialogCmp(),
		modelDialog:       dialog.NewModelDialogCmp(app.Agents.Active),
		agentDialog:       dialog.NewAgentDialogCmp(app.Agents.Active),
		permissions:       dialog.NewPermissionDialogCmp(),
		initDialog:        dialog.NewInitDialogCmp(),
		themeDialog:       dialog.NewThemeDialogCmp(),
		timelineDialog:    dialog.NewTimelineDialogCmp(),
		forkDialog:        dialog.NewForkDialogCmp(),
		sessionMoveDialog: dialog.NewSessionMoveDialogCmp(),
		stashDialog:       dialog.NewStashDialogCmp(),
		pluginsDialog:     dialog.NewPluginsDialog(),
		connectDialog:     dialog.NewConnectDialog(),
		app:               app,
		commands:          []dialog.Command{},
		pages: map[page.PageID]tea.Model{
			page.ChatPage: page.NewChatPage(app),
			page.LogsPage: page.NewLogsPage(),
		},
		filepicker: dialog.NewFilepickerCmp(app),
	}

	model.RegisterCommand(dialog.Command{
		ID:          "init",
		Group:       "System",
		Title:       "Initialize Project",
		Description: "Create/Update the OpenCode.md memory file",
		Handler: func(cmd dialog.Command) tea.Cmd {
			prompt := `Please analyze this codebase and create a OpenCode.md file containing:
1. Build/lint/test commands - especially for running a single test
2. Code style guidelines including imports, formatting, types, naming conventions, error handling, etc.

The file you create will be given to agentic coding agents (such as yourself) that operate in this repository. Make it about 20 lines long.
If there's already a opencode.md, improve it.
If there are Cursor rules (in .cursor/rules/ or .cursorrules) or Copilot rules (in .github/copilot-instructions.md), make sure to include them.`
			return tea.Batch(
				util.CmdHandler(chat.SendMsg{
					Text: prompt,
				}),
			)
		},
	})

	model.RegisterCommand(dialog.Command{
		ID:          "compact",
		Group:       "Session",
		Title:       "Compact Session",
		Description: "Summarize the current session and create a new one with the summary",
		Handler: func(cmd dialog.Command) tea.Cmd {
			return func() tea.Msg {
				return startCompactSessionMsg{}
			}
		},
	})
	model.RegisterCommand(dialog.Command{
		ID:          "export",
		Group:       "Session",
		Title:       "Export Session",
		Description: "Export the conversation to Markdown and open it in your editor",
		Handler: func(cmd dialog.Command) tea.Cmd {
			return util.CmdHandler(exportSessionMsg{})
		},
	})

	model.RegisterCommand(dialog.Command{
		ID:          "timeline",
		Group:       "Session",
		Title:       "Jump to Message",
		Description: "Pick a user message of this session and jump to it",
		Handler: func(cmd dialog.Command) tea.Cmd {
			return util.CmdHandler(showTimelineMsg{})
		},
	})

	model.RegisterCommand(dialog.Command{
		ID:          "session_copy",
		Group:       "Session",
		Title:       "Copy Session Transcript",
		Description: "Copy the whole conversation as Markdown to the clipboard",
		Handler: func(cmd dialog.Command) tea.Cmd {
			return util.CmdHandler(copySessionMsg{})
		},
	})

	model.RegisterCommand(dialog.Command{
		ID:          "fork",
		Group:       "Session",
		Title:       "Fork Session",
		Description: "Copy this session (or up to a message) into a new one",
		Handler: func(cmd dialog.Command) tea.Cmd {
			return util.CmdHandler(showForkMsg{})
		},
	})

	model.RegisterCommand(dialog.Command{
		ID:          "prompt_stash",
		Group:       "Prompt",
		Title:       "Stash prompt",
		Description: "Save the current prompt into the stash and clear the composer",
		Handler: func(cmd dialog.Command) tea.Cmd {
			return util.CmdHandler(chat.StashPushMsg{})
		},
	})

	model.RegisterCommand(dialog.Command{
		ID:          "prompt_stash_pop",
		Group:       "Prompt",
		Title:       "Stash pop",
		Description: "Restore the most recently stashed prompt",
		Handler: func(cmd dialog.Command) tea.Cmd {
			return util.CmdHandler(chat.StashPopMsg{})
		},
	})

	model.RegisterCommand(dialog.Command{
		ID:          "prompt_stash_list",
		Group:       "Prompt",
		Title:       "Stash list",
		Description: "List the stashed prompts and pick one to restore",
		Handler: func(cmd dialog.Command) tea.Cmd {
			return util.CmdHandler(chat.RequestStashListMsg{})
		},
	})

	model.RegisterCommand(dialog.Command{
		ID:          "new",
		Group:       "Session",
		Title:       "New Session",
		Description: "Start a new session",
		Aliases:     []string{"clear"},
		Handler: func(cmd dialog.Command) tea.Cmd {
			return util.CmdHandler(page.NewSessionMsg{})
		},
	})
	model.RegisterCommand(dialog.Command{
		ID:          "sessions",
		Group:       "Session",
		Title:       "Switch Session",
		Description: "List and switch between sessions",
		Aliases:     []string{"resume", "continue"},
		Handler: func(cmd dialog.Command) tea.Cmd {
			return util.CmdHandler(showSessionListMsg{})
		},
	})
	model.RegisterCommand(dialog.Command{
		ID:          "models",
		Group:       "Agent",
		Title:       "Select Model",
		Description: "List the available models",
		Handler: func(cmd dialog.Command) tea.Cmd {
			return util.CmdHandler(showModelListMsg{})
		},
	})
	model.RegisterCommand(dialog.Command{
		ID:          "themes",
		Group:       "System",
		Title:       "Select Theme",
		Description: "List the available themes",
		Handler: func(cmd dialog.Command) tea.Cmd {
			return util.CmdHandler(showThemeListMsg{})
		},
	})
	model.RegisterCommand(dialog.Command{
		ID:          "help",
		Group:       "System",
		Title:       "Show Help",
		Description: "Show the keyboard shortcuts",
		Handler: func(cmd dialog.Command) tea.Cmd {
			return util.CmdHandler(showHelpMsg{})
		},
	})
	model.RegisterCommand(dialog.Command{
		ID:          "editor",
		Group:       "Prompt",
		Title:       "External Editor",
		Description: "Compose the message in your $EDITOR",
		Handler: func(cmd dialog.Command) tea.Cmd {
			return util.CmdHandler(chat.OpenEditorMsg{})
		},
	})
	model.RegisterCommand(dialog.Command{
		ID:          "thinking",
		Group:       "Session",
		Title:       "Toggle Thinking",
		Description: "Show or hide the reasoning blocks in the conversation",
		Handler: func(cmd dialog.Command) tea.Cmd {
			return util.CmdHandler(chat.ToggleThinkingMsg{})
		},
	})
	model.RegisterCommand(dialog.Command{
		ID:          "details",
		Group:       "Session",
		Title:       "Toggle Tool Details",
		Description: "Show or hide tool execution details in the conversation",
		Handler: func(cmd dialog.Command) tea.Cmd {
			return util.CmdHandler(chat.ToggleDetailsMsg{})
		},
	})
	model.RegisterCommand(dialog.Command{
		ID:          "undo",
		Group:       "Session",
		Title:       "Undo Message",
		Description: "Revert the last user message, its responses and their file changes",
		Handler: func(cmd dialog.Command) tea.Cmd {
			return util.CmdHandler(undoMessageMsg{})
		},
	})
	model.RegisterCommand(dialog.Command{
		ID:          "redo",
		Group:       "Session",
		Title:       "Redo Message",
		Description: "Clear the staged revert: restore the messages and file changes",
		Handler: func(cmd dialog.Command) tea.Cmd {
			return util.CmdHandler(redoMessageMsg{})
		},
	})

	model.RegisterCommand(dialog.Command{
		ID:          "move",
		Group:       "Session",
		Title:       "Move Session",
		Description: "Choose the working directory for this session's tools",
		Handler: func(cmd dialog.Command) tea.Cmd {
			return util.CmdHandler(openSessionMoveDialogMsg{})
		},
	})

	model.RegisterCommand(dialog.Command{
		ID:          "exit",
		Group:       "System",
		Title:       "Exit OpenCode",
		Description: "Leave the TUI (asks for confirmation)",
		Aliases:     []string{"quit", "q"},
		Handler: func(cmd dialog.Command) tea.Cmd {
			return util.CmdHandler(showQuitMsg{})
		},
	})

	model.RegisterCommand(dialog.Command{
		ID:          "plugins",
		Group:       "System",
		Title:       "Plugins",
		Description: "List the loaded plugins, their hooks and tools",
		Handler: func(cmd dialog.Command) tea.Cmd {
			return util.CmdHandler(dialog.ShowPluginsDialogMsg{Plugins: pluginInfos()})
		},
	})
	model.RegisterCommand(dialog.Command{
		ID:          "connect",
		Group:       "Integration",
		Title:       "Connect Provider",
		Description: "Store an API key for a provider (e.g. OpenCode Zen)",
		Handler: func(cmd dialog.Command) tea.Cmd {
			return util.CmdHandler(dialog.ShowConnectDialogMsg{Provider: models.ProviderOpencode})
		},
	})

	// Load custom commands
	customCommands, err := dialog.LoadCustomCommands()
	if err != nil {
		logging.Warn("Failed to load custom commands", "error", err)
	} else {
		for _, cmd := range customCommands {
			model.RegisterCommand(cmd)
		}
	}

	return model
}
