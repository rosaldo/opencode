package permission

import (
	"fmt"
	"strings"
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

// bashPermissionKey is the permission key gated by the bash tool. It also
// accepts command patterns instead of a single action.
const bashPermissionKey = "bash"

// Rule pairs a wildcard pattern with the action it triggers.
type Rule struct {
	Pattern string
	Action  Action
}

// Policy is the parsed form of an agent's permissions block. Patterns are
// matched against permission keys (see PermissionKey) or, for the keys that
// accept a map ("bash" for shell commands, "task" for subagent names), against
// the pattern the request carries.
//
// The zero Policy is valid and means "no opinion": every request falls through
// to the wrapped service.
type Policy struct {
	// rules hold the permission keys, including the ones that take a map (the
	// entry for such a key carries the action of its "*" pattern).
	rules []Rule
	// subRules hold the patterns of the map valued keys.
	subRules map[string][]Rule
	// subDefault holds the "*" action of each map valued key.
	subDefault map[string]Action

	// HasRules is false when nothing was configured.
	HasRules bool
}

// ParsePolicy converts an agent's permissions map into a Policy.
//
// Values are "allow", "ask" or "deny". The "bash" and "task" keys additionally
// accept a map of patterns to actions, e.g. {"*": "ask", "git *": "allow"} for
// bash commands or {"*": "deny", "reviewer": "allow"} for subagents.
func ParsePolicy(perms map[string]any) (Policy, error) {
	var p Policy
	if len(perms) == 0 {
		return p, nil
	}
	p.HasRules = true
	for key, raw := range perms {
		switch v := raw.(type) {
		case string:
			action, err := parseAction(key, v)
			if err != nil {
				return Policy{}, err
			}
			p.rules = append(p.rules, Rule{Pattern: key, Action: action})
		case map[string]any:
			if err := p.addSubRules(key, v); err != nil {
				return Policy{}, err
			}
		case map[string]string:
			if err := p.addSubRules(key, stringMapToAny(v)); err != nil {
				return Policy{}, err
			}
		default:
			return Policy{}, fmt.Errorf("permissions.%s: unsupported value %T", key, raw)
		}
	}
	return p, nil
}

func (p *Policy) addSubRules(key string, m map[string]any) error {
	if p.subRules == nil {
		p.subRules = make(map[string][]Rule)
		p.subDefault = make(map[string]Action)
	}
	rules := make([]Rule, 0, len(m))
	for pattern, raw := range m {
		value, ok := raw.(string)
		if !ok {
			return fmt.Errorf("permissions.%s[%s]: expected allow, ask or deny", key, pattern)
		}
		action, err := parseAction(key+"."+pattern, value)
		if err != nil {
			return err
		}
		rules = append(rules, Rule{Pattern: pattern, Action: action})
		if pattern == "*" {
			p.subDefault[key] = action
		}
	}
	p.subRules[key] = rules
	// Registering the key itself (even with an empty default) is what makes
	// Decide and Task reach the patterns below.
	p.rules = append(p.rules, Rule{Pattern: key, Action: p.subDefault[key]})
	return nil
}

// Decide returns the action the policy dictates for a request. The second
// result is false when the policy has nothing to say about the request, which
// means the caller should keep its default behaviour.
func (p Policy) Decide(req CreatePermissionRequest) (Action, bool) {
	if !p.HasRules {
		return "", false
	}
	key := PermissionKey(req.ToolName)
	action, matched := matchRule(p.rules, key)
	if !matched {
		return "", false
	}
	if patterns, ok := p.subRules[key]; ok && len(patterns) > 0 {
		action = p.subDefault[key]
		if target := targetFromParams(req.Params); target != "" {
			if refined, ok := matchRule(patterns, target); ok {
				action = refined
			}
		}
	}
	if action == "" {
		return "", false
	}
	return action, true
}

// Task returns the action the policy dictates for delegating work to the
// subagent with the given name. It reads the "task" key, which may be a plain
// action or a map of subagent names to actions. As with Decide, a false second
// result means "no opinion".
func (p Policy) Task(subagent string) (Action, bool) {
	if !p.HasRules {
		return "", false
	}
	action, matched := matchRule(p.rules, "task")
	if !matched {
		return "", false
	}
	if patterns, ok := p.subRules["task"]; ok && len(patterns) > 0 {
		action = p.subDefault["task"]
		if refined, ok := matchRule(patterns, subagent); ok {
			action = refined
		}
	}
	if action == "" {
		return "", false
	}
	return action, true
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
	bashPermissionKey: bashPermissionKey,
	"write":           "edit",
	"edit":            "edit",
	"patch":           "edit",
	"view":            "read",
	"ls":              "list",
	"fetch":           "webfetch",
	"sourcegraph":     "webfetch",
	"agent":           "task",
}

// CommandProvider is implemented by tool parameter structs that carry the value
// a map valued permission key is matched against: the shell command for bash,
// the subagent name for the agent tool. It exists so this package can read it
// out of a request without importing internal/llm/tools, which depends on this
// package.
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

// matchRule returns the action of the most specific pattern matching value. An
// exact match always wins; among wildcards the one with the longest literal part
// wins, so "git *" beats "*" regardless of map iteration order.
func matchRule(rules []Rule, value string) (Action, bool) {
	var best Action
	bestScore := -1
	found := false
	for _, r := range rules {
		if !MatchWildcard(r.Pattern, value) {
			continue
		}
		if score := wildcardScore(r.Pattern, value); score > bestScore {
			best, bestScore, found = r.Action, score, true
		}
	}
	return best, found
}

func wildcardScore(pattern, value string) int {
	if pattern == value {
		return 1<<30 + len(pattern)
	}
	return len(strings.ReplaceAll(pattern, "*", ""))
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

func parseAction(what, value string) (Action, error) {
	switch action := Action(strings.ToLower(strings.TrimSpace(value))); action {
	case ActionAllow, ActionAsk, ActionDeny:
		return action, nil
	case "":
		return "", fmt.Errorf("permissions.%s: empty action", what)
	default:
		return "", fmt.Errorf("permissions.%s: invalid action %q, expected allow, ask or deny", what, value)
	}
}

func stringMapToAny(in map[string]string) map[string]any {
	out := make(map[string]any, len(in))
	for k, v := range in {
		out[k] = v
	}
	return out
}
