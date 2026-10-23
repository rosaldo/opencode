package cmd

import (
	"errors"
	"path/filepath"
	"testing"

	"github.com/opencode-ai/opencode/internal/session"
)

func TestLastSessionID(t *testing.T) {
	dir := t.TempDir()
	abs, err := filepath.Abs(dir)
	if err != nil {
		t.Fatal(err)
	}

	sessions := []session.Session{
		{ID: "other-project", WorkingDirectory: "/somewhere/else"},
		{ID: "current", WorkingDirectory: abs},
		{ID: "legacy", WorkingDirectory: ""},
	}
	if got := lastSessionID(sessions, nil, dir); got != "current" {
		t.Errorf("lastSessionID = %q, want current", got)
	}
	if got := lastSessionID(sessions, nil, "/unknown"); got != "legacy" {
		t.Errorf("lastSessionID fallback = %q, want legacy", got)
	}
	if got := lastSessionID(nil, nil, dir); got != "" {
		t.Errorf("empty list = %q, want empty", got)
	}
	if got := lastSessionID(nil, errors.New("boom"), dir); got != "" {
		t.Errorf("error = %q, want empty", got)
	}
}
