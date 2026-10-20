package agent

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/opencode-ai/opencode/internal/agentdef"
	"github.com/opencode-ai/opencode/internal/config"
	"github.com/opencode-ai/opencode/internal/llm/tools"
	"github.com/opencode-ai/opencode/internal/permission"
	"github.com/opencode-ai/opencode/internal/plugin"
	"github.com/spf13/viper"
)

// setDeclarativePlugin installs a plugin manager with a manifest-only plugin
// (no process) that declares one tool. Declarative plugins keep this test
// hermetic: no subprocess and no network.
func setDeclarativePlugin(t *testing.T) {
	t.Helper()
	dir := t.TempDir()
	content := `{"name":"helper","tools":[{"name":"echo","description":"echoes","parameters":{"properties":{"x":{"type":"string"}},"required":["x"]}}]}`
	pdir := filepath.Join(dir, ".opencode", "plugins", "helper")
	if err := os.MkdirAll(pdir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(pdir, "plugin.json"), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}

	mgr := plugin.NewManager(dir)
	mgr.Load(nil)
	plugin.SetManager(mgr)
	t.Cleanup(func() { plugin.SetManager(nil) })
}

func loadConfigForToolTest(t *testing.T) {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, ".config"))
	t.Setenv("ANTHROPIC_API_KEY", "test-key")
	project := t.TempDir()
	viper.Reset()
	if _, err := config.Load(project, false); err != nil {
		t.Fatalf("config.Load: %v", err)
	}
}

func TestGetPluginToolsInjection(t *testing.T) {
	setDeclarativePlugin(t)

	perms := permission.NewPermissionService()
	list := GetPluginTools(perms)
	if len(list) != 1 {
		t.Fatalf("tools = %d, want 1", len(list))
	}
	info := list[0].Info()
	if info.Name != "plugin_helper_echo" {
		t.Fatalf("name = %q", info.Name)
	}
	if info.Description != "echoes" {
		t.Fatalf("description = %q", info.Description)
	}
	if len(info.Required) != 1 || info.Required[0] != "x" {
		t.Fatalf("required = %v", info.Required)
	}
	if _, ok := info.Parameters["x"]; !ok {
		t.Fatalf("parameters = %v", info.Parameters)
	}
}

func TestPluginToolInFullToolSet(t *testing.T) {
	setDeclarativePlugin(t)
	deps := AgentToolDeps{Permissions: permission.NewPermissionService()}
	set := FullToolSet(context.Background(), nil, deps)
	if !containsTool(set, "plugin_helper_echo") {
		t.Fatalf("plugin tool missing from full tool set")
	}
}

func TestPluginToolPermissionDenied(t *testing.T) {
	loadConfigForToolTest(t)
	setDeclarativePlugin(t)

	policy, err := permission.ParsePolicy([]agentdef.PermissionRule{
		{Action: "plugin_helper_echo", Resource: "", Effect: "deny"},
	})
	if err != nil {
		t.Fatalf("ParsePolicy: %v", err)
	}
	denied := permission.NewAgentScoped(permission.NewPermissionService(), policy)
	list := GetPluginTools(denied)
	if len(list) != 1 {
		t.Fatalf("tools = %d", len(list))
	}
	ctx := context.WithValue(context.Background(), tools.SessionIDContextKey, "s1")
	ctx = context.WithValue(ctx, tools.MessageIDContextKey, "m1")
	resp, err := list[0].Run(ctx, tools.ToolCall{Name: "plugin_helper_echo", Input: `{"x":"y"}`})
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	if !resp.IsError {
		t.Fatalf("expected permission denial, got %+v", resp)
	}
}

func containsTool(list []tools.BaseTool, name string) bool {
	for _, t := range list {
		if t.Info().Name == name {
			return true
		}
	}
	return false
}
