package agent

import (
	"context"
	"os"
	"path/filepath"
	"sync"
	"testing"

	"github.com/opencode-ai/opencode/internal/config"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
	"github.com/spf13/viper"
)

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}

// resetMCPManagerForTest rebuilds the process-wide manager from the current
// config so a test can point it at a fake server.
func resetMCPManagerForTest() {
	mcpManagerOnce = sync.Once{}
	mcpManagerInst = nil
}

func newFakeMCPServer(t *testing.T) *server.MCPServer {
	t.Helper()
	s := server.NewMCPServer("fake", "1.0.0")
	s.AddTool(
		mcp.NewTool("echo", mcp.WithDescription("echoes the message"), mcp.WithString("msg")),
		func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			return mcp.NewToolResultText("echo: " + req.GetString("msg", "")), nil
		},
	)
	s.AddResource(
		mcp.NewResource("fake://doc", "Doc", mcp.WithResourceDescription("a doc")),
		func(ctx context.Context, req mcp.ReadResourceRequest) ([]mcp.ResourceContents, error) {
			return []mcp.ResourceContents{mcp.TextResourceContents{URI: "fake://doc", Text: "resource body"}}, nil
		},
	)
	s.AddPrompt(
		mcp.NewPrompt("greet", mcp.WithPromptDescription("greets")),
		func(ctx context.Context, req mcp.GetPromptRequest) (*mcp.GetPromptResult, error) {
			return &mcp.GetPromptResult{
				Messages: []mcp.PromptMessage{
					mcp.NewPromptMessage(mcp.RoleAssistant, mcp.NewTextContent("hello")),
				},
			}, nil
		},
	)
	return s
}

// TestMCPHTTP exercises the whole MCP v2 surface in one test because config.Load
// is a process-wide singleton: it loads "fake" (enabled, pointing at the fake
// server) and "off" (disabled), then checks transport, tools, resources,
// prompts and the enabled/disabled behaviour together.
func TestMCPHTTP(t *testing.T) {
	s := newFakeMCPServer(t)
	ts := server.NewTestStreamableHTTPServer(s)
	defer ts.Close()

	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, ".config"))
	t.Setenv("ANTHROPIC_API_KEY", "test")
	project := t.TempDir()
	writeFile(t, filepath.Join(project, "opencode.json"), `{
		"mcp": {"timeout": 5000, "servers": {
			"fake": {"type": "remote", "url": "`+ts.URL+`", "enabled": true},
			"off":  {"type": "remote", "url": "`+ts.URL+`", "enabled": false}
		}}
	}`)
	viper.Reset()
	if _, err := config.Load(project, false); err != nil {
		t.Fatalf("load config: %v", err)
	}
	resetMCPManagerForTest()
	defer resetMCPManagerForTest()

	ctx := context.Background()
	mgr := GetMCPManager()

	if mgr.Enabled("off") {
		t.Fatal("disabled server must not be managed")
	}
	if !mgr.Enabled("fake") {
		t.Fatal("enabled server must be managed")
	}

	tools := discoverMcpTools(ctx)
	if len(tools) != 1 || tools[0].tool.Name != "echo" || tools[0].mcpName != "fake" {
		t.Fatalf("tools = %+v", tools)
	}

	c, _, err := mgr.Client(ctx, "fake")
	if err != nil {
		t.Fatalf("client: %v", err)
	}
	resp, err := callTool(ctx, c, "echo", `{"msg":"hi"}`)
	if err != nil {
		t.Fatalf("call: %v", err)
	}
	if resp.Content != "echo: hi" {
		t.Fatalf("response = %q", resp.Content)
	}

	res := ListMCPResources(ctx)
	if len(res) != 1 || res[0].URI != "fake://doc" {
		t.Fatalf("resources = %+v", res)
	}
	if body, err := ReadMCPResource(ctx, "fake", "fake://doc"); err != nil || body != "resource body" {
		t.Fatalf("read resource = %q, %v", body, err)
	}

	prompts := ListMCPPrompts(ctx)
	if len(prompts) != 1 || prompts[0].Name != "greet" {
		t.Fatalf("prompts = %+v", prompts)
	}
	if text, err := GetMCPPrompt(ctx, "fake", "greet", nil); err != nil || text != "hello" {
		t.Fatalf("get prompt = %q, %v", text, err)
	}
}
