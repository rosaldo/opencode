// Package plugin loads OpenCode plugins (Spec 10). A plugin is a directory
// containing a plugin.json manifest and, for executable plugins, a command that
// speaks the stdio JSON protocol implemented in client.go.
package plugin

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/opencode-ai/opencode/internal/config"
)

// lookPath is exec.LookPath, indirected for tests.
var lookPath = exec.LookPath

// Hook names a lifecycle point a plugin can subscribe to (Spec 10b).
type Hook string

const (
	HookSessionStart      Hook = "session.start"
	HookMessageBeforeSend Hook = "message.before_send"
	HookToolBeforeExecute Hook = "tool.before_execute"
	HookToolAfterExecute  Hook = "tool.after_execute"
	HookChatParams        Hook = "chat.params"
)

// ToolSpec declares a custom tool a plugin provides (Spec 10c).
type ToolSpec struct {
	Name        string         `json:"name"`
	Description string         `json:"description,omitempty"`
	Parameters  map[string]any `json:"parameters,omitempty"`
}

// Manifest is the content of a plugin.json file.
type Manifest struct {
	Name        string `json:"name"`
	Version     string `json:"version,omitempty"`
	Description string `json:"description,omitempty"`
	// Command is the executable (plus args) that implements the stdio protocol.
	// When empty the plugin is declarative (hooks/tools only, no process).
	Command []string   `json:"command,omitempty"`
	Hooks   []Hook     `json:"hooks,omitempty"`
	Tools   []ToolSpec `json:"tools,omitempty"`
}

// Plugin is a resolved plugin: its manifest, where it lives and the options
// supplied in the config.
type Plugin struct {
	Manifest Manifest
	Dir      string
	Options  map[string]any
}

// Name returns the plugin name (manifest name, falling back to the dir base).
func (p Plugin) Name() string {
	if p.Manifest.Name != "" {
		return p.Manifest.Name
	}
	return filepath.Base(p.Dir)
}

// Loader discovers and resolves plugins.
type Loader struct {
	// ProjectDir is <project>/.opencode/plugins.
	ProjectDir string
	// GlobalDir is ~/.config/opencode/plugins.
	GlobalDir string
	// SystemDir is /etc/opencode/plugins (optional).
	SystemDir string
}

// NewLoader builds a Loader for the given working directory using the v2 plugin
// directories.
func NewLoader(workingDir string) *Loader {
	l := &Loader{
		ProjectDir: filepath.Join(workingDir, ".opencode", "plugins"),
		SystemDir:  "/etc/opencode/plugins",
	}
	if home, err := os.UserHomeDir(); err == nil {
		l.GlobalDir = filepath.Join(home, ".config", "opencode", "plugins")
	}
	return l
}

// searchDirs returns the directories scanned for plugin sub-directories, in
// precedence order (project > global > system).
func (l *Loader) searchDirs() []string {
	dirs := []string{}
	for _, d := range []string{l.ProjectDir, l.GlobalDir, l.SystemDir} {
		if d != "" {
			dirs = append(dirs, d)
		}
	}
	return dirs
}

// Resolve turns the configured plugin specs into resolved plugins. Specs are
// applied first (in order, giving them precedence); any plugin discovered by
// convention but not named is appended afterwards. A "-name" spec (Disabled)
// removes that plugin from the result. Warnings describe problems that did not
// stop loading.
func (l *Loader) Resolve(specs []config.PluginSpec) ([]Plugin, []string) {
	warnings := []string{}
	byName := map[string]Plugin{}
	order := []string{}
	disabled := map[string]bool{}

	// Discover by convention.
	for _, dir := range l.searchDirs() {
		plugins, err := l.readDir(dir)
		if err != nil {
			continue
		}
		for _, p := range plugins {
			name := p.Name()
			if _, seen := byName[name]; !seen {
				byName[name] = p
				order = append(order, name)
			}
		}
	}

	// Apply explicit specs (highest precedence).
	explicit := []string{}
	for _, spec := range specs {
		if spec.Package == "" {
			continue
		}
		if spec.Disabled {
			disabled[spec.Package] = true
			continue
		}
		p, err := l.resolveSpec(spec)
		if err != nil {
			warnings = append(warnings, fmt.Sprintf("plugin %q: %v", spec.Package, err))
			continue
		}
		name := p.Name()
		byName[name] = p
		if _, seen := indexOf(order, name); !seen {
			explicit = append(explicit, name)
		}
	}
	// Explicit plugins come first, in config order.
	order = append(explicit, order...)

	seen := map[string]bool{}
	result := []Plugin{}
	for _, name := range order {
		if seen[name] || disabled[name] {
			continue
		}
		seen[name] = true
		result = append(result, byName[name])
	}
	return result, warnings
}

func indexOf(s []string, v string) (int, bool) {
	for i, x := range s {
		if x == v {
			return i, true
		}
	}
	return -1, false
}

// resolveSpec resolves a single config spec to a plugin. A path-like package is
// read directly; a bare name is looked up among the conventional directories.
func (l *Loader) resolveSpec(spec config.PluginSpec) (Plugin, error) {
	if isPathLike(spec.Package) {
		p, err := l.readOne(spec.Package)
		if err != nil {
			return Plugin{}, err
		}
		p.Options = spec.Options
		return p, nil
	}
	for _, dir := range l.searchDirs() {
		cand := filepath.Join(dir, spec.Package)
		if fi, err := os.Stat(cand); err == nil && fi.IsDir() {
			p, err := l.readOne(cand)
			if err != nil {
				return Plugin{}, err
			}
			p.Options = spec.Options
			return p, nil
		}
	}
	// Treat as an executable package on PATH: a manifest-less plugin.
	if path, err := lookPath(spec.Package); err == nil {
		return Plugin{
			Dir:     filepath.Dir(path),
			Options: spec.Options,
			Manifest: Manifest{
				Name:    spec.Package,
				Command: []string{path},
			},
		}, nil
	}
	return Plugin{}, fmt.Errorf("not found in %v or on PATH", l.searchDirs())
}

func isPathLike(s string) bool {
	return strings.HasPrefix(s, ".") || strings.HasPrefix(s, "/") || strings.ContainsAny(s, "/\\")
}

// readDir reads every plugin directory under dir.
func (l *Loader) readDir(dir string) ([]Plugin, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}
	out := []Plugin{}
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		p, err := l.readOne(filepath.Join(dir, e.Name()))
		if err != nil {
			continue
		}
		out = append(out, p)
	}
	return out, nil
}

// readOne reads a plugin from a directory: either its plugin.json manifest or,
// failing that, a manifest-less declarative plugin named after the directory.
func (l *Loader) readOne(dir string) (Plugin, error) {
	fi, err := os.Stat(dir)
	if err != nil {
		return Plugin{}, err
	}
	if !fi.IsDir() {
		return Plugin{}, fmt.Errorf("%s is not a directory", dir)
	}
	manifestPath := filepath.Join(dir, "plugin.json")
	data, err := os.ReadFile(manifestPath)
	if err != nil {
		if os.IsNotExist(err) {
			return Plugin{Dir: dir, Manifest: Manifest{Name: filepath.Base(dir)}}, nil
		}
		return Plugin{}, err
	}
	var m Manifest
	if err := json.Unmarshal(data, &m); err != nil {
		return Plugin{}, fmt.Errorf("invalid plugin.json: %w", err)
	}
	if m.Name == "" {
		m.Name = filepath.Base(dir)
	}
	return Plugin{Dir: dir, Manifest: m}, nil
}
