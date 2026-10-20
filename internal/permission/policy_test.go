package permission

import (
	"context"
	"testing"

	"github.com/opencode-ai/opencode/internal/agentdef"
	"github.com/opencode-ai/opencode/internal/pubsub"
)

type fakeService struct {
	requests int
	last     CreatePermissionRequest
	answer   bool
}

func (s *fakeService) Subscribe(context.Context) <-chan pubsub.Event[PermissionRequest] {
	return nil
}
func (s *fakeService) GrantPersistant(PermissionRequest) {}
func (s *fakeService) Grant(PermissionRequest)           {}
func (s *fakeService) Deny(PermissionRequest)            {}
func (s *fakeService) AutoApproveSession(string)         {}
func (s *fakeService) Request(opts CreatePermissionRequest) bool {
	s.requests++
	s.last = opts
	return s.answer
}

// cmdParams stands in for tools.BashPermissionsParams, which is in a package
// that imports this one.
type cmdParams struct{ Cmd string }

func (p cmdParams) PermissionCommand() string { return p.Cmd }

// rule is a shorthand for a PermissionRule in tests.
func rule(action, resource, effect string) agentdef.PermissionRule {
	return agentdef.PermissionRule{Action: action, Resource: resource, Effect: effect}
}

func TestParsePolicy(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name      string
		perms     []agentdef.PermissionRule
		wantErr   bool
		wantRules int
	}{
		{name: "empty", perms: nil, wantRules: 0},
		{name: "single action", perms: []agentdef.PermissionRule{rule("edit", "", "deny")}, wantRules: 1},
		{name: "ordered rules", perms: []agentdef.PermissionRule{
			rule("shell", "*", "ask"),
			rule("shell", "git *", "allow"),
		}, wantRules: 2},
		{name: "task rules", perms: []agentdef.PermissionRule{
			rule("task", "*", "deny"),
			rule("task", "reviewer", "allow"),
		}, wantRules: 2},
		{name: "effect is case insensitive", perms: []agentdef.PermissionRule{rule("shell", "", "Allow")}, wantRules: 1},
		{name: "invalid effect", perms: []agentdef.PermissionRule{rule("edit", "", "maybe")}, wantErr: true},
		{name: "empty effect", perms: []agentdef.PermissionRule{rule("edit", "", "")}, wantErr: true},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			policy, err := ParsePolicy(tc.perms)
			if tc.wantErr {
				if err == nil {
					t.Fatalf("expected an error, got policy %+v", policy)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if len(policy.rules) != tc.wantRules {
				t.Errorf("rules = %d, want %d (%+v)", len(policy.rules), tc.wantRules, policy.rules)
			}
			if len(tc.perms) == 0 && policy.HasRules {
				t.Error("empty policy must not report HasRules")
			}
			if len(tc.perms) > 0 && !policy.HasRules {
				t.Error("parsed policy must report HasRules")
			}
		})
	}
}

func TestPolicyDecide(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name        string
		perms       []agentdef.PermissionRule
		tool        string
		params      any
		wantAction  Action
		wantMatched bool
	}{
		{
			name: "no policy never matches",
			tool: "bash",
		},
		{
			name:        "edit key covers write",
			perms:       []agentdef.PermissionRule{rule("edit", "", "deny")},
			tool:        "write",
			wantAction:  ActionDeny,
			wantMatched: true,
		},
		{
			name:        "edit key covers patch",
			perms:       []agentdef.PermissionRule{rule("edit", "", "deny")},
			tool:        "patch",
			wantAction:  ActionDeny,
			wantMatched: true,
		},
		{
			name:        "edit key does not cover bash",
			perms:       []agentdef.PermissionRule{rule("edit", "", "deny")},
			tool:        "bash",
			wantMatched: false,
		},
		{
			name:        "unknown tool keeps its own name as key",
			perms:       []agentdef.PermissionRule{rule("mysrv_search", "", "deny")},
			tool:        "mysrv_search",
			wantAction:  ActionDeny,
			wantMatched: true,
		},
		{
			name:        "rule may name the tool directly",
			perms:       []agentdef.PermissionRule{rule("bash", "", "deny")},
			tool:        "bash",
			wantAction:  ActionDeny,
			wantMatched: true,
		},
		{
			name:        "effect is case insensitive",
			perms:       []agentdef.PermissionRule{rule("webfetch", "", "Allow")},
			tool:        "fetch",
			wantAction:  ActionAllow,
			wantMatched: true,
		},
		{
			name: "first matching rule wins",
			perms: []agentdef.PermissionRule{
				rule("shell", "", "deny"),
				rule("shell", "git *", "allow"),
			},
			tool:        "bash",
			params:      "git status",
			wantAction:  ActionDeny,
			wantMatched: true,
		},
		{
			name: "specific rule first wins",
			perms: []agentdef.PermissionRule{
				rule("shell", "git *", "allow"),
				rule("shell", "", "deny"),
			},
			tool:        "bash",
			params:      "git status",
			wantAction:  ActionAllow,
			wantMatched: true,
		},
		{
			name: "non matching resource falls through to catch all",
			perms: []agentdef.PermissionRule{
				rule("shell", "git *", "allow"),
				rule("shell", "", "deny"),
			},
			tool:        "bash",
			params:      "rm -rf /tmp/cache",
			wantAction:  ActionDeny,
			wantMatched: true,
		},
		{
			name: "resource matched against the command",
			perms: []agentdef.PermissionRule{
				rule("shell", "*", "ask"),
				rule("shell", "git *", "allow"),
			},
			tool:        "bash",
			params:      "rm -rf /tmp/cache",
			wantAction:  ActionAsk,
			wantMatched: true,
		},
		{
			name: "command may come from a CommandProvider",
			perms: []agentdef.PermissionRule{
				rule("shell", "git status", "allow"),
				rule("shell", "", "deny"),
			},
			tool:        "bash",
			params:      cmdParams{Cmd: "git status"},
			wantAction:  ActionAllow,
			wantMatched: true,
		},
		{
			name: "no catch all falls through",
			perms: []agentdef.PermissionRule{
				rule("shell", "git *", "allow"),
			},
			tool:        "bash",
			params:      "rm -rf /",
			wantMatched: false,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			policy, err := ParsePolicy(tc.perms)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			action, matched := policy.Decide(CreatePermissionRequest{
				ToolName: tc.tool,
				Params:   tc.params,
			})
			if matched != tc.wantMatched {
				t.Fatalf("matched = %v, want %v", matched, tc.wantMatched)
			}
			if matched && action != tc.wantAction {
				t.Errorf("action = %q, want %q", action, tc.wantAction)
			}
		})
	}
}

func TestMatchWildcard(t *testing.T) {
	t.Parallel()

	cases := []struct {
		pattern string
		value   string
		want    bool
	}{
		{"*", "anything", true},
		{"*", "", true},
		{"bash", "bash", true},
		{"bash", "bas", false},
		{"bash", "bash ", false},
		{"git *", "git status", true},
		{"git *", "git -C /tmp status", true},
		{"git *", "gitt", false},
		{"mymcp_*", "mymcp_search", true},
		{"mymcp_*", "mymcp_", true},
		{"mymcp_*", "other_search", false},
		{"mymcp_foo_*", "mymcp_bar_x", false},
		{"a*b*c", "abc", true},
		{"a*b*c", "axxbxxc", true},
		{"a*b*c", "acb", false},
		{"a?c", "abc", true},
		{"a?c", "ac", false},
		{"", "", true},
		{"", "a", false},
	}

	for _, tc := range cases {
		if got := MatchWildcard(tc.pattern, tc.value); got != tc.want {
			t.Errorf("MatchWildcard(%q, %q) = %v, want %v", tc.pattern, tc.value, got, tc.want)
		}
	}
}

func TestPermissionKey(t *testing.T) {
	t.Parallel()

	cases := map[string]string{
		"bash":        "shell",
		"shell":       "shell",
		"write":       "edit",
		"edit":        "edit",
		"patch":       "edit",
		"view":        "read",
		"ls":          "list",
		"fetch":       "webfetch",
		"sourcegraph": "webfetch",
		"agent":       "task",
		"mysrv_tool":  "mysrv_tool",
	}
	for tool, want := range cases {
		if got := PermissionKey(tool); got != want {
			t.Errorf("PermissionKey(%q) = %q, want %q", tool, got, want)
		}
	}
}

func TestPolicyTask(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name        string
		perms       []agentdef.PermissionRule
		subagent    string
		wantAction  Action
		wantMatched bool
	}{
		{name: "no policy never matches", subagent: "task"},
		{
			name:        "unrelated key never matches",
			perms:       []agentdef.PermissionRule{rule("edit", "", "deny")},
			subagent:    "task",
			wantMatched: false,
		},
		{
			name:        "plain action applies to every subagent",
			perms:       []agentdef.PermissionRule{rule("task", "", "deny")},
			subagent:    "task",
			wantAction:  ActionDeny,
			wantMatched: true,
		},
		{
			name: "specific subagent rule first wins",
			perms: []agentdef.PermissionRule{
				rule("task", "reviewer", "allow"),
				rule("task", "", "deny"),
			},
			subagent:    "reviewer",
			wantAction:  ActionAllow,
			wantMatched: true,
		},
		{
			name: "catch all denies the rest",
			perms: []agentdef.PermissionRule{
				rule("task", "reviewer", "allow"),
				rule("task", "", "deny"),
			},
			subagent:    "planner",
			wantAction:  ActionDeny,
			wantMatched: true,
		},
		{
			name: "no catch all has no opinion",
			perms: []agentdef.PermissionRule{
				rule("task", "reviewer", "deny"),
			},
			subagent:    "planner",
			wantMatched: false,
		},
		{
			name: "ask is reported as is",
			perms: []agentdef.PermissionRule{
				rule("task", "", "ask"),
			},
			subagent:    "anyone",
			wantAction:  ActionAsk,
			wantMatched: true,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			policy, err := ParsePolicy(tc.perms)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			action, matched := policy.Task(tc.subagent)
			if matched != tc.wantMatched {
				t.Fatalf("matched = %v, want %v", matched, tc.wantMatched)
			}
			if matched && action != tc.wantAction {
				t.Errorf("action = %q, want %q", action, tc.wantAction)
			}
		})
	}
}

func TestNewAgentScoped(t *testing.T) {
	t.Parallel()

	deny, err := ParsePolicy([]agentdef.PermissionRule{rule("edit", "", "deny")})
	if err != nil {
		t.Fatal(err)
	}
	allow, err := ParsePolicy([]agentdef.PermissionRule{rule("edit", "", "allow")})
	if err != nil {
		t.Fatal(err)
	}
	ask, err := ParsePolicy([]agentdef.PermissionRule{rule("edit", "", "ask")})
	if err != nil {
		t.Fatal(err)
	}

	t.Run("deny rejects without asking the base", func(t *testing.T) {
		base := &fakeService{answer: true}
		svc := NewAgentScoped(base, deny)
		if svc.Request(CreatePermissionRequest{ToolName: "write"}) {
			t.Fatal("deny must reject")
		}
		if base.requests != 0 {
			t.Fatalf("base was called %d times", base.requests)
		}
	})

	t.Run("allow approves without asking the base", func(t *testing.T) {
		base := &fakeService{answer: false}
		svc := NewAgentScoped(base, allow)
		if !svc.Request(CreatePermissionRequest{ToolName: "write"}) {
			t.Fatal("allow must approve")
		}
		if base.requests != 0 {
			t.Fatalf("base was called %d times", base.requests)
		}
	})

	t.Run("ask falls back to the base", func(t *testing.T) {
		base := &fakeService{answer: true}
		svc := NewAgentScoped(base, ask)
		if !svc.Request(CreatePermissionRequest{ToolName: "write"}) {
			t.Fatal("ask must keep the interactive flow")
		}
		if base.requests != 1 {
			t.Fatalf("base was called %d times, want 1", base.requests)
		}
	})

	t.Run("unmatched tool falls back to the base", func(t *testing.T) {
		base := &fakeService{answer: true}
		svc := NewAgentScoped(base, deny)
		if !svc.Request(CreatePermissionRequest{ToolName: "glob"}) {
			t.Fatal("unmatched tools keep the default flow")
		}
		if base.requests != 1 {
			t.Fatalf("base was called %d times, want 1", base.requests)
		}
	})

	t.Run("empty policy returns the base unchanged", func(t *testing.T) {
		base := &fakeService{}
		if NewAgentScoped(base, Policy{}) != base {
			t.Fatal("an empty policy must not wrap the service")
		}
	})
}
