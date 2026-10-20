package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestInterpolateEnv(t *testing.T) {
	t.Setenv("OPENCODE_TEST_VAR", "hello")
	if got := interpolate("{env:OPENCODE_TEST_VAR}", "/x"); got != "hello" {
		t.Fatalf("env = %q, want hello", got)
	}
	if got := interpolate("a={env:OPENCODE_TEST_VAR} b", "/x"); got != "a=hello b" {
		t.Fatalf("embedded env = %q", got)
	}
	if got := interpolate("{env:OPENCODE_MISSING_VAR}", "/x"); got != "" {
		t.Fatalf("missing env = %q, want empty", got)
	}
}

func TestInterpolateFile(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "p.txt"), []byte("from file"), 0o644); err != nil {
		t.Fatal(err)
	}
	if got := interpolate("{file:./p.txt}", dir); got != "from file" {
		t.Fatalf("relative = %q", got)
	}
	if got := interpolate("{file:p.txt}", dir); got != "from file" {
		t.Fatalf("bare relative = %q", got)
	}
	abs := filepath.Join(dir, "p.txt")
	if got := interpolate("{file:"+abs+"}", "/elsewhere"); got != "from file" {
		t.Fatalf("absolute = %q", got)
	}
	if got := interpolate("{file:./missing.txt}", dir); got != "" {
		t.Fatalf("missing = %q, want empty", got)
	}
}

func TestInterpolatePassthrough(t *testing.T) {
	cases := []string{
		"plain text",
		"{unknown:thing}",
		"a { b",
		"{}",
	}
	for _, in := range cases {
		if got := interpolate(in, "/x"); got != in {
			t.Errorf("interpolate(%q) = %q, want unchanged", in, got)
		}
	}
}

func TestInterpolateMixed(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "suffix.txt"), []byte("S"), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("OPENCODE_TEST_PREFIX", "P")
	got := interpolate("{env:OPENCODE_TEST_PREFIX}-{file:./suffix.txt}", dir)
	if got != "P-S" {
		t.Fatalf("mixed = %q, want P-S", got)
	}
}
