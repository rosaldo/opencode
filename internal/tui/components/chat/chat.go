package chat

import (
	"fmt"
	"os"
	"regexp"
	"sort"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
	"github.com/opencode-ai/opencode/internal/config"
	"github.com/opencode-ai/opencode/internal/message"
	"github.com/opencode-ai/opencode/internal/session"
	"github.com/opencode-ai/opencode/internal/tui/styles"
	"github.com/opencode-ai/opencode/internal/tui/theme"
	"github.com/opencode-ai/opencode/internal/version"
)

type SendMsg struct {
	Text        string
	Attachments []message.Attachment
	// MentionedAgent is set when the message text named a subagent with
	// "@name"; it routes the turn to that agent in a child session instead of
	// the active primary agent.
	MentionedAgent string
}

// mentionPattern matches a leading or whitespace-preceded "@name" token. Names
// follow the agent-name charset (letters, digits, dash, underscore, dot).
var mentionPattern = regexp.MustCompile(`(^|\s)@([A-Za-z0-9._-]+)`)

// splitMention extracts the first "@name" mention from text. It returns the
// cleaned message (mention removed, whitespace collapsed at the start) and the
// mentioned name, or an empty name when there is none. Only the first mention
// is honored; that keeps the routing unambiguous.
func splitMention(text string) (clean string, name string) {
	loc := mentionPattern.FindStringSubmatchIndex(text)
	if loc == nil {
		return text, ""
	}
	name = text[loc[4]:loc[5]]
	clean = strings.TrimSpace(text[:loc[0]] + text[loc[1]:])
	if clean == "" {
		// Keep the mention text as the prompt when it was the whole message.
		clean = strings.TrimSpace(text[loc[0]:loc[1]])
	}
	return clean, name
}

type SessionSelectedMsg = session.Session

type SessionClearedMsg struct{}

// RunSlashCommandMsg asks the app to open the command dialog pre-filtered by a
// slash command name (Spec 09c). The editor emits it when the user types "/" at
// the start of the prompt.
type RunSlashCommandMsg struct {
	// Name is the command name after "/", empty when only "/" was typed.
	Name string
}

// RunShellMsg asks the app to run a shell command and add its output to the
// conversation (Spec 09c). The editor emits it for "!cmd" input.
type RunShellMsg struct {
	Command string
}

type EditorFocusMsg bool

// RevertBannerMsg carries the "N messages reverted" banner text shown above
// the prompt while a revert is staged (Spec 09u). An empty Text hides it.
type RevertBannerMsg struct {
	Text string
}

// CommitRevertMsg is emitted when a new turn starts: like the TS version, the
// staged revert is committed (the truncated transcript and rolled-back files
// become permanent) unless the user clears it first (Spec 09u).
type CommitRevertMsg struct{}

func header(width int, session session.Session) string {
	return lipgloss.JoinVertical(
		lipgloss.Top,
		logo(width),
		repo(width),
		"",
		cwd(width, session),
	)
}

func lspsConfigured(width int) string {
	cfg := config.Get()
	title := "LSP Configuration"
	title = ansi.Truncate(title, width, "…")

	t := theme.CurrentTheme()
	baseStyle := styles.BaseStyle()

	lsps := baseStyle.
		Width(width).
		Foreground(t.Primary()).
		Bold(true).
		Render(title)

	// Get LSP names and sort them for consistent ordering
	var lspNames []string
	for name := range cfg.LSP {
		lspNames = append(lspNames, name)
	}
	sort.Strings(lspNames)

	var lspViews []string
	for _, name := range lspNames {
		lsp := cfg.LSP[name]
		lspName := baseStyle.
			Foreground(t.Text()).
			Render(fmt.Sprintf("• %s", name))

		cmd := lsp.Command
		cmd = ansi.Truncate(cmd, width-lipgloss.Width(lspName)-3, "…")

		lspPath := baseStyle.
			Foreground(t.TextMuted()).
			Render(fmt.Sprintf(" (%s)", cmd))

		lspViews = append(lspViews,
			baseStyle.
				Width(width).
				Render(
					lipgloss.JoinHorizontal(
						lipgloss.Left,
						lspName,
						lspPath,
					),
				),
		)
	}

	return baseStyle.
		Width(width).
		Render(
			lipgloss.JoinVertical(
				lipgloss.Left,
				lsps,
				lipgloss.JoinVertical(
					lipgloss.Left,
					lspViews...,
				),
			),
		)
}

func logo(width int) string {
	logo := fmt.Sprintf("%s %s", styles.OpenCodeIcon, "OpenCode")
	t := theme.CurrentTheme()
	baseStyle := styles.BaseStyle()

	versionText := baseStyle.
		Foreground(t.TextMuted()).
		Render(version.Version)

	return baseStyle.
		Bold(true).
		Width(width).
		Render(
			lipgloss.JoinHorizontal(
				lipgloss.Left,
				logo,
				" ",
				versionText,
			),
		)
}

func repo(width int) string {
	repo := "https://github.com/opencode-ai/opencode"
	t := theme.CurrentTheme()

	return styles.BaseStyle().
		Foreground(t.TextMuted()).
		Width(width).
		Render(repo)
}

func cwd(width int, session session.Session) string {
	dir := session.WorkingDirectory
	if dir == "" {
		dir = config.WorkingDirectory()
	}
	if dir == "" {
		if wd, err := os.Getwd(); err == nil {
			dir = wd
		}
	}
	cwd := fmt.Sprintf("cwd: %s", dir)
	t := theme.CurrentTheme()

	return styles.BaseStyle().
		Foreground(t.TextMuted()).
		Width(width).
		Render(cwd)
}
