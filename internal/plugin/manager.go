package plugin

import (
	"context"
	"fmt"
	"time"

	"github.com/opencode-ai/opencode/internal/config"
	"github.com/opencode-ai/opencode/internal/logging"
)

// hookTimeout bounds each hook invocation so a hung plugin cannot stall the app.
const hookTimeout = 5 * time.Second

// Manager owns the running plugin clients and dispatches hooks.
type Manager struct {
	loader  *Loader
	clients []*Client
}

// NewManager builds a manager that discovers plugins for workingDir.
func NewManager(workingDir string) *Manager {
	return &Manager{loader: NewLoader(workingDir)}
}

// Load resolves the configured plugins and starts each one. A plugin that fails
// to start is skipped with a warning so one bad plugin cannot break the app
// (acceptance criterion 6).
func (m *Manager) Load(specs []config.PluginSpec) []string {
	plugins, warnings := m.loader.Resolve(specs)
	for _, p := range plugins {
		c, err := Start(p)
		if err != nil {
			warnings = append(warnings, "plugin "+p.Name()+": failed to start: "+err.Error())
			continue
		}
		m.clients = append(m.clients, c)
	}
	return warnings
}

// Plugins returns the loaded plugin names in order.
func (m *Manager) Plugins() []string {
	out := make([]string, 0, len(m.clients))
	for _, c := range m.clients {
		out = append(out, c.Name())
	}
	return out
}

// Close stops every plugin.
func (m *Manager) Close() {
	for _, c := range m.clients {
		if err := c.Close(); err != nil {
			logging.Warn("plugin close failed", "plugin", c.Name(), "error", err)
		}
	}
	m.clients = nil
}

// ToolMeta describes a tool a plugin provides.
type ToolMeta struct {
	Plugin      string
	Name        string
	Description string
	Parameters  map[string]any
}

// FullName is the qualified tool name exposed to the model.
func (t ToolMeta) FullName() string { return "plugin_" + t.Plugin + "_" + t.Name }

// PluginSummary describes a loaded plugin for UI display.
type PluginSummary struct {
	Name  string
	Hooks []Hook
	Tools []string
}

// Summaries lists the loaded plugins with their hooks and tools (Spec 10c).
func (m *Manager) Summaries() []PluginSummary {
	out := make([]PluginSummary, 0, len(m.clients))
	for _, c := range m.clients {
		s := PluginSummary{Name: c.Name()}
		s.Hooks = append(s.Hooks, c.plugin.Manifest.Hooks...)
		for _, t := range c.plugin.Manifest.Tools {
			s.Tools = append(s.Tools, t.Name)
		}
		out = append(out, s)
	}
	return out
}

// Tools lists the tools declared by every loaded plugin.
func (m *Manager) Tools() []ToolMeta {
	var out []ToolMeta
	for _, c := range m.clients {
		for _, t := range c.plugin.Manifest.Tools {
			out = append(out, ToolMeta{
				Plugin:      c.Name(),
				Name:        t.Name,
				Description: t.Description,
				Parameters:  t.Parameters,
			})
		}
	}
	return out
}

// CallTool invokes a plugin tool by its qualified name (Spec 10c).
func (m *Manager) CallTool(ctx context.Context, fullName string, args map[string]any) (ToolResult, error) {
	for _, c := range m.clients {
		for _, t := range c.plugin.Manifest.Tools {
			if (ToolMeta{Plugin: c.Name(), Name: t.Name}).FullName() == fullName {
				cctx, cancel := context.WithTimeout(ctx, hookTimeout)
				defer cancel()
				return c.CallTool(cctx, t.Name, args)
			}
		}
	}
	return ToolResult{}, fmt.Errorf("plugin tool %q not found", fullName)
}

// hasHook reports whether the client subscribed to hook.
func (c *Client) hasHook(h Hook) bool {
	for _, x := range c.plugin.Manifest.Hooks {
		if x == h {
			return true
		}
	}
	return false
}

// HookDecision is the aggregated verdict of a tool.before_execute hook chain.
type HookDecision struct {
	Allow   bool
	Message string
	Mutate  map[string]any
}

// RunToolBeforeExecute runs tool.before_execute across plugins. The first plugin
// that denies wins (allow=false). Mutations from granted plugins are merged.
func (m *Manager) RunToolBeforeExecute(ctx context.Context, toolName string, args map[string]any) HookDecision {
	decision := HookDecision{Allow: true, Mutate: map[string]any{}}
	for _, c := range m.clients {
		if !c.hasHook(HookToolBeforeExecute) {
			continue
		}
		hctx, cancel := context.WithTimeout(ctx, hookTimeout)
		res, err := c.RunHook(hctx, HookToolBeforeExecute, map[string]any{
			"tool": toolName,
			"args": args,
		})
		cancel()
		if err != nil {
			logging.Warn("plugin hook failed", "plugin", c.Name(), "hook", HookToolBeforeExecute, "error", err)
			continue
		}
		if res.Allow != nil && !*res.Allow {
			decision.Allow = false
			decision.Message = res.Message
			return decision
		}
		for k, v := range res.Mutate {
			decision.Mutate[k] = v
		}
	}
	return decision
}

// RunToolAfterExecute notifies plugins after a tool ran. Failures are isolated.
func (m *Manager) RunToolAfterExecute(ctx context.Context, toolName string, args map[string]any, result string) {
	m.notify(ctx, HookToolAfterExecute, map[string]any{
		"tool":   toolName,
		"args":   args,
		"result": result,
	})
}

// RunSessionStart notifies plugins that a session started.
func (m *Manager) RunSessionStart(ctx context.Context, sessionID string) {
	m.notify(ctx, HookSessionStart, map[string]any{"session": sessionID})
}

// ChatParams is the result of running chat.params hooks.
type ChatParams struct {
	Model  string
	Params map[string]any
}

// RunChatParams runs chat.params hooks; the last non-empty model/params win.
func (m *Manager) RunChatParams(ctx context.Context, model string, params map[string]any) ChatParams {
	out := ChatParams{Model: model, Params: map[string]any{}}
	for k, v := range params {
		out.Params[k] = v
	}
	for _, c := range m.clients {
		if !c.hasHook(HookChatParams) {
			continue
		}
		hctx, cancel := context.WithTimeout(ctx, hookTimeout)
		res, err := c.RunHook(hctx, HookChatParams, map[string]any{"model": out.Model})
		cancel()
		if err != nil {
			logging.Warn("plugin hook failed", "plugin", c.Name(), "hook", HookChatParams, "error", err)
			continue
		}
		if res.Model != "" {
			out.Model = res.Model
		}
		for k, v := range res.Params {
			out.Params[k] = v
		}
	}
	return out
}

func (m *Manager) notify(ctx context.Context, hook Hook, payload map[string]any) {
	for _, c := range m.clients {
		if !c.hasHook(hook) {
			continue
		}
		hctx, cancel := context.WithTimeout(ctx, hookTimeout)
		if _, err := c.RunHook(hctx, hook, payload); err != nil {
			logging.Warn("plugin hook failed", "plugin", c.Name(), "hook", hook, "error", err)
		}
		cancel()
	}
}
