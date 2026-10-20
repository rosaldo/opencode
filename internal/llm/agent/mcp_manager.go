package agent

import (
	"context"
	"fmt"
	"net"
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
	// oauthListener is the reserved loopback listener for the OAuth callback
	// flow, kept open between the client build and the authorization redirect
	// so the redirect URI matches (Spec 08b).
	oauthListener net.Listener
	oauthRedirect string
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

// oauthConfigFor builds the mcp-go OAuth configuration for a server, reserving a
// loopback callback listener so the redirect URI is known before the flow
// starts. The returned listener must be closed by the caller once done.
func oauthConfigFor(server string, o *config.MCPOAuth) (transport.OAuthConfig, net.Listener, string, error) {
	ln, err := net.Listen("tcp", fmt.Sprintf("127.0.0.1:%d", o.CallbackPort))
	if err != nil {
		return transport.OAuthConfig{}, nil, "", fmt.Errorf("oauth callback listener: %w", err)
	}
	port := ln.Addr().(*net.TCPAddr).Port
	redirect := fmt.Sprintf("http://localhost:%d/callback", port)
	return transport.OAuthConfig{
		ClientID:     o.ClientID,
		ClientSecret: o.ClientSecret,
		RedirectURI:  redirect,
		Scopes:       o.Scopes,
		TokenStore:   oauthTokenStore(server, o),
		PKCEEnabled:  pkceEnabled(o),
	}, ln, redirect, nil
}

// newClient builds a client for the server's transport and initializes it. The
// bool result is false when the transport is per-call (non-persistent).
func (m *MCPManager) newClient(ctx context.Context, name string, srv config.MCPServer) (MCPClient, bool, error) {
	initRequest := mcp.InitializeRequest{}
	initRequest.Params.ProtocolVersion = mcp.LATEST_PROTOCOL_VERSION
	initRequest.Params.ClientInfo = mcp.Implementation{
		Name:    "OpenCode",
		Version: version.Version,
	}

	var oauthCfg *transport.OAuthConfig
	if srv.OAuth != nil {
		cfg, ln, redirect, err := oauthConfigFor(name, srv.OAuth)
		if err != nil {
			return nil, true, err
		}
		oauthCfg = &cfg
		m.setOAuthListener(name, ln, redirect)
	}

	var (
		c   *client.Client
		err error
	)
	switch srv.Type {
	case config.MCPStdio:
		c, err = client.NewStdioMCPClient(srv.Command, srv.Env, srv.Args...)
		if err != nil {
			return nil, true, err
		}
		if _, err := c.Initialize(ctx, initRequest); err != nil {
			c.Close()
			return nil, true, err
		}
		return c, true, nil
	case config.MCPSse:
		opts := []transport.ClientOption{transport.WithHeaders(srv.Headers)}
		if oauthCfg != nil {
			opts = append(opts, transport.WithOAuth(*oauthCfg))
		}
		c, err = client.NewSSEMCPClient(srv.URL, opts...)
		if err != nil {
			return nil, true, err
		}
		if _, err := c.Initialize(ctx, initRequest); err != nil {
			c.Close()
			if client.IsOAuthAuthorizationRequiredError(err) && srv.OAuth != nil {
				if aerr := m.authorize(ctx, name, srv.OAuth, client.GetOAuthHandler(err)); aerr != nil {
					return nil, true, aerr
				}
				return m.newClient(ctx, name, srv)
			}
			return nil, true, err
		}
		return c, true, nil
	case config.MCPHTTP, config.MCPRemote:
		opts := []transport.StreamableHTTPCOption{
			transport.WithHTTPHeaders(srv.Headers),
			transport.WithHTTPTimeout(m.timeoutFor(srv)),
		}
		if oauthCfg != nil {
			opts = append(opts, transport.WithHTTPOAuth(*oauthCfg))
		}
		c, err = client.NewStreamableHttpClient(srv.URL, opts...)
		if err != nil {
			return nil, true, err
		}
		if err := c.Start(ctx); err != nil {
			c.Close()
			if client.IsOAuthAuthorizationRequiredError(err) && srv.OAuth != nil {
				if aerr := m.authorize(ctx, name, srv.OAuth, client.GetOAuthHandler(err)); aerr != nil {
					return nil, true, aerr
				}
				return m.newClient(ctx, name, srv)
			}
			return nil, true, err
		}
		if _, err := c.Initialize(ctx, initRequest); err != nil {
			c.Close()
			// An OAuth-required error is recoverable: authorize and retry once.
			if client.IsOAuthAuthorizationRequiredError(err) && srv.OAuth != nil {
				if aerr := m.authorize(ctx, name, srv.OAuth, client.GetOAuthHandler(err)); aerr != nil {
					return nil, true, aerr
				}
				return m.newClient(ctx, name, srv)
			}
			return nil, true, err
		}
		return c, true, nil
	default:
		return nil, false, fmt.Errorf("invalid mcp type %q", srv.Type)
	}
}

// setOAuthListener records the reserved callback listener for a server so the
// flow can serve the redirect on the same port (Spec 08b).
func (m *MCPManager) setOAuthListener(name string, ln net.Listener, redirect string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if st, ok := m.servers[name]; ok {
		if st.oauthListener != nil {
			st.oauthListener.Close()
		}
		st.oauthListener = ln
		st.oauthRedirect = redirect
	}
}

// authorize runs the interactive OAuth flow for a handler, using the reserved
// callback listener when present.
func (m *MCPManager) authorize(ctx context.Context, name string, o *config.MCPOAuth, handler *transport.OAuthHandler) error {
	if handler == nil {
		return fmt.Errorf("oauth handler unavailable")
	}
	m.mu.Lock()
	var ln net.Listener
	if st, ok := m.servers[name]; ok {
		ln = st.oauthListener
	}
	m.mu.Unlock()

	actx, cancel := context.WithTimeout(ctx, 5*time.Minute)
	defer cancel()
	if ln != nil {
		return runOAuthFlowWithListener(actx, name, o, handler, ln)
	}
	return runOAuthFlow(actx, name, o, handler)
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
	c, persistent, err := m.newClient(ctx, name, srv)
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
	if state.oauthListener != nil {
		state.oauthListener.Close()
		state.oauthListener = nil
		state.oauthRedirect = ""
	}
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
		if state.oauthListener != nil {
			state.oauthListener.Close()
			state.oauthListener = nil
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
