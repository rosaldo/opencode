package agent

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/opencode-ai/opencode/internal/config"
	"github.com/opencode-ai/opencode/internal/logging"

	"github.com/mark3labs/mcp-go/client/transport"
)

// fileTokenStore persists OAuth tokens to disk under the data directory so a
// server stays authorized across restarts (Spec 08b).
type fileTokenStore struct {
	path string
}

func newFileTokenStore(server string) *fileTokenStore {
	dir := ""
	if cfg := config.Get(); cfg != nil {
		if cfg.Data.Directory != "" {
			dir = cfg.Data.Directory
		} else {
			dir = cfg.WorkingDir
		}
	}
	if dir == "" {
		dir = os.TempDir()
	}
	return &fileTokenStore{path: filepath.Join(dir, "mcp-oauth", sanitizeName(server)+".json")}
}

func (s *fileTokenStore) GetToken() (*transport.Token, error) {
	b, err := os.ReadFile(s.path)
	if err != nil {
		return nil, err
	}
	var tok transport.Token
	if err := json.Unmarshal(b, &tok); err != nil {
		return nil, err
	}
	return &tok, nil
}

func (s *fileTokenStore) SaveToken(token *transport.Token) error {
	if err := os.MkdirAll(filepath.Dir(s.path), 0o700); err != nil {
		return err
	}
	b, err := json.MarshalIndent(token, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(s.path, b, 0o600)
}

// sanitizeName makes a server name safe for use as a file name.
func sanitizeName(name string) string {
	return strings.Map(func(r rune) rune {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9', r == '-', r == '_':
			return r
		default:
			return '_'
		}
	}, name)
}

func pkceEnabled(o *config.MCPOAuth) bool {
	return o.PKCE == nil || *o.PKCE
}

// oauthTokenStore builds the token store for a server's OAuth settings.
func oauthTokenStore(server string, _ *config.MCPOAuth) transport.TokenStore {
	return newFileTokenStore(server)
}

// runOAuthFlow completes an interactive authorization-code flow for a handler
// that reported an authorization-required error. It registers a client when
// needed, builds the authorization URL, opens the browser and waits for the
// local redirect to deliver the code (Spec 08b).
func runOAuthFlow(ctx context.Context, server string, o *config.MCPOAuth, handler *transport.OAuthHandler) error {
	return runOAuthFlowWithListener(ctx, server, o, handler, nil)
}

// runOAuthFlowWithListener performs the authorization flow, reusing a reserved
// callback listener when one is provided so the redirect URI is exact.
func runOAuthFlowWithListener(ctx context.Context, server string, o *config.MCPOAuth, handler *transport.OAuthHandler, ln net.Listener) error {
	if o.ClientID == "" {
		rctx, cancel := context.WithTimeout(ctx, 30*time.Second)
		defer cancel()
		if err := handler.RegisterClient(rctx, "OpenCode"); err != nil {
			return fmt.Errorf("oauth client registration: %w", err)
		}
	}

	state, err := randomURLSafe(32)
	if err != nil {
		return err
	}
	var verifier string
	if pkceEnabled(o) {
		verifier, err = randomURLSafe(32)
		if err != nil {
			return err
		}
	}

	codeChallenge := ""
	if verifier != "" {
		sum := sha256.Sum256([]byte(verifier))
		codeChallenge = base64.RawURLEncoding.EncodeToString(sum[:])
	}

	authURL, err := handler.GetAuthorizationURL(ctx, state, codeChallenge)
	if err != nil {
		return fmt.Errorf("oauth authorization url: %w", err)
	}

	code, err := awaitAuthorizationCode(ctx, ln, o.CallbackPort, state, authURL)
	if err != nil {
		return err
	}
	if err := handler.ProcessAuthorizationResponse(ctx, code, state, verifier); err != nil {
		return fmt.Errorf("oauth token exchange: %w", err)
	}
	logging.Info("mcp oauth authorized", "server", server)
	return nil
}

// awaitAuthorizationCode starts (or reuses) a loopback callback server, opens
// the browser and blocks until the redirect carries the authorization code.
func awaitAuthorizationCode(ctx context.Context, reserved net.Listener, port int, state, authURL string) (string, error) {
	ln := reserved
	if ln == nil {
		var err error
		ln, err = net.Listen("tcp", fmt.Sprintf("127.0.0.1:%d", port))
		if err != nil {
			return "", fmt.Errorf("oauth callback listener: %w", err)
		}
	}
	defer ln.Close()

	codeCh := make(chan string, 1)
	errCh := make(chan error, 1)
	mux := http.NewServeMux()
	mux.HandleFunc("/callback", func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()
		if e := q.Get("error"); e != "" {
			http.Error(w, "authorization failed: "+e, http.StatusBadRequest)
			errCh <- fmt.Errorf("oauth error: %s", e)
			return
		}
		if q.Get("state") != state {
			http.Error(w, "invalid state", http.StatusBadRequest)
			errCh <- transport.ErrInvalidState
			return
		}
		fmt.Fprintln(w, "Authorization complete. You can close this window.")
		codeCh <- q.Get("code")
	})
	srv := &http.Server{Handler: mux}
	go func() {
		if err := srv.Serve(ln); err != nil && err != http.ErrServerClosed {
			errCh <- err
		}
	}()
	defer func() {
		sctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		srv.Shutdown(sctx)
	}()

	logging.Info("mcp oauth authorization required", "url", authURL)
	fmt.Fprintf(os.Stderr, "\nOpen this URL to authorize the MCP server:\n%s\n\n", authURL)
	openBrowser(authURL)

	select {
	case code := <-codeCh:
		return code, nil
	case err := <-errCh:
		return "", err
	case <-ctx.Done():
		return "", ctx.Err()
	}
}

// openBrowser best-effort opens the URL with the platform's opener. It is a
// variable so tests can substitute a fake browser.
var openBrowser = func(url string) {
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "darwin":
		cmd = exec.Command("open", url)
	case "windows":
		cmd = exec.Command("rundll32", "url.dll,FileProtocolHandler", url)
	default:
		cmd = exec.Command("xdg-open", url)
	}
	if err := cmd.Start(); err != nil {
		logging.Debug("could not open browser", "error", err)
	}
}

// randomURLSafe returns n bytes of randomness as a URL-safe string.
func randomURLSafe(n int) (string, error) {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}
