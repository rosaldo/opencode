package cmd

import (
	"context"
	"os"

	"github.com/opencode-ai/opencode/internal/acp"
	"github.com/opencode-ai/opencode/internal/app"
	"github.com/opencode-ai/opencode/internal/config"
	"github.com/opencode-ai/opencode/internal/db"
	"github.com/opencode-ai/opencode/internal/llm/agent"
	"github.com/opencode-ai/opencode/internal/message"
	"github.com/spf13/cobra"
)

var acpCmd = &cobra.Command{
	Use:   "acp",
	Short: "Run the Agent Client Protocol server over stdio",
	Long: `Serve the agent over stdio using the Agent Client Protocol so editors
(Zed, etc.) can drive it.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		cwd, err := os.Getwd()
		if err != nil {
			return err
		}
		if c, _ := cmd.Flags().GetString("cwd"); c != "" {
			cwd = c
		}
		if _, err := config.Load(cwd, false); err != nil {
			return err
		}

		conn, err := db.Connect()
		if err != nil {
			return err
		}
		defer conn.Close()

		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		a, err := app.New(ctx, conn)
		if err != nil {
			return err
		}
		defer a.Shutdown()

		svc, err := a.Agents.ActiveAgent()
		if err != nil {
			return err
		}

		srv := acp.New(&acpAdapter{svc: svc}, os.Stdin, os.Stdout)
		return srv.Run(ctx)
	},
}

// acpAdapter adapts an agent.Service to the acp.Agent interface.
type acpAdapter struct {
	svc agent.Service
}

func (a *acpAdapter) Run(ctx context.Context, sessionID, content string) (<-chan acp.Chunk, error) {
	events, err := a.svc.Run(ctx, sessionID, content)
	if err != nil {
		return nil, err
	}
	out := make(chan acp.Chunk, 32)
	go func() {
		defer close(out)
		for ev := range events {
			if ev.Error != nil {
				out <- acp.Chunk{Err: ev.Error}
				return
			}
			if ev.Type == agent.AgentEventTypeResponse {
				if text := textFromMessage(ev.Message); text != "" {
					out <- acp.Chunk{Text: text}
				}
			}
		}
		out <- acp.Chunk{Done: true}
	}()
	return out, nil
}

func (a *acpAdapter) Cancel(sessionID string) { a.svc.Cancel(sessionID) }

// textFromMessage extracts the assistant text content from an agent event.
func textFromMessage(m message.Message) string {
	if m.Role != message.Assistant {
		return ""
	}
	return m.Content().Text
}

func init() {
	acpCmd.Flags().StringP("cwd", "c", "", "Working directory")
	rootCmd.AddCommand(acpCmd)
}
