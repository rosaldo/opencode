package agent

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"testing"

	"github.com/opencode-ai/opencode/internal/agentdef"
	"github.com/opencode-ai/opencode/internal/config"
	"github.com/opencode-ai/opencode/internal/llm/tools"
	"github.com/opencode-ai/opencode/internal/permission"
)

type fakeTool struct{ name string }

func (t fakeTool) Info() tools.ToolInfo { return tools.ToolInfo{Name: t.name} }
func (t fakeTool) Run(context.Context, tools.ToolCall) (tools.ToolResponse, error) {
	return tools.ToolResponse{}, nil
}

// withTestRegistry installs a process wide registry for the duration of the
// test, mirroring the builtins config.Load would register.
func withTestRegistry(t *testing.T, defs ...*agentdef.Definition) *agentdef.Registry {
	t.Helper()
	previous := agentdef.Get()
	reg := agentdef.NewRegistry()
	for _, def := range defs {
		if err := reg.Register(def); err != nil {
			t.Fatalf("failed to register %s: %v", def.Name, err)
		}
	}
	agentdef.Set(reg)
	t.Cleanup(func() { agentdef.Set(previous) })
	return reg
}

func builtinDefs() []*agentdef.Definition {
	return []*agentdef.Definition{
		{Name: string(config.AgentCoder), Mode: agentdef.ModePrimary, Builtin: true},
		{Name: string(config.AgentTask), Mode: agentdef.ModeSubagent, Hidden: true, Builtin: true},
		{Name: string(config.AgentTitle), Mode: agentdef.ModeSubagent, Hidden: true, Builtin: true},
		{Name: string(config.AgentSummarizer), Mode: agentdef.ModeSubagent, Hidden: true, Builtin: true},
	}
}

func toolNames(list []tools.BaseTool) map[string]bool {
	names := make(map[string]bool, len(list))
	for _, tool := range list {
		names[tool.Info().Name] = true
	}
	return names
}

func TestToolEnabled(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name  string
		rules map[string]bool
		tool  string
		want  bool
	}{
		{name: "no rules keeps everything", rules: nil, tool: "bash", want: true},
		{name: "deny list removes one", rules: map[string]bool{"bash": false}, tool: "bash", want: false},
		{name: "deny list keeps the rest", rules: map[string]bool{"bash": false}, tool: "glob", want: true},
		{name: "deny list with several entries", rules: map[string]bool{
			"write": false, "edit": false, "bash": false,
		}, tool: "edit", want: false},
		{name: "deny list keeps unrelated", rules: map[string]bool{
			"write": false, "edit": false, "bash": false,
		}, tool: "view", want: true},
		{name: "catch all true keeps everything", rules: map[string]bool{"*": true}, tool: "bash", want: true},
		{name: "catch all false removes everything", rules: map[string]bool{"*": false}, tool: "glob", want: false},
		{name: "exact entry beats catch all", rules: map[string]bool{"*": false, "bash": true}, tool: "bash", want: true},
		{name: "exact entry beats catch all the other way", rules: map[string]bool{"*": true, "bash": false}, tool: "bash", want: false},
		{name: "allow list keeps the entry", rules: map[string]bool{"view": true}, tool: "view", want: true},
		{name: "allow list drops the rest", rules: map[string]bool{"view": true}, tool: "bash", want: false},
		{name: "wildcard deny", rules: map[string]bool{"mymcp_*": false}, tool: "mymcp_search", want: false},
		{name: "wildcard deny keeps other servers", rules: map[string]bool{"mymcp_*": false}, tool: "other_search", want: true},
		{name: "longest wildcard wins", rules: map[string]bool{
			"mymcp_*": false, "mymcp_write_*": true,
		}, tool: "mymcp_write_file", want: true},
		{name: "longest wildcard loses the rest of the server", rules: map[string]bool{
			"mymcp_*": false, "mymcp_write_*": true,
		}, tool: "mymcp_search", want: false},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if got := ToolEnabled(tc.rules, tc.tool); got != tc.want {
				t.Errorf("ToolEnabled(%v, %q) = %v, want %v", tc.rules, tc.tool, got, tc.want)
			}
		})
	}
}

func TestFilterTools(t *testing.T) {
	t.Parallel()

	list := []tools.BaseTool{
		fakeTool{"bash"}, fakeTool{"edit"}, fakeTool{"glob"},
	}
	names := toolNames(FilterTools(list, map[string]bool{"bash": false, "edit": false}))
	if len(names) != 1 || !names["glob"] {
		t.Fatalf("names = %v, want only glob", names)
	}

	unchanged := FilterTools(list, nil)
	if len(unchanged) != len(list) {
		t.Fatalf("no rules must keep every tool, got %d", len(unchanged))
	}
}

func TestToolsForDefinitionCoderKeepsFullSet(t *testing.T) {
	withTestRegistry(t, builtinDefs()...)
	def := &agentdef.Definition{
		Name:    string(config.AgentCoder),
		Mode:    agentdef.ModePrimary,
		Builtin: true,
	}
	names := toolNames(ToolsForDefinition(def, AgentToolDeps{
		Permissions: permission.NewPermissionService(),
	}))

	for _, want := range []string{
		tools.BashToolName, tools.EditToolName, tools.WriteToolName,
		tools.PatchToolName, tools.ViewToolName, tools.GlobToolName,
		tools.GrepToolName, tools.LSToolName, tools.FetchToolName,
		tools.SourcegraphToolName, AgentToolName,
	} {
		if !names[want] {
			t.Errorf("coder must keep %q", want)
		}
	}
}

func TestToolsForDefinitionTaskIsReadOnly(t *testing.T) {
	def := &agentdef.Definition{
		Name:    string(config.AgentTask),
		Mode:    agentdef.ModeSubagent,
		Builtin: true,
	}
	names := toolNames(ToolsForDefinition(def, AgentToolDeps{}))

	want := []string{
		tools.GlobToolName, tools.GrepToolName, tools.LSToolName,
		tools.SourcegraphToolName, tools.ViewToolName,
	}
	if len(names) != len(want) {
		t.Fatalf("task tools = %v, want %d tools", names, len(want))
	}
	for _, name := range want {
		if !names[name] {
			t.Errorf("task must keep %q", name)
		}
	}
	for _, banned := range []string{tools.BashToolName, tools.WriteToolName, AgentToolName} {
		if names[banned] {
			t.Errorf("task must not get %q", banned)
		}
	}
}

func TestToolsForDefinitionCustomAgentStartsFromFullSet(t *testing.T) {
	withTestRegistry(t, builtinDefs()...)
	full := toolNames(ToolsForDefinition(&agentdef.Definition{
		Name: "custom",
		Mode: agentdef.ModeAll,
	}, AgentToolDeps{Permissions: permission.NewPermissionService()}))
	coder := toolNames(ToolsForDefinition(&agentdef.Definition{
		Name:    string(config.AgentCoder),
		Mode:    agentdef.ModePrimary,
		Builtin: true,
	}, AgentToolDeps{Permissions: permission.NewPermissionService()}))

	if len(full) != len(coder) {
		t.Fatalf("a custom agent without rules must get the full set (%d tools), got %d", len(coder), len(full))
	}
	for name := range coder {
		if !full[name] {
			t.Errorf("custom agent is missing %q", name)
		}
	}
}

func TestToolsForDefinitionAppliesToolsRules(t *testing.T) {
	names := toolNames(ToolsForDefinition(&agentdef.Definition{
		Name: "restricted",
		Mode: agentdef.ModeAll,
		Tools: map[string]bool{
			tools.WriteToolName: false,
			tools.EditToolName:  false,
			tools.PatchToolName: false,
			tools.BashToolName:  false,
		},
	}, AgentToolDeps{Permissions: permission.NewPermissionService()}))

	for _, banned := range []string{tools.WriteToolName, tools.EditToolName, tools.PatchToolName, tools.BashToolName} {
		if names[banned] {
			t.Errorf("restricted agent must not get %q", banned)
		}
	}
	if !names[tools.ViewToolName] || !names[tools.GlobToolName] {
		t.Errorf("restricted agent must keep the read only tools, got %v", names)
	}
}

func TestToolsForDefinitionAllowList(t *testing.T) {
	names := toolNames(ToolsForDefinition(&agentdef.Definition{
		Name:  "reader",
		Mode:  agentdef.ModeAll,
		Tools: map[string]bool{tools.ViewToolName: true},
	}, AgentToolDeps{}))

	if len(names) != 1 || !names[tools.ViewToolName] {
		t.Fatalf("allow list must keep only the listed tools, got %v", names)
	}
}

func TestToolsForDefinitionWildcardBlocksMcpTools(t *testing.T) {
	names := toolNames(ToolsForDefinition(&agentdef.Definition{
		Name:  "nomcp",
		Mode:  agentdef.ModeAll,
		Tools: map[string]bool{"mymcp_*": false},
	}, AgentToolDeps{}))

	if names["mymcp_search"] {
		t.Fatal("wildcard must block the server tools")
	}
	if len(names) < 10 {
		t.Fatalf("a wildcard rule must not drop the local tools, got %d", len(names))
	}
}

func TestToolsForDefinitionPolicyDeniesBash(t *testing.T) {
	if _, err := config.Load(t.TempDir(), false); err != nil {
		t.Fatalf("failed to load config: %v", err)
	}

	list := ToolsForDefinition(&agentdef.Definition{
		Name:        "nobash",
		Mode:        agentdef.ModeAll,
		Permissions: []agentdef.PermissionRule{{Action: "shell", Effect: "deny"}},
	}, AgentToolDeps{Permissions: permission.NewPermissionService()})

	var bashTool tools.BaseTool
	for _, tool := range list {
		if tool.Info().Name == tools.BashToolName {
			bashTool = tool
		}
	}
	if bashTool == nil {
		t.Fatal("the bash tool must still be offered to the model")
	}

	ctx := context.WithValue(context.Background(), tools.SessionIDContextKey, "session")
	ctx = context.WithValue(ctx, tools.MessageIDContextKey, "message")

	// Denied by the policy, so the command is never executed.
	_, err := bashTool.Run(ctx, tools.ToolCall{
		Input: `{"command": "mkdir /tmp/agentdef-denied"}`,
	})
	if !errors.Is(err, permission.ErrorPermissionDenied) {
		t.Fatalf("bash must be denied by the agent policy, got %v", err)
	}
}

func TestToolsForDefinitionPolicyDeniesWrite(t *testing.T) {
	if _, err := config.Load(t.TempDir(), false); err != nil {
		t.Fatalf("failed to load config: %v", err)
	}

	list := ToolsForDefinition(&agentdef.Definition{
		Name:        "noedit",
		Mode:        agentdef.ModeAll,
		Permissions: []agentdef.PermissionRule{{Action: "edit", Effect: "deny"}},
	}, AgentToolDeps{Permissions: permission.NewPermissionService()})

	var writeTool tools.BaseTool
	for _, tool := range list {
		if tool.Info().Name == tools.WriteToolName {
			writeTool = tool
		}
	}
	if writeTool == nil {
		t.Fatal("the write tool must still be offered to the model")
	}

	ctx := context.WithValue(context.Background(), tools.SessionIDContextKey, "session")
	ctx = context.WithValue(ctx, tools.MessageIDContextKey, "message")
	filePath := filepath.Join(t.TempDir(), "denied.txt")

	_, err := writeTool.Run(ctx, tools.ToolCall{
		Input: fmt.Sprintf(`{"file_path": %q, "content": "nope"}`, filePath),
	})
	if !errors.Is(err, permission.ErrorPermissionDenied) {
		t.Fatalf("write must be denied by the agent policy, got %v", err)
	}
}
