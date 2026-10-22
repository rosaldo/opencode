package tui

import (
	"context"
	"os"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/opencode-ai/opencode/internal/tui/components/chat"
	"github.com/opencode-ai/opencode/internal/tui/util"
)

// exportSessionMsg asks the app model to export the selected session: the
// handler of the /export command cannot reach the model itself (Spec 09g).
type exportSessionMsg struct{}

// exportSession renders the current session as Markdown, writes it to a temp
// file and opens that file in $EDITOR — the /export command and <leader>x
// (Spec 09g).
func (a appModel) exportSession() (appModel, tea.Cmd) {
	if a.app == nil || a.selectedSession.ID == "" {
		return a, util.ReportWarn("No active session to export")
	}
	msgs, err := a.app.Messages.List(context.Background(), a.selectedSession.ID)
	if err != nil {
		return a, util.ReportError(err)
	}
	if len(msgs) == 0 {
		return a, util.ReportWarn("Nothing to export yet")
	}
	file, err := os.CreateTemp("", "opencode-export-*.md")
	if err != nil {
		return a, util.ReportError(err)
	}
	if _, err := file.WriteString(chat.SessionToMarkdown(a.selectedSession.Title, msgs)); err != nil {
		file.Close()
		return a, util.ReportError(err)
	}
	if err := file.Close(); err != nil {
		return a, util.ReportError(err)
	}
	path := file.Name()
	return a, tea.ExecProcess(chat.EditorCommand(path), func(err error) tea.Msg {
		if err != nil {
			return util.ReportError(err)
		}
		return util.ReportInfo("Exported session to " + path)
	})
}
