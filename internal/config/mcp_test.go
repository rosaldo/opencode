package config

import (
	"path/filepath"
	"testing"
)

func TestMCPBlockV2Parse(t *testing.T) {
	home := t.TempDir()
	project := t.TempDir()
	writeConfigFile(t, filepath.Join(project, "opencode.json"), `{
		"mcp": {
			"timeout": 45000,
			"servers": {
				"playwright": {"type": "local", "command": ["bunx", "@playwright/mcp"], "enabled": true},
				"jira": {"type": "remote", "url": "https://jira.example.com/mcp", "enabled": false},
				"docs": {"type": "remote", "url": "https://docs.example.com/mcp", "timeout": 5000, "headers": {"X-Tenant": "acme"}}
			}
		}
	}`)

	loaded := loadForTest(t, home, project)
	defer resetLoad(t)

	if loaded.MCPTimeout != 45000 {
		t.Fatalf("mcp timeout = %d, want 45000", loaded.MCPTimeout)
	}
	pw := loaded.MCPServers["playwright"]
	if pw.Type != MCPStdio {
		t.Fatalf("playwright type = %q, want stdio", pw.Type)
	}
	if pw.Command != "bunx" {
		t.Fatalf("playwright command = %q", pw.Command)
	}
	if len(pw.Args) != 1 || pw.Args[0] != "@playwright/mcp" {
		t.Fatalf("playwright args = %v", pw.Args)
	}
	if !pw.IsEnabled() {
		t.Fatalf("playwright should be enabled")
	}

	jira := loaded.MCPServers["jira"]
	if jira.Type != MCPHTTP {
		t.Fatalf("jira type = %q, want http", jira.Type)
	}
	if jira.IsEnabled() {
		t.Fatalf("jira should be disabled")
	}

	docs := loaded.MCPServers["docs"]
	if docs.Timeout != 5000 {
		t.Fatalf("docs timeout = %d, want 5000", docs.Timeout)
	}
	if docs.Headers["X-Tenant"] != "acme" {
		t.Fatalf("docs headers = %v", docs.Headers)
	}
}

func TestMCPServersLegacyStillWorks(t *testing.T) {
	home := t.TempDir()
	project := t.TempDir()
	writeConfigFile(t, filepath.Join(project, "opencode.json"), `{
		"mcpServers": {
			"local-tools": {"type": "stdio", "command": "node", "args": ["server.js"]}
		}
	}`)

	loaded := loadForTest(t, home, project)
	defer resetLoad(t)

	s := loaded.MCPServers["local-tools"]
	if s.Type != MCPStdio || s.Command != "node" {
		t.Fatalf("legacy server = %+v", s)
	}
	if !s.IsEnabled() {
		t.Fatalf("legacy server should be enabled by default")
	}
}

func TestMCPLegacyWinsOnClash(t *testing.T) {
	home := t.TempDir()
	project := t.TempDir()
	writeConfigFile(t, filepath.Join(project, "opencode.json"), `{
		"mcpServers": {"x": {"type": "stdio", "command": "legacy"}},
		"mcp": {"servers": {"x": {"type": "local", "command": ["v2"]}}}
	}`)

	loaded := loadForTest(t, home, project)
	defer resetLoad(t)

	if got := loaded.MCPServers["x"].Command; got != "legacy" {
		t.Fatalf("command = %q, want legacy", got)
	}
}
