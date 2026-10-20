package permission

import (
	"fmt"
	"strings"

	"github.com/opencode-ai/opencode/internal/agentdef"
)

// Action is the outcome of a permission policy rule.
type Action string

const (
	// ActionAllow approves the request without prompting the user.
	ActionAllow Action = "allow"
	// ActionAsk defers to the interactive permission flow.
	ActionAsk Action = "ask"
	// ActionDeny rejects the request without prompting the user.
	ActionDeny Action = "deny"
)

// Rule is one ordered entry of a policy: for requests whose permission key (see
// PermissionKey) equals Action and whose value (shell command, subagent name,
// …) matches Resource, the policy replies Effect. An empty Resource matches any
// value, including none.
//
// Rules are evaluated in order and the LAST matching rule wins (opencode v2), so
// the common shape is a catch-all first and more specific rules after it.
type Rule struct {
	Action   string
	Resource string
	Effect   Action
}

// Policy is the parsed form of a permissions list (opencode v2). The list is
// ordered and the last rule that matches a request wins.
//
// The zero Policy is valid and means "no opinion": every request falls through
// to the wrapped service.
type Policy struct {
	rules []Rule
	// HasRules is false when nothing was configured.
	HasRules bool
}

// ParsePolicy converts an agent's ordered permission rules into a Policy.
// Every rule's Effect must be "allow", "ask" or "deny".
func ParsePolicy(perms []agentdef.PermissionRule) (Policy, error) {
	var p Policy
	if len(perms) == 0 {
		return p, nil
	}
	p.HasRules = true
	p.rules = make([]Rule, 0, len(perms))
	for i, raw := range perms {
		action, err := parseAction(i, raw.Effect)
		if err != nil {
			return Policy{}, err
		}
		p.rules = append(p.rules, Rule{
			Action:   strings.TrimSpace(raw.Action),
			Resource: raw.Resource,
			Effect:   action,
		})
	}
	return p, nil
}

// Decide returns the action the policy dictates for a request. The second
// result is false when the policy has nothing to say about the request, which
// means the caller should keep its default behaviour. The last matching rule
// wins.
func (p Policy) Decide(req CreatePermissionRequest) (Action, bool) {
	if !p.HasRules {
		return "", false
	}
	key := PermissionKey(req.ToolName)
	value := targetFromParams(req.Params)
	var (
		effect  Action
		matched bool
	)
	for _, rule := range p.rules {
		if rule.Action != key && rule.Action != req.ToolName {
			continue
		}
		if rule.Resource != "" && !MatchWildcard(rule.Resource, value) {
			continue
		}
		effect, matched = rule.Effect, true
	}
	return effect, matched
}

// Task returns the action the policy dictates for delegating work to the
// subagent with the given name. It reads the rules whose Action is "task" and
// whose Resource matches the name; a false second result means "no opinion".
// The last matching rule wins.
func (p Policy) Task(subagent string) (Action, bool) {
	if !p.HasRules {
		return "", false
	}
	var (
		effect  Action
		matched bool
	)
	for _, rule := range p.rules {
		if rule.Action != "task" {
			continue
		}
		if rule.Resource != "" && !MatchWildcard(rule.Resource, subagent) {
			continue
		}
		effect, matched = rule.Effect, true
	}
	return effect, matched
}

// Provider returns the action the policies dictate for using the named
// provider (policy action "provider.use"). A false second result means no
// opinion. Last matching rule wins.
func (p Policy) Provider(provider string) (Action, bool) {
	return p.decideNamed("provider.use", provider)
}

// Tool returns the action the policies dictate for using a tool, where target
// is "tool" or "tool:value" (for example "shell:git push *", policy action
// "tool.use"). A false second result means no opinion. Last matching rule wins.
func (p Policy) Tool(target string) (Action, bool) {
	return p.decideNamed("tool.use", target)
}

// decideNamed evaluates rules whose Action equals name, matching Resource
// against value; the last matching rule wins.
func (p Policy) decideNamed(name, value string) (Action, bool) {
	if !p.HasRules {
		return "", false
	}
	var (
		effect  Action
		matched bool
	)
	for _, rule := range p.rules {
		if rule.Action != name {
			continue
		}
		if rule.Resource != "" && !MatchWildcard(rule.Resource, value) {
			continue
		}
		effect, matched = rule.Effect, true
	}
	return effect, matched
}

// PermissionKey maps a tool name to the permission key gating it, mirroring the
// TypeScript implementation (the "edit" key gates write, edit and patch). MCP
// tools keep their own "server_tool" name as the key.
func PermissionKey(toolName string) string {
	if key, ok := permissionKeys[toolName]; ok {
		return key
	}
	return toolName
}

var permissionKeys = map[string]string{
	bashPermissionKey: shellPermissionKey,
	"shell":           shellPermissionKey,
	"write":           "edit",
	"edit":            "edit",
	"patch":           "edit",
	"view":            "read",
	"ls":              "list",
	"fetch":           "webfetch",
	"sourcegraph":     "webfetch",
	"agent":           "task",
}

// bashPermissionKey is the tool name of the shell tool; shellPermissionKey is
// the v2 permission action that gates it.
const (
	bashPermissionKey  = "bash"
	shellPermissionKey = "shell"
)

// CommandProvider is implemented by tool parameter structs that carry the value
// a rule resource is matched against: the shell command for bash, the subagent
// name for the agent tool. It exists so this package can read it out of a
// request without importing internal/llm/tools, which depends on this package.
type CommandProvider interface {
	PermissionCommand() string
}

func targetFromParams(params any) string {
	switch v := params.(type) {
	case string:
		return v
	case CommandProvider:
		return v.PermissionCommand()
	}
	return ""
}

// MatchWildcard matches value against pattern, where "*" matches any run of
// characters (including path separators, unlike path.Match, so that "git *"
// also matches "git -C /tmp status") and "?" matches a single one.
func MatchWildcard(pattern, value string) bool {
	for len(pattern) > 0 {
		switch pattern[0] {
		case '*':
			pattern = strings.TrimLeft(pattern, "*")
			if pattern == "" {
				return true
			}
			for i := 0; i <= len(value); i++ {
				if MatchWildcard(pattern, value[i:]) {
					return true
				}
			}
			return false
		case '?':
			if value == "" {
				return false
			}
			pattern, value = pattern[1:], value[1:]
		default:
			if value == "" || value[0] != pattern[0] {
				return false
			}
			pattern, value = pattern[1:], value[1:]
		}
	}
	return value == ""
}

func parseAction(index int, value string) (Action, error) {
	switch action := Action(strings.ToLower(strings.TrimSpace(value))); action {
	case ActionAllow, ActionAsk, ActionDeny:
		return action, nil
	case "":
		return "", fmt.Errorf("permissions[%d]: empty effect", index)
	default:
		return "", fmt.Errorf("permissions[%d]: invalid effect %q, expected allow, ask or deny", index, value)
	}
}
