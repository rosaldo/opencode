package agent

import (
	"context"

	"github.com/opencode-ai/opencode/internal/agentdef"
	"github.com/opencode-ai/opencode/internal/config"
	"github.com/opencode-ai/opencode/internal/history"
	"github.com/opencode-ai/opencode/internal/llm/tools"
	"github.com/opencode-ai/opencode/internal/lsp"
	"github.com/opencode-ai/opencode/internal/message"
	"github.com/opencode-ai/opencode/internal/permission"
	"github.com/opencode-ai/opencode/internal/session"
)

// CoderAgentTools returns the full tool set of the coder agent. Kept as a thin
// wrapper over FullToolSet for callers that do not have an agent definition.
func CoderAgentTools(
	permissions permission.Service,
	sessions session.Service,
	messages message.Service,
	history history.Service,
	lspClients map[string]*lsp.Client,
) []tools.BaseTool {
	caller, _ := agentdef.Get().Get(string(config.AgentCoder))
	return FullToolSet(context.Background(), caller, AgentToolDeps{
		Permissions: permissions,
		Sessions:    sessions,
		Messages:    messages,
		History:     history,
		LspClients:  lspClients,
	})
}

// TaskAgentTools returns the read only tool set of the task subagent. Kept as a
// thin wrapper over TaskToolSet for callers that do not have a definition.
func TaskAgentTools(lspClients map[string]*lsp.Client) []tools.BaseTool {
	return TaskToolSet(AgentToolDeps{LspClients: lspClients})
}
