package permission

import (
	"context"
	"testing"

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

func TestParsePolicy(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name      string
		perms     map[string]any
		wantErr   bool
		wantRules int
	}{
		{name: "empty", perms: nil, wantRules: 0},
		{name: "single action", perms: map[string]any{"edit": "deny"}, wantRules: 1},
		{name: "bash map", perms: map[string]any{
			"bash": map[string]any{"*": "ask", "git *": "allow"},
		}, wantRules: 3},
		{name: "task map", perms: map[string]any{
			"task": map[string]any{"*": "deny", "reviewer": "allow"},
		}, wantRules: 3},
		{name: "bash as string", perms: map[string]any{"bash": "allow"}, wantRules: 1},
		{name: "invalid action", perms: map[string]any{"edit": "maybe"}, wantErr: true},
		{name: "invalid type", perms: map[string]any{"edit": 42}, wantErr: true},
		{name: "bash map with bad action", perms: map[string]any{
			"bash": map[string]any{"*": "nope"},
		}, wantErr: true},
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
			gotRules := len(policy.rules) + len(policy.subRules["bash"]) + len(policy.subRules["task"])
			if gotRules != tc.wantRules {
				t.Errorf("rules = %d, want %d (%+v sub %+v)", gotRules, tc.wantRules, policy.rules, policy.subRules)
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
		perms       map[string]any
		tool        string
		params      any
		wantAction  Action
		wantMatched bool
	}{
		{
			name:  "no policy never matches",
			perms: nil,
			tool:  "bash",
		},
		{
			name:        "edit key covers write",
			perms:       map[string]any{"edit": "deny"},
			tool:        "write",
			wantAction:  ActionDeny,
			wantMatched: true,
		},
		{
			name:        "edit key covers patch",
			perms:       map[string]any{"edit": "deny"},
			tool:        "patch",
			wantAction:  ActionDeny,
			wantMatched: true,
		},
		{
			name:        "edit key does not cover bash",
			perms:       map[string]any{"edit": "deny"},
			tool:        "bash",
			wantMatched: false,
		},
		{
			name:        "unknown tool keeps its own name as key",
			perms:       map[string]any{"mysrv_search": "deny"},
			tool:        "mysrv_search",
			wantAction:  ActionDeny,
			wantMatched: true,
		},
		{
			name:        "action is case insensitive",
			perms:       map[string]any{"webfetch": "Allow"},
			tool:        "fetch",
			wantAction:  ActionAllow,
			wantMatched: true,
		},
		{
			name:        "exact rule beats catch all",
			perms:       map[string]any{"bash": "deny", "*": "allow"},
			tool:        "bash",
			wantAction:  ActionDeny,
			wantMatched: true,
		},
		{
			name:        "wildcard covers mcp tools",
			perms:       map[string]any{"mysrv_*": "deny"},
			tool:        "mysrv_search",
			wantAction:  ActionDeny,
			wantMatched: true,
		},
		{
			name:        "wildcard does not cover other servers",
			perms:       map[string]any{"mysrv_*": "deny"},
			tool:        "othersrv_search",
			wantMatched: false,
		},
		{
			name:        "longest wildcard wins",
			perms:       map[string]any{"mysrv_*": "ask", "mysrv_write_*": "deny"},
			tool:        "mysrv_write_file",
			wantAction:  ActionDeny,
			wantMatched: true,
		},
		{
			name: "bash map asks by default",
			perms: map[string]any{
				"bash": map[string]any{"*": "ask", "git *": "allow"},
			},
			tool:        "bash",
			params:      "rm -rf /tmp/cache",
			wantAction:  ActionAsk,
			wantMatched: true,
		},
		{
			name: "bash map allows matching commands",
			perms: map[string]any{
				"bash": map[string]any{"*": "ask", "git *": "allow"},
			},
			tool:        "bash",
			params:      "git commit -m 'nope'",
			wantAction:  ActionAllow,
			wantMatched: true,
		},
		{
			name: "bash command may come from a CommandProvider",
			perms: map[string]any{
				"bash": map[string]any{"*": "deny", "git status": "allow"},
			},
			tool:        "bash",
			params:      cmdParams{Cmd: "git status"},
			wantAction:  ActionAllow,
			wantMatched: true,
		},
		{
			name: "bash map without catch all falls through",
			perms: map[string]any{
				"bash": map[string]any{"git *": "allow"},
			},
			tool:        "bash",
			params:      "rm -rf /",
			wantMatched: false,
		},
		{
			name:        "bash plain string needs no command",
			perms:       map[string]any{"bash": "deny"},
			tool:        "bash",
			params:      "git status",
			wantAction:  ActionDeny,
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
		"bash":        "bash",
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
		perms       map[string]any
		subagent    string
		wantAction  Action
		wantMatched bool
	}{
		{name: "no policy never matches", perms: nil, subagent: "task"},
		{
			name:        "unrelated key never matches",
			perms:       map[string]any{"edit": "deny"},
			subagent:    "task",
			wantMatched: false,
		},
		{
			name:        "plain action applies to every subagent",
			perms:       map[string]any{"task": "deny"},
			subagent:    "task",
			wantAction:  ActionDeny,
			wantMatched: true,
		},
		{
			name: "plain action applies to a named subagent",
			perms: map[string]any{
				"task": map[string]any{"*": "deny", "reviewer": "allow"},
			},
			subagent:    "reviewer",
			wantAction:  ActionAllow,
			wantMatched: true,
		},
		{
			name: "catch all denies the rest",
			perms: map[string]any{
				"task": map[string]any{"*": "deny", "reviewer": "allow"},
			},
			subagent:    "planner",
			wantAction:  ActionDeny,
			wantMatched: true,
		},
		{
			name: "map without catch all has no opinion",
			perms: map[string]any{
				"task": map[string]any{"reviewer": "deny"},
			},
			subagent:    "planner",
			wantMatched: false,
		},
		{
			name: "ask is reported as is",
			perms: map[string]any{
				"task": map[string]any{"*": "ask"},
			},
			subagent:    "anyone",
			wantAction:  ActionAsk,
			wantMatched: true,
		},
		{
			name:        "longest pattern wins",
			perms:       map[string]any{"task": map[string]any{"rev*": "deny", "reviewer": "allow"}},
			subagent:    "reviewer",
			wantAction:  ActionAllow,
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

	deny, err := ParsePolicy(map[string]any{"edit": "deny"})
	if err != nil {
		t.Fatal(err)
	}
	allow, err := ParsePolicy(map[string]any{"edit": "allow"})
	if err != nil {
		t.Fatal(err)
	}
	ask, err := ParsePolicy(map[string]any{"edit": "ask"})
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
