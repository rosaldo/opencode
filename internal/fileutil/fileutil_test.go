package fileutil

import (
	"os"
	"path/filepath"
	"sync"
	"testing"
)

func TestSkipHidden(t *testing.T) {
	skip := []string{
		".env",
		"src/node_modules/pkg/index.js",
		"a/.git/HEAD",
		"proj/vendor/lib.go",
		"build/out.bin",
		".vscode/settings.json",
		"__pycache__/x.pyc",
		"logs/app.log",
	}
	for _, p := range skip {
		if !SkipHidden(p) {
			t.Errorf("SkipHidden(%q) = false, want true", p)
		}
	}

	keep := []string{
		"main.go",
		"src/app/index.js",
		"a/b/c.txt",
		".",
	}
	for _, p := range keep {
		if SkipHidden(p) {
			t.Errorf("SkipHidden(%q) = true, want false", p)
		}
	}
}

func TestGlobWithDoublestar(t *testing.T) {
	dir := t.TempDir()
	files := map[string]string{
		"a.go":          "package a",
		"sub/b.go":      "package b",
		"sub/deep/c.go": "package c",
		"notes.txt":     "hi",
	}
	for rel, content := range files {
		full := filepath.Join(dir, rel)
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(full, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	got, truncated, err := GlobWithDoublestar("**/*.go", dir, 0)
	if err != nil {
		t.Fatalf("GlobWithDoublestar: %v", err)
	}
	if truncated {
		t.Error("did not expect truncation with limit 0")
	}
	if len(got) != 3 {
		t.Fatalf("matched %d files, want 3: %v", len(got), got)
	}
}

func TestGlobWithDoublestarLimit(t *testing.T) {
	dir := t.TempDir()
	for _, name := range []string{"a.go", "b.go", "c.go", "d.go"} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte("x"), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	got, truncated, err := GlobWithDoublestar("*.go", dir, 2)
	if err != nil {
		t.Fatalf("GlobWithDoublestar: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("matched %d files, want 2", len(got))
	}
	if !truncated {
		t.Error("expected truncation flag when limit is exceeded")
	}
}

func TestGlobWithDoublestarSkipsHidden(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, ".hidden.go"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "visible.go"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}

	got, _, err := GlobWithDoublestar("*.go", dir, 0)
	if err != nil {
		t.Fatalf("GlobWithDoublestar: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("matched %v, want only visible.go", got)
	}
}

func TestGetRgCmdNilWhenMissing(t *testing.T) {
	// A bogus PATH guarantees exec.LookPath fails, regardless of the host.
	t.Setenv("PATH", "/nonexistent-dir-for-test")
	rgOnce = sync.Once{}
	rgPath = ""
	t.Cleanup(func() { rgOnce = sync.Once{}; rgPath = "" })

	if cmd := GetRgCmd("*.go"); cmd != nil {
		t.Fatalf("expected nil cmd when rg is absent, got %v", cmd)
	}
}

func TestGetFzfCmdNilWhenMissing(t *testing.T) {
	t.Setenv("PATH", "/nonexistent-dir-for-test")
	fzfOnce = sync.Once{}
	fzfPath = ""
	t.Cleanup(func() { fzfOnce = sync.Once{}; fzfPath = "" })

	if cmd := GetFzfCmd("query"); cmd != nil {
		t.Fatalf("expected nil cmd when fzf is absent, got %v", cmd)
	}
}
