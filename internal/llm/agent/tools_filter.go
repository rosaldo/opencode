package agent

import (
	"context"
	"strings"

	"github.com/opencode-ai/opencode/internal/agentdef"
	"github.com/opencode-ai/opencode/internal/config"
	"github.com/opencode-ai/opencode/internal/history"
	"github.com/opencode-ai/opencode/internal/llm/tools"
	"github.com/opencode-ai/opencode/internal/lsp"
	"github.com/opencode-ai/opencode/internal/message"
	"github.com/opencode-ai/opencode/internal/permission"
	"github.com/opencode-ai/opencode/internal/session"
)

// AgentToolDeps carries the services an agent's tools need. The app builds one
// value and reuses it for every agent it instantiates. DelegationChain lists
// the agents that spawned the current one (outermost first); it is used to
// refuse delegations that would loop back up the chain.
type AgentToolDeps struct {
	Permissions     permission.Service
	Sessions        session.Service
	Messages        message.Service
	History         history.Service
	LspClients      map[string]*lsp.Client
	DelegationChain []string
}

// FullToolSet returns every tool available to a full access agent: the local
// tools, the delegation tool (when the agent may delegate), diagnostics when an
// LSP is running and the MCP tools. caller is the definition the tools are
// built for; it may be nil for callers that do not have one.
func FullToolSet(ctx context.Context, caller *agentdef.Definition, deps AgentToolDeps) []tools.BaseTool {
	local := []tools.BaseTool{
		tools.NewBashTool(deps.Permissions),
		tools.NewEditTool(deps.LspClients, deps.Permissions, deps.History),
		tools.NewFetchTool(deps.Permissions),
		tools.NewGlobTool(),
		tools.NewGrepTool(),
		tools.NewLsTool(),
		tools.NewSourcegraphTool(),
		tools.NewViewTool(deps.LspClients),
		tools.NewPatchTool(deps.LspClients, deps.Permissions, deps.History),
		tools.NewWriteTool(deps.LspClients, deps.Permissions, deps.History),
	}
	if tool := agentToolFor(caller, deps); tool != nil {
		local = append(local, tool)
	}
	out := append(local, GetMcpTools(ctx, deps.Permissions)...)
	if len(deps.LspClients) > 0 {
		out = append(out, tools.NewDiagnosticsTool(deps.LspClients))
	}
	return out
}

// TaskToolSet is the read only tool set of the builtin task subagent.
func TaskToolSet(deps AgentToolDeps) []tools.BaseTool {
	return []tools.BaseTool{
		tools.NewGlobTool(),
		tools.NewGrepTool(),
		tools.NewLsTool(),
		tools.NewSourcegraphTool(),
		tools.NewViewTool(deps.LspClients),
	}
}

// ToolsForDefinition builds the tool list for one agent. It starts from the
// agent's base set, wraps the permission service with the agent's own policy
// and drops every tool the agent denies itself.
func ToolsForDefinition(def *agentdef.Definition, deps AgentToolDeps) []tools.BaseTool {
	if def == nil {
		return FullToolSet(context.Background(), nil, deps)
	}
	base := deps
	if policy, err := permission.ParsePolicy(def.Permissions); err == nil && policy.HasRules {
		base.Permissions = permission.NewAgentScoped(base.Permissions, policy)
	}
	return FilterTools(baseToolSet(def, base), def.Tools)
}

// baseToolSet picks the starting tool set for an agent. Custom agents inherit
// the full set; builtins keep the lists they had before this package existed.
func baseToolSet(def *agentdef.Definition, deps AgentToolDeps) []tools.BaseTool {
	if def == nil || !def.Builtin {
		return FullToolSet(context.Background(), def, deps)
	}
	switch def.Name {
	case string(config.AgentTask):
		return TaskToolSet(deps)
	case string(config.AgentTitle), string(config.AgentSummarizer):
		return nil
	default:
		return FullToolSet(context.Background(), def, deps)
	}
}

// FilterTools applies an agent's tools rules to a tool list. No rules keeps the
// list untouched. Otherwise "name": false removes a tool, and a map that only
// enables tools (at least one true and no catch all) is read as an allow list,
// dropping everything it does not mention.
func FilterTools(list []tools.BaseTool, rules map[string]bool) []tools.BaseTool {
	if len(rules) == 0 {
		return list
	}
	out := make([]tools.BaseTool, 0, len(list))
	for _, tool := range list {
		if ToolEnabled(rules, tool.Info().Name) {
			out = append(out, tool)
		}
	}
	return out
}

// ToolEnabled reports whether an agent with the given rules may use a tool. An
// exact name wins over a wildcard, and the longest literal part of a wildcard
// wins over a shorter one ("mymcp_foo_*" beats "mymcp_*").
func ToolEnabled(rules map[string]bool, name string) bool {
	if len(rules) == 0 {
		return true
	}
	if enabled, ok := rules[name]; ok {
		return enabled
	}
	if enabled, ok := bestWildcard(rules, name); ok {
		return enabled
	}
	if catchAll, ok := rules["*"]; ok {
		return catchAll
	}
	// No catch all and at least one true entry: the map lists what is allowed.
	return !hasAnyTrue(rules)
}

func hasAnyTrue(rules map[string]bool) bool {
	for _, enabled := range rules {
		if enabled {
			return true
		}
	}
	return false
}

func bestWildcard(rules map[string]bool, name string) (bool, bool) {
	var best bool
	score := -1
	found := false
	for pattern, enabled := range rules {
		if pattern == "*" || !strings.Contains(pattern, "*") {
			continue
		}
		if !permission.MatchWildcard(pattern, name) {
			continue
		}
		if s := len(strings.ReplaceAll(pattern, "*", "")); s > score {
			best, score, found = enabled, s, true
		}
	}
	return best, found
}
