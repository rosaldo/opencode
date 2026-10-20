package config

import (
	"path/filepath"
	"testing"
)

func TestShareParse(t *testing.T) {
	home := t.TempDir()
	project := t.TempDir()
	writeConfigFile(t, filepath.Join(project, "opencode.json"), `{"share": "auto"}`)

	loaded := loadForTest(t, home, project)
	defer resetLoad(t)
	if loaded.Share != "auto" {
		t.Fatalf("share = %q", loaded.Share)
	}
}

func TestShareDefault(t *testing.T) {
	home := t.TempDir()
	project := t.TempDir()
	writeConfigFile(t, filepath.Join(project, "opencode.json"), `{}`)

	loaded := loadForTest(t, home, project)
	defer resetLoad(t)
	if loaded.Share != "disabled" {
		t.Fatalf("share = %q, want disabled", loaded.Share)
	}
}
