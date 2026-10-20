package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"github.com/opencode-ai/opencode/internal/agentdef"
	"github.com/opencode-ai/opencode/internal/config"
	"github.com/opencode-ai/opencode/internal/llm/tools"
	"github.com/opencode-ai/opencode/internal/message"
	"github.com/opencode-ai/opencode/internal/permission"
)

type agentTool struct {
	caller *agentdef.Definition
	policy permission.Policy
	deps   AgentToolDeps
	// chain holds the agents that spawned the caller (outermost first). A
	// delegation that would re-enter an agent already on the chain is refused,
	// which bounds recursion without an arbitrary depth limit.
	chain []string
}

const (
	AgentToolName = "agent"
	// DefaultSubagent is the subagent used when the caller names none.
	DefaultSubagent = string(config.AgentTask)
	// maxDescriptionLength truncates a single subagent description.
	maxDescriptionLength = 120
)

type AgentParams struct {
	Prompt   string `json:"prompt"`
	Subagent string `json:"subagent,omitempty"`
}

func (b *agentTool) Info() tools.ToolInfo {
	return tools.ToolInfo{
		Name:        AgentToolName,
		Description: b.describe(),
		Parameters: map[string]any{
			"prompt": map[string]any{
				"type":        "string",
				"description": "The task for the agent to perform",
			},
			"subagent": map[string]any{
				"type":        "string",
				"description": fmt.Sprintf("The name of the agent to run the task (defaults to %q). Any registered agent may be named; the list above shows the usual ones.", DefaultSubagent),
			},
		},
		Required: []string{"prompt"},
	}
}

// describe builds the tool description: the usage guidance followed by the
// agents this one may run. Any registered agent can be named; listing them is
// only a convenience. The agents of the caller's own department (team) are
// listed with a one line description, since those are the usual delegates; the
// agents of other departments are listed by name only, grouped by department,
// so a registry of hundreds of agents still costs few tokens and no agent is
// ever hidden.
func (b *agentTool) describe() string {
	var sb strings.Builder
	sb.WriteString(toolGuidance)
	listed := subagents(b.caller, false, b.deps.DelegationChain)
	if len(listed) == 0 {
		return sb.String()
	}

	own, others := partitionByTeam(listed, teamOf(b.caller))

	if len(own) > 0 {
		sb.WriteString("\n\nYour department (run by name via the subagent parameter):\n")
		for _, def := range own {
			sb.WriteString(fmt.Sprintf("- %s — %s\n", def.Name, oneLine(def.Description)))
		}
	}
	if len(others) > 0 {
		sb.WriteString("\nOther departments (names only):\n")
		for _, team := range sortedKeys(others) {
			sb.WriteString(fmt.Sprintf("- %s: %s\n", team, strings.Join(others[team], ", ")))
		}
	}
	return sb.String()
}

// teamOf returns the department an agent belongs to. Callers without a
// definition (or without a team) belong to the anonymous team "".
func teamOf(def *agentdef.Definition) string {
	if def == nil {
		return ""
	}
	return def.Team
}

// partitionByTeam splits agent names into the caller's own department and the
// rest, grouped by department name.
func partitionByTeam(defs []*agentdef.Definition, own string) ([]*agentdef.Definition, map[string][]string) {
	var ownList []*agentdef.Definition
	others := make(map[string][]string)
	for _, def := range defs {
		if def.Team == own {
			ownList = append(ownList, def)
			continue
		}
		team := def.Team
		if team == "" {
			team = "(no department)"
		}
		others[team] = append(others[team], def.Name)
	}
	return ownList, others
}

func sortedKeys(m map[string][]string) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

func (b *agentTool) Run(ctx context.Context, call tools.ToolCall) (tools.ToolResponse, error) {
	var params AgentParams
	if err := json.Unmarshal([]byte(call.Input), &params); err != nil {
		return tools.NewTextErrorResponse(fmt.Sprintf("error parsing parameters: %s", err)), nil
	}
	if params.Prompt == "" {
		return tools.NewTextErrorResponse("prompt is required"), nil
	}

	name := strings.TrimSpace(params.Subagent)
	if name == "" {
		name = DefaultSubagent
	}
	target, ok := b.subagentByName(name)
	if !ok {
		return tools.NewTextErrorResponse(fmt.Sprintf("unknown subagent %q", name)), nil
	}
	if action, matched := b.policy.Task(name); matched && action == permission.ActionDeny {
		return tools.NewTextErrorResponse(fmt.Sprintf("the %s agent may not use the %s subagent", callerName(b.caller), name)), nil
	}

	sessionID, messageID := tools.GetContextValues(ctx)
	if sessionID == "" || messageID == "" {
		return tools.ToolResponse{}, fmt.Errorf("session_id and message_id are required")
	}

	agent, err := NewAgent(name, b.deps.Sessions, b.deps.Messages, ToolsForDefinition(target, b.childDeps()))
	if err != nil {
		return tools.ToolResponse{}, fmt.Errorf("error creating agent: %s", err)
	}

	session, err := b.deps.Sessions.CreateTaskSession(ctx, call.ID, sessionID, "New Agent Session")
	if err != nil {
		return tools.ToolResponse{}, fmt.Errorf("error creating session: %s", err)
	}

	done, err := agent.Run(ctx, session.ID, params.Prompt)
	if err != nil {
		return tools.ToolResponse{}, fmt.Errorf("error generating agent: %s", err)
	}
	result := <-done
	if result.Error != nil {
		return tools.ToolResponse{}, fmt.Errorf("error generating agent: %s", result.Error)
	}

	response := result.Message
	if response.Role != message.Assistant {
		return tools.NewTextErrorResponse("no response"), nil
	}

	updatedSession, err := b.deps.Sessions.Get(ctx, session.ID)
	if err != nil {
		return tools.ToolResponse{}, fmt.Errorf("error getting session: %s", err)
	}
	parentSession, err := b.deps.Sessions.Get(ctx, sessionID)
	if err != nil {
		return tools.ToolResponse{}, fmt.Errorf("error getting parent session: %s", err)
	}

	parentSession.Cost += updatedSession.Cost

	_, err = b.deps.Sessions.Save(ctx, parentSession)
	if err != nil {
		return tools.ToolResponse{}, fmt.Errorf("error saving parent session: %s", err)
	}
	return tools.NewTextResponse(response.Content().String()), nil
}

// childDeps returns the deps for the agent this tool spawns: the delegation
// chain extended with this agent, so the child can never delegate back up.
func (b *agentTool) childDeps() AgentToolDeps {
	child := b.deps
	child.DelegationChain = append(append([]string{}, b.deps.DelegationChain...), callerName(b.caller))
	return child
}

// subagentByName resolves a subagent this agent may actually run. Hidden agents
// stay invokable even though they are not advertised, and agents already on the
// delegation chain are refused.
func (b *agentTool) subagentByName(name string) (*agentdef.Definition, bool) {
	def, ok := agentdef.Get().Get(name)
	if !ok || !canDelegateTo(b.caller, def, b.chain) {
		return nil, false
	}
	return def, true
}

// subagents lists the registry definitions caller may delegate to. Hidden
// agents are only returned when includeHidden is true; agents that deny the
// caller the "task" permission and agents already on the delegation chain are
// never returned.
func subagents(caller *agentdef.Definition, includeHidden bool, chain []string) []*agentdef.Definition {
	policy, err := permission.ParsePolicy(callerPermissions(caller))
	if err != nil {
		policy = permission.Policy{}
	}
	out := make([]*agentdef.Definition, 0)
	for _, def := range agentdef.Get().List() {
		if !canDelegateTo(caller, def, chain) {
			continue
		}
		if !includeHidden && def.Hidden {
			continue
		}
		if action, matched := policy.Task(def.Name); matched && action == permission.ActionDeny {
			continue
		}
		out = append(out, def)
	}
	return out
}

// canDelegateTo reports whether caller may run target as a subagent. Any
// registered agent is eligible — role (primary/subagent) only decides who may
// *be* a session agent, not who may be *launched*. The only hard refusals are:
// disabled agents, the internal helpers title/summarizer, the caller itself
// and anything already on the delegation chain (that is what bounds recursion,
// instead of an arbitrary depth limit).
func canDelegateTo(caller, target *agentdef.Definition, chain []string) bool {
	if target == nil || target.Disabled {
		return false
	}
	// Internal helpers are not meant to be run by other agents.
	switch target.Name {
	case string(config.AgentTitle), string(config.AgentSummarizer):
		return false
	}
	if caller != nil && caller.Name == target.Name {
		return false // no recursion into the same agent
	}
	for _, name := range chain {
		if name == target.Name {
			return false // would loop back up the chain
		}
	}
	return true
}

// agentToolFor builds the delegation tool for an agent, or nil when it should
// not get one: subagent-only agents (they are leaves), internal helpers and
// agents with nothing to delegate to.
func agentToolFor(caller *agentdef.Definition, deps AgentToolDeps) tools.BaseTool {
	if caller == nil || caller.Mode == agentdef.ModeSubagent {
		return nil
	}
	switch caller.Name {
	case string(config.AgentTitle), string(config.AgentSummarizer):
		return nil
	}
	if len(subagents(caller, true, deps.DelegationChain)) == 0 {
		return nil
	}
	return NewAgentTool(caller, deps)
}

func NewAgentTool(caller *agentdef.Definition, deps AgentToolDeps) tools.BaseTool {
	policy, _ := permission.ParsePolicy(callerPermissions(caller))
	return &agentTool{caller: caller, policy: policy, deps: deps, chain: deps.DelegationChain}
}

func callerPermissions(caller *agentdef.Definition) map[string]any {
	if caller == nil {
		return nil
	}
	return caller.Permissions
}

func callerName(caller *agentdef.Definition) string {
	if caller == nil {
		return "agent"
	}
	return caller.Name
}

func oneLine(text string) string {
	text = strings.Join(strings.Fields(text), " ")
	if text == "" {
		return "(no description)"
	}
	if len(text) > maxDescriptionLength {
		return text[:maxDescriptionLength] + "…"
	}
	return text
}

const toolGuidance = `Launch a new agent to work on a task autonomously and return its report to you. Pick which agent runs the task with the "subagent" parameter; the default is the read-only "task" agent. Any registered agent may be run by name, even when it is not listed below, and you may launch several in parallel in a single message (one tool use per agent).

When you are searching for a keyword or file and are not confident that you will find the right match on the first try, use this tool to perform the search for you. For example:

- If you are searching for a keyword like "config" or "logger", or for questions like "which file does X?", this tool is strongly recommended
- If you want to read a specific file path, use the View or Glob tool instead, to find the match more quickly
- If you are searching for a specific class definition like "class Foo", use the Glob tool instead, to find the match more quickly

Usage notes:
1. Launch multiple agents concurrently whenever possible, to maximize performance; to do that, use a single message with multiple tool uses
2. When the agent is done, it will return a single message back to you. The result returned by the agent is not visible to the user. To show the user the result, you should send a text message back to the user with a concise summary of the result.
3. Each agent invocation is stateless. You will not be able to send additional messages to the agent, nor will the agent be able to communicate with you outside of its final report. Therefore, your prompt should contain a highly detailed task description for the agent to perform autonomously and you should specify exactly what information the agent should return back to you in its final and only message to you.
4. The agent's outputs should generally be trusted
5. The tools a subagent has depend on its definition; when you need to change files, run the tools yourself instead of delegating.`
