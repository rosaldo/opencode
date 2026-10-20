package agent

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/opencode-ai/opencode/internal/config"
	"github.com/opencode-ai/opencode/internal/logging"
	"github.com/opencode-ai/opencode/internal/version"

	"github.com/mark3labs/mcp-go/client"
	"github.com/mark3labs/mcp-go/client/transport"
	"github.com/mark3labs/mcp-go/mcp"
)

// defaultMCPTimeout is used when neither the global nor the per-server timeout
// is configured (Spec 08).
const defaultMCPTimeout = 30 * time.Second

// mcpServerState holds one persistent client per server so that the process is
// started once and reused across calls, and can be restarted on failure.
type mcpServerState struct {
	name   string
	config config.MCPServer
	client MCPClient
	// persistent is false when the transport cannot be reused and a fresh
	// client is created per call.
	persistent bool
}

// MCPManager owns the MCP clients: it creates them lazily, keeps them alive and
// can restart a failed server without bringing down the app (Spec 08).
type MCPManager struct {
	mu      sync.Mutex
	servers map[string]*mcpServerState
	timeout time.Duration
}

var (
	mcpManagerOnce sync.Once
	mcpManagerInst *MCPManager
)

// GetMCPManager returns the process-wide manager, building it from the current
// config on first use.
func GetMCPManager() *MCPManager {
	mcpManagerOnce.Do(func() {
		mcpManagerInst = newMCPManager()
	})
	return mcpManagerInst
}

func newMCPManager() *MCPManager {
	m := &MCPManager{servers: map[string]*mcpServerState{}}
	cfg := config.Get()
	if cfg == nil {
		m.timeout = defaultMCPTimeout
		return m
	}
	m.timeout = timeoutFor(cfg, config.MCPServer{})
	for name, srv := range cfg.MCPServers {
		if !srv.IsEnabled() {
			continue
		}
		m.servers[name] = &mcpServerState{name: name, config: srv}
	}
	return m
}

// timeoutFor resolves a server's timeout, falling back to the global one and
// then the default. Values are in milliseconds.
func timeoutFor(cfg *config.Config, srv config.MCPServer) time.Duration {
	if srv.Timeout > 0 {
		return time.Duration(srv.Timeout) * time.Millisecond
	}
	if cfg != nil && cfg.MCPTimeout > 0 {
		return time.Duration(cfg.MCPTimeout) * time.Millisecond
	}
	return defaultMCPTimeout
}

// newClient builds a client for the server's transport and initializes it. The
// bool result is false when the transport is per-call (non-persistent).
func (m *MCPManager) newClient(ctx context.Context, srv config.MCPServer) (MCPClient, bool, error) {
	initRequest := mcp.InitializeRequest{}
	initRequest.Params.ProtocolVersion = mcp.LATEST_PROTOCOL_VERSION
	initRequest.Params.ClientInfo = mcp.Implementation{
		Name:    "OpenCode",
		Version: version.Version,
	}

	switch srv.Type {
	case config.MCPStdio:
		c, err := client.NewStdioMCPClient(srv.Command, srv.Env, srv.Args...)
		if err != nil {
			return nil, true, err
		}
		if _, err := c.Initialize(ctx, initRequest); err != nil {
			c.Close()
			return nil, true, err
		}
		return c, true, nil
	case config.MCPSse:
		c, err := client.NewSSEMCPClient(srv.URL, client.WithHeaders(srv.Headers))
		if err != nil {
			return nil, true, err
		}
		if _, err := c.Initialize(ctx, initRequest); err != nil {
			c.Close()
			return nil, true, err
		}
		return c, true, nil
	case config.MCPHTTP, config.MCPRemote:
		c, err := client.NewStreamableHttpClient(srv.URL,
			transport.WithHTTPHeaders(srv.Headers),
			transport.WithHTTPTimeout(m.timeoutFor(srv)),
		)
		if err != nil {
			return nil, true, err
		}
		if err := c.Start(ctx); err != nil {
			c.Close()
			return nil, true, err
		}
		if _, err := c.Initialize(ctx, initRequest); err != nil {
			c.Close()
			return nil, true, err
		}
		return c, true, nil
	default:
		return nil, false, fmt.Errorf("invalid mcp type %q", srv.Type)
	}
}

func (m *MCPManager) timeoutFor(srv config.MCPServer) time.Duration {
	if srv.Timeout > 0 {
		return time.Duration(srv.Timeout) * time.Millisecond
	}
	if m.timeout > 0 {
		return m.timeout
	}
	return defaultMCPTimeout
}

// Client returns a live client for the named server, creating it on demand and
// reusing it afterwards. Callers must NOT close it; use Restart or CloseAll.
func (m *MCPManager) Client(ctx context.Context, name string) (MCPClient, config.MCPServer, error) {
	m.mu.Lock()
	state, ok := m.servers[name]
	if !ok {
		m.mu.Unlock()
		return nil, config.MCPServer{}, fmt.Errorf("unknown mcp server %q", name)
	}
	if state.client != nil {
		c := state.client
		srv := state.config
		m.mu.Unlock()
		return c, srv, nil
	}
	srv := state.config
	m.mu.Unlock()

	ctx, cancel := context.WithTimeout(ctx, m.timeoutFor(srv))
	defer cancel()
	c, persistent, err := m.newClient(ctx, srv)
	if err != nil {
		return nil, srv, err
	}
	if persistent {
		m.mu.Lock()
		state.client, state.persistent = c, true
		m.mu.Unlock()
	}
	return c, srv, nil
}

// Restart drops the cached client for a server so the next Client call rebuilds
// it. It is used when a call fails, to recover without a full process restart.
func (m *MCPManager) Restart(name string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	state, ok := m.servers[name]
	if !ok || state.client == nil {
		return
	}
	if err := state.client.Close(); err != nil {
		logging.Warn("error closing mcp client on restart", "server", name, "error", err)
	}
	state.client = nil
	state.persistent = false
}

// CloseAll closes every persistent client. It is safe to call multiple times.
func (m *MCPManager) CloseAll() {
	m.mu.Lock()
	defer m.mu.Unlock()
	for name, state := range m.servers {
		if state.client != nil {
			if err := state.client.Close(); err != nil {
				logging.Warn("error closing mcp client", "server", name, "error", err)
			}
			state.client = nil
		}
	}
}

// Servers lists the enabled server names.
func (m *MCPManager) Servers() []string {
	m.mu.Lock()
	defer m.mu.Unlock()
	names := make([]string, 0, len(m.servers))
	for name := range m.servers {
		names = append(names, name)
	}
	return names
}

// Enabled reports whether the named server is managed (present and enabled).
func (m *MCPManager) Enabled(name string) bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	_, ok := m.servers[name]
	return ok
}
