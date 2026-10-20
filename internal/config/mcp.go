package config

import (
	"encoding/json"

	"github.com/opencode-ai/opencode/internal/logging"
)

// mcpConfig is the v2 "mcp" block: a global timeout and a servers map using the
// local/remote vocabulary (Spec 08). It is decoded from the raw merged map so
// that "command" may be a plain string or an array and keys keep their case.
type mcpConfig struct {
	Timeout int                     `json:"timeout,omitempty"`
	Servers map[string]mcpServerRaw `json:"servers,omitempty"`
}

// mcpServerRaw is one entry of mcp.servers. Its Command is kept raw because the
// v2 format allows both "bunx" and ["bunx", "@playwright/mcp"].
type mcpServerRaw struct {
	Type    MCPType           `json:"type,omitempty"`
	Command json.RawMessage   `json:"command,omitempty"`
	Args    []string          `json:"args,omitempty"`
	Env     []string          `json:"env,omitempty"`
	URL     string            `json:"url,omitempty"`
	Headers map[string]string `json:"headers,omitempty"`
	Enabled *bool             `json:"enabled,omitempty"`
	Timeout int               `json:"timeout,omitempty"`
	OAuth   *MCPOAuth         `json:"oauth,omitempty"`
}

// applyMCPConfig parses the v2 "mcp" block from the raw merged map and merges it
// into cfg.MCPServers (legacy "mcpServers" entries win on a name clash, since
// they are the older, more explicit form). Global and per-server timeouts are
// recorded, and local/remote types are normalised to the internal stdio/http.
func applyMCPConfig(merged map[string]any) {
	if cfg == nil {
		return
	}
	raw, ok := merged["mcp"]
	if !ok {
		return
	}
	b, err := json.Marshal(raw)
	if err != nil {
		return
	}
	var mc mcpConfig
	if err := json.Unmarshal(b, &mc); err != nil {
		logging.Warn("failed to parse mcp config", "error", err)
		return
	}
	cfg.MCPTimeout = mc.Timeout
	if cfg.MCPServers == nil {
		cfg.MCPServers = make(map[string]MCPServer)
	}
	for name, s := range mc.Servers {
		if _, exists := cfg.MCPServers[name]; exists {
			continue
		}
		cfg.MCPServers[name] = s.toMCPServer(cfg.MCPTimeout)
	}
}

// toMCPServer converts the v2 raw form into the internal MCPServer, resolving
// the local/remote vocabulary and the command form.
func (s mcpServerRaw) toMCPServer(globalTimeout int) MCPServer {
	srv := MCPServer{
		Type:    s.Type,
		Env:     s.Env,
		Args:    s.Args,
		URL:     s.URL,
		Headers: s.Headers,
		Enabled: s.Enabled,
		Timeout: s.Timeout,
		OAuth:   s.OAuth,
	}

	if len(s.Command) > 0 {
		var single string
		if err := json.Unmarshal(s.Command, &single); err == nil {
			srv.Command = single
		} else {
			var many []string
			if err := json.Unmarshal(s.Command, &many); err == nil && len(many) > 0 {
				srv.Command = many[0]
				srv.Args = append(append([]string{}, many[1:]...), srv.Args...)
			}
		}
	}

	switch srv.Type {
	case MCPLocal:
		srv.Type = MCPStdio
	case MCPRemote:
		srv.Type = MCPHTTP
	}
	if srv.Timeout == 0 {
		srv.Timeout = globalTimeout
	}
	return srv
}
