package agent

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/opencode-ai/opencode/internal/config"
	"github.com/opencode-ai/opencode/internal/llm/tools"
	"github.com/opencode-ai/opencode/internal/logging"
	"github.com/opencode-ai/opencode/internal/permission"

	"github.com/mark3labs/mcp-go/mcp"
)

type mcpTool struct {
	mcpName     string
	tool        mcp.Tool
	mcpConfig   config.MCPServer
	permissions permission.Service
}

type MCPClient interface {
	Initialize(
		ctx context.Context,
		request mcp.InitializeRequest,
	) (*mcp.InitializeResult, error)
	ListTools(ctx context.Context, request mcp.ListToolsRequest) (*mcp.ListToolsResult, error)
	CallTool(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error)
	ListResources(ctx context.Context, request mcp.ListResourcesRequest) (*mcp.ListResourcesResult, error)
	ReadResource(ctx context.Context, request mcp.ReadResourceRequest) (*mcp.ReadResourceResult, error)
	ListPrompts(ctx context.Context, request mcp.ListPromptsRequest) (*mcp.ListPromptsResult, error)
	GetPrompt(ctx context.Context, request mcp.GetPromptRequest) (*mcp.GetPromptResult, error)
	Close() error
}

func (b *mcpTool) Info() tools.ToolInfo {
	required := b.tool.InputSchema.Required
	if required == nil {
		required = make([]string, 0)
	}
	return tools.ToolInfo{
		Name:        fmt.Sprintf("%s_%s", b.mcpName, b.tool.Name),
		Description: b.tool.Description,
		Parameters:  b.tool.InputSchema.Properties,
		Required:    required,
	}
}

// callTool invokes a tool on an already-initialized client.
func callTool(ctx context.Context, c MCPClient, toolName string, input string) (tools.ToolResponse, error) {
	toolRequest := mcp.CallToolRequest{}
	toolRequest.Params.Name = toolName
	var args map[string]any
	if err := json.Unmarshal([]byte(input), &args); err != nil {
		return tools.NewTextErrorResponse(fmt.Sprintf("error parsing parameters: %s", err)), nil
	}
	toolRequest.Params.Arguments = args
	result, err := c.CallTool(ctx, toolRequest)
	if err != nil {
		return tools.NewTextErrorResponse(err.Error()), nil
	}

	output := ""
	for _, v := range result.Content {
		if v, ok := v.(mcp.TextContent); ok {
			output = v.Text
		} else {
			output = fmt.Sprintf("%v", v)
		}
	}

	return tools.NewTextResponse(output), nil
}

func (b *mcpTool) Run(ctx context.Context, params tools.ToolCall) (tools.ToolResponse, error) {
	sessionID, messageID := tools.GetContextValues(ctx)
	if sessionID == "" || messageID == "" {
		return tools.ToolResponse{}, fmt.Errorf("session ID and message ID are required for creating a new file")
	}
	permissionDescription := fmt.Sprintf("execute %s with the following parameters: %s", b.Info().Name, params.Input)
	p := b.permissions.Request(
		permission.CreatePermissionRequest{
			SessionID:   sessionID,
			Path:        config.WorkingDirectory(),
			ToolName:    b.Info().Name,
			Action:      "execute",
			Description: permissionDescription,
			Params:      params.Input,
		},
	)
	if !p {
		return tools.NewTextErrorResponse("permission denied"), nil
	}

	mgr := GetMCPManager()
	c, _, err := mgr.Client(ctx, b.mcpName)
	if err != nil {
		return tools.NewTextErrorResponse(err.Error()), nil
	}
	resp, err := callTool(ctx, c, b.tool.Name, params.Input)
	if err != nil {
		// A dead client is restarted once so a crashed server recovers without
		// taking the app down (Spec 08).
		mgr.Restart(b.mcpName)
		if c, _, err2 := mgr.Client(ctx, b.mcpName); err2 == nil {
			if r2, e2 := callTool(ctx, c, b.tool.Name, params.Input); e2 == nil {
				return r2, nil
			}
		}
		return tools.NewTextErrorResponse(err.Error()), nil
	}
	return resp, nil
}

func NewMcpTool(name string, tool mcp.Tool, permissions permission.Service, mcpConfig config.MCPServer) tools.BaseTool {
	return &mcpTool{
		mcpName:     name,
		tool:        tool,
		mcpConfig:   mcpConfig,
		permissions: permissions,
	}
}

// mcpToolDef is the permission-free description of one MCP tool. Only this is
// cached: the permission service is bound when the tools are built, so that
// every agent can have its own policy.
type mcpToolDef struct {
	mcpName   string
	tool      mcp.Tool
	mcpConfig config.MCPServer
}

var mcpToolDefs []mcpToolDef

// discoverTools lists the tools of an already-initialized client.
func discoverTools(ctx context.Context, name string, m config.MCPServer, c MCPClient) []mcpToolDef {
	var defs []mcpToolDef
	result, err := c.ListTools(ctx, mcp.ListToolsRequest{})
	if err != nil {
		logging.Error("error listing tools", "server", name, "error", err)
		return defs
	}
	for _, t := range result.Tools {
		defs = append(defs, mcpToolDef{mcpName: name, tool: t, mcpConfig: m})
	}
	return defs
}

func discoverMcpTools(ctx context.Context) []mcpToolDef {
	var defs []mcpToolDef
	mgr := GetMCPManager()
	for _, name := range mgr.Servers() {
		c, srv, err := mgr.Client(ctx, name)
		if err != nil {
			logging.Error("error creating mcp client", "server", name, "error", err)
			continue
		}
		defs = append(defs, discoverTools(ctx, name, srv, c)...)
	}
	return defs
}

func GetMcpTools(ctx context.Context, permissions permission.Service) []tools.BaseTool {
	if len(mcpToolDefs) == 0 {
		mcpToolDefs = discoverMcpTools(ctx)
	}
	mcpTools := make([]tools.BaseTool, 0, len(mcpToolDefs))
	for _, def := range mcpToolDefs {
		mcpTools = append(mcpTools, NewMcpTool(def.mcpName, def.tool, permissions, def.mcpConfig))
	}
	return mcpTools
}

// MCPResourceInfo is a resource exposed by an MCP server (Spec 08).
type MCPResourceInfo struct {
	Server      string
	URI         string
	Name        string
	Description string
	MimeType    string
}

// MCPPromptInfo is a prompt exposed by an MCP server (Spec 08).
type MCPPromptInfo struct {
	Server      string
	Name        string
	Description string
	Arguments   []mcp.PromptArgument
}

// ListMCPResources lists the resources of every enabled server. Failures are
// logged and skipped so one bad server cannot break the listing. This is the
// minimal surface for this phase; wiring into the agent loop is Spec 09.
func ListMCPResources(ctx context.Context) []MCPResourceInfo {
	var out []MCPResourceInfo
	mgr := GetMCPManager()
	for _, name := range mgr.Servers() {
		c, _, err := mgr.Client(ctx, name)
		if err != nil {
			logging.Error("error creating mcp client", "server", name, "error", err)
			continue
		}
		res, err := c.ListResources(ctx, mcp.ListResourcesRequest{})
		if err != nil {
			logging.Warn("error listing mcp resources", "server", name, "error", err)
			continue
		}
		for _, r := range res.Resources {
			out = append(out, MCPResourceInfo{
				Server:      name,
				URI:         r.URI,
				Name:        r.Name,
				Description: r.Description,
				MimeType:    r.MIMEType,
			})
		}
	}
	return out
}

// ListMCPPrompts lists the prompts of every enabled server.
func ListMCPPrompts(ctx context.Context) []MCPPromptInfo {
	var out []MCPPromptInfo
	mgr := GetMCPManager()
	for _, name := range mgr.Servers() {
		c, _, err := mgr.Client(ctx, name)
		if err != nil {
			logging.Error("error creating mcp client", "server", name, "error", err)
			continue
		}
		res, err := c.ListPrompts(ctx, mcp.ListPromptsRequest{})
		if err != nil {
			logging.Warn("error listing mcp prompts", "server", name, "error", err)
			continue
		}
		for _, p := range res.Prompts {
			out = append(out, MCPPromptInfo{
				Server:      name,
				Name:        p.Name,
				Description: p.Description,
				Arguments:   p.Arguments,
			})
		}
	}
	return out
}

// ReadMCPResource reads one resource and returns its concatenated text content.
func ReadMCPResource(ctx context.Context, server, uri string) (string, error) {
	c, _, err := GetMCPManager().Client(ctx, server)
	if err != nil {
		return "", err
	}
	res, err := c.ReadResource(ctx, mcp.ReadResourceRequest{Params: mcp.ReadResourceParams{URI: uri}})
	if err != nil {
		return "", err
	}
	out := ""
	for _, rc := range res.Contents {
		if tc, ok := rc.(mcp.TextResourceContents); ok {
			out += tc.Text
		}
	}
	return out, nil
}

// GetMCPPrompt renders a prompt (with arguments) and returns its text.
func GetMCPPrompt(ctx context.Context, server, name string, args map[string]string) (string, error) {
	c, _, err := GetMCPManager().Client(ctx, server)
	if err != nil {
		return "", err
	}
	res, err := c.GetPrompt(ctx, mcp.GetPromptRequest{Params: mcp.GetPromptParams{Name: name, Arguments: args}})
	if err != nil {
		return "", err
	}
	out := ""
	for _, m := range res.Messages {
		if tc, ok := m.Content.(mcp.TextContent); ok {
			out += tc.Text
		}
	}
	return out, nil
}
