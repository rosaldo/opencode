package plugin

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/opencode-ai/opencode/internal/config"
)

func writeManifest(t *testing.T, dir, name, body string) {
	t.Helper()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if body == "" {
		body = `{"name":"` + name + `"}`
	}
	if err := os.WriteFile(filepath.Join(dir, "plugin.json"), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestLoaderDiscoversConventionDirs(t *testing.T) {
	project := t.TempDir()
	global := t.TempDir()
	writeManifest(t, filepath.Join(project, "plugins", "alpha"), "alpha", "")
	writeManifest(t, filepath.Join(global, "beta"), "beta", "")

	l := &Loader{
		ProjectDir: filepath.Join(project, "plugins"),
		GlobalDir:  global,
	}
	got, warns := l.Resolve(nil)
	if len(warns) != 0 {
		t.Fatalf("warnings: %v", warns)
	}
	if names(got) == nil || len(got) != 2 {
		t.Fatalf("plugins = %v", names(got))
	}
	if got[0].Name() != "alpha" || got[1].Name() != "beta" {
		t.Fatalf("order = %v", names(got))
	}
}

func TestLoaderExplicitPathAndOptions(t *testing.T) {
	project := t.TempDir()
	writeManifest(t, filepath.Join(project, "local"), "local", `{"name":"local","hooks":["session.start"]}`)

	l := &Loader{ProjectDir: filepath.Join(project, "plugins")}
	got, warns := l.Resolve([]config.PluginSpec{{
		Package: filepath.Join(project, "local"),
		Options: map[string]any{"enabled": true},
	}})
	if len(warns) != 0 {
		t.Fatalf("warnings: %v", warns)
	}
	if len(got) != 1 || got[0].Name() != "local" {
		t.Fatalf("plugins = %v", names(got))
	}
	if got[0].Options["enabled"] != true {
		t.Fatalf("options = %v", got[0].Options)
	}
	if len(got[0].Manifest.Hooks) != 1 || got[0].Manifest.Hooks[0] != HookSessionStart {
		t.Fatalf("hooks = %v", got[0].Manifest.Hooks)
	}
}

func TestLoaderDisable(t *testing.T) {
	project := t.TempDir()
	writeManifest(t, filepath.Join(project, "plugins", "alpha"), "alpha", "")
	writeManifest(t, filepath.Join(project, "plugins", "beta"), "beta", "")

	l := &Loader{ProjectDir: filepath.Join(project, "plugins")}
	got, _ := l.Resolve([]config.PluginSpec{{Package: "alpha", Disabled: true}})
	if len(got) != 1 || got[0].Name() != "beta" {
		t.Fatalf("plugins = %v, want only beta", names(got))
	}
}

func TestLoaderMissingExplicitWarns(t *testing.T) {
	project := t.TempDir()
	l := &Loader{ProjectDir: filepath.Join(project, "plugins")}
	got, warns := l.Resolve([]config.PluginSpec{{Package: "./does-not-exist"}})
	if len(got) != 0 {
		t.Fatalf("plugins = %v", names(got))
	}
	if len(warns) != 1 {
		t.Fatalf("warnings = %v", warns)
	}
}

func TestLoaderManifestless(t *testing.T) {
	project := t.TempDir()
	dir := filepath.Join(project, "plugins", "plain")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	l := &Loader{ProjectDir: filepath.Join(project, "plugins")}
	got, _ := l.Resolve(nil)
	if len(got) != 1 || got[0].Name() != "plain" {
		t.Fatalf("plugins = %v", names(got))
	}
}

func names(ps []Plugin) []string {
	out := make([]string, 0, len(ps))
	for _, p := range ps {
		out = append(out, p.Name())
	}
	return out
}
