package agent

import (
	"context"
	"encoding/json"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"strconv"
	"testing"
	"time"

	"github.com/opencode-ai/opencode/internal/config"

	"github.com/mark3labs/mcp-go/client/transport"
)

func TestFileTokenStoreRoundTrip(t *testing.T) {
	dir := t.TempDir()
	store := &fileTokenStore{path: filepath.Join(dir, "mcp-oauth", "srv.json")}
	if _, err := store.GetToken(); err == nil {
		t.Fatal("expected error for missing token")
	}
	tok := &transport.Token{AccessToken: "a", TokenType: "Bearer", ExpiresAt: time.Now().Add(time.Hour)}
	if err := store.SaveToken(tok); err != nil {
		t.Fatalf("save: %v", err)
	}
	got, err := store.GetToken()
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if got.AccessToken != "a" {
		t.Fatalf("token = %+v", got)
	}
}

func TestOAuthFlowCompletes(t *testing.T) {
	var authSrv *httptest.Server
	mux := http.NewServeMux()
	mux.HandleFunc("/.well-known/oauth-authorization-server", func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(transport.AuthServerMetadata{
			Issuer:                 authSrv.URL,
			AuthorizationEndpoint:  authSrv.URL + "/authorize",
			TokenEndpoint:          authSrv.URL + "/token",
			RegistrationEndpoint:   authSrv.URL + "/register",
			ResponseTypesSupported: []string{"code"},
		})
	})
	mux.HandleFunc("/register", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusCreated)
		json.NewEncoder(w).Encode(map[string]string{"client_id": "dyn-client"})
	})
	mux.HandleFunc("/authorize", func(w http.ResponseWriter, r *http.Request) {
		redirect := r.URL.Query().Get("redirect_uri")
		state := r.URL.Query().Get("state")
		http.Redirect(w, r, redirect+"?code=XYZ&state="+url.QueryEscape(state), http.StatusFound)
	})
	mux.HandleFunc("/token", func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(transport.Token{AccessToken: "access-1", TokenType: "Bearer", ExpiresIn: 3600})
	})
	authSrv = httptest.NewServer(mux)
	defer authSrv.Close()

	// Reserve the callback listener exactly as newClient would.
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	port := ln.Addr().(*net.TCPAddr).Port
	redirect := "http://localhost:" + strconv.Itoa(port) + "/callback"

	dir := t.TempDir()
	store := &fileTokenStore{path: filepath.Join(dir, "tok.json")}
	handler := transport.NewOAuthHandler(transport.OAuthConfig{
		RedirectURI:           redirect,
		TokenStore:            store,
		PKCEEnabled:           true,
		AuthServerMetadataURL: authSrv.URL + "/.well-known/oauth-authorization-server",
	})

	// Substitute the browser with an HTTP GET of the authorization URL; the fake
	// authorization endpoint redirects to our callback with the code.
	old := openBrowser
	openBrowser = func(u string) { http.Get(u) }
	defer func() { openBrowser = old }()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	o := &config.MCPOAuth{PKCE: boolPtr(true), Scopes: []string{"mcp.read"}}
	if err := runOAuthFlowWithListener(ctx, "srv", o, handler, ln); err != nil {
		t.Fatalf("flow: %v", err)
	}
	got, err := store.GetToken()
	if err != nil {
		t.Fatalf("token not saved: %v", err)
	}
	if got.AccessToken != "access-1" {
		t.Fatalf("token = %+v", got)
	}
}

func boolPtr(b bool) *bool { return &b }
