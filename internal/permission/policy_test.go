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
			name: "last matching rule wins",
			perms: []agentdef.PermissionRule{
				rule("shell", "", "deny"),
				rule("shell", "git *", "allow"),
			},
			tool:        "bash",
			params:      "git status",
			wantAction:  ActionAllow,
			wantMatched: true,
		},
		{
			name: "catch all first, specific after",
			perms: []agentdef.PermissionRule{
				rule("shell", "*", "ask"),
				rule("shell", "git push *", "deny"),
			},
			tool:        "bash",
			params:      "git push origin main",
			wantAction:  ActionDeny,
			wantMatched: true,
		},
		{
			name: "specific first is overridden by later catch all",
			perms: []agentdef.PermissionRule{
				rule("shell", "git *", "allow"),
				rule("shell", "", "deny"),
			},
			tool:        "bash",
			params:      "git status",
			wantAction:  ActionDeny,
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
				rule("shell", "", "deny"),
				rule("shell", "git status", "allow"),
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
			name: "catch all first, specific after",
			perms: []agentdef.PermissionRule{
				rule("task", "", "deny"),
				rule("task", "reviewer", "allow"),
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

func TestPolicyProviderAndTool(t *testing.T) {
	t.Parallel()

	p, err := ParsePolicy([]agentdef.PermissionRule{
		rule("provider.use", "*", "deny"),
		rule("provider.use", "anthropic", "allow"),
		rule("tool.use", "shell:git push *", "deny"),
	})
	if err != nil {
		t.Fatal(err)
	}

	if action, ok := p.Provider("openai"); !ok || action != ActionDeny {
		t.Errorf("openai = %v %v, want deny", action, ok)
	}
	if action, ok := p.Provider("anthropic"); !ok || action != ActionAllow {
		t.Errorf("anthropic = %v %v, want allow", action, ok)
	}
	if action, ok := p.Tool("shell:git push origin main"); !ok || action != ActionDeny {
		t.Errorf("git push = %v %v, want deny", action, ok)
	}
	if _, ok := p.Tool("shell:git status"); ok {
		t.Errorf("git status should have no opinion")
	}
	if _, ok := p.Decide(CreatePermissionRequest{ToolName: "bash", Params: "git status"}); ok {
		t.Errorf("provider.use/tool.use rules must not leak into Decide")
	}
}

func TestAgentScopedLastWins(t *testing.T) {
	t.Parallel()

	base := &fakeService{answer: true}
	policy, err := ParsePolicy([]agentdef.PermissionRule{
		rule("shell", "*", "ask"),
		rule("shell", "git push *", "deny"),
	})
	if err != nil {
		t.Fatal(err)
	}
	svc := NewAgentScoped(base, policy)

	if svc.Request(CreatePermissionRequest{ToolName: "bash", Params: "git push origin main"}) {
		t.Error("git push should be denied by the later rule")
	}
	if !svc.Request(CreatePermissionRequest{ToolName: "bash", Params: "git status"}) {
		t.Error("git status should ask (deferring to the base service)")
	}
}

func TestPolicyScopedToolUse(t *testing.T) {
	t.Parallel()

	base := &fakeService{answer: true}
	policy, err := ParsePolicy([]agentdef.PermissionRule{
		rule("tool.use", "shell:git push *", "deny"),
	})
	if err != nil {
		t.Fatal(err)
	}
	svc := NewPolicyScoped(base, policy)

	if svc.Request(CreatePermissionRequest{ToolName: "bash", Params: cmdParams{Cmd: "git push origin main"}}) {
		t.Error("git push should be blocked by the tool.use policy")
	}
	if !svc.Request(CreatePermissionRequest{ToolName: "bash", Params: cmdParams{Cmd: "git status"}}) {
		t.Error("git status should defer to the base service")
	}
}
