package agent

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/opencode-ai/opencode/internal/config"
	"github.com/opencode-ai/opencode/internal/llm/tools"
	"github.com/opencode-ai/opencode/internal/permission"
	"github.com/opencode-ai/opencode/internal/plugin"
)

// pluginTool adapts a plugin-declared tool to the agent tool interface (Spec
// 10c). Its name is qualified ("plugin_<plugin>_<tool>") and, like MCP tools, it
// goes through the permission service before being run.
type pluginTool struct {
	meta        plugin.ToolMeta
	permissions permission.Service
}

func (b *pluginTool) Info() tools.ToolInfo {
	required, _ := b.meta.Parameters["required"].([]any)
	req := make([]string, 0, len(required))
	for _, r := range required {
		if s, ok := r.(string); ok {
			req = append(req, s)
		}
	}
	params := b.meta.Parameters
	if props, ok := params["properties"].(map[string]any); ok {
		params = props
	}
	return tools.ToolInfo{
		Name:        b.meta.FullName(),
		Description: b.meta.Description,
		Parameters:  params,
		Required:    req,
	}
}

func (b *pluginTool) Run(ctx context.Context, params tools.ToolCall) (tools.ToolResponse, error) {
	sessionID, messageID := tools.GetContextValues(ctx)
	if sessionID == "" || messageID == "" {
		return tools.ToolResponse{}, fmt.Errorf("session ID and message ID are required")
	}
	p := b.permissions.Request(permission.CreatePermissionRequest{
		SessionID:   sessionID,
		Path:        config.WorkingDirectory(),
		ToolName:    b.Info().Name,
		Action:      "execute",
		Description: fmt.Sprintf("execute plugin tool %s with parameters: %s", b.Info().Name, params.Input),
		Params:      params.Input,
	})
	if !p {
		return tools.NewTextErrorResponse("permission denied"), nil
	}

	mgr := plugin.GetManager()
	if mgr == nil {
		return tools.NewTextErrorResponse("plugins are not loaded"), nil
	}
	var args map[string]any
	if params.Input != "" {
		if err := json.Unmarshal([]byte(params.Input), &args); err != nil {
			return tools.NewTextErrorResponse(fmt.Sprintf("error parsing parameters: %s", err)), nil
		}
	}
	res, err := mgr.CallTool(ctx, b.meta.FullName(), args)
	if err != nil {
		return tools.NewTextErrorResponse(err.Error()), nil
	}
	if res.IsError {
		return tools.NewTextErrorResponse(res.Content), nil
	}
	return tools.NewTextResponse(res.Content), nil
}

// GetPluginTools returns the tools declared by the loaded plugins, bound to the
// given permission service so every agent applies its own policy.
func GetPluginTools(permissions permission.Service) []tools.BaseTool {
	mgr := plugin.GetManager()
	if mgr == nil {
		return nil
	}
	metas := mgr.Tools()
	out := make([]tools.BaseTool, 0, len(metas))
	for _, meta := range metas {
		out = append(out, &pluginTool{meta: meta, permissions: permissions})
	}
	return out
}
