package agentdef

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/bmatcuk/doublestar/v4"
	"github.com/opencode-ai/opencode/internal/logging"
	"gopkg.in/yaml.v3"
)

// Source produces agent definitions. Every way of declaring an agent — markdown
// files or the "agents" map of the JSON configuration — is one implementation,
// so the registry has a single kind of input (Spec 01, Spec 13).
type Source interface {
	Definitions() ([]*Definition, error)
}

// sourceDirs are the directory names scanned for agent markdown files.
var sourceDirs = []string{"agent", "agents"}

// FileSource discovers agent markdown files under the agent(s) directories of
// one root (a global config dir or the project dir). Only frontmatter is parsed;
// prompt bodies are read lazily via Definition.Prompt.
type FileSource struct {
	// Dir is the root that holds the agent(s) subdirectories.
	Dir string
}

// Definitions discovers and parses the markdown agent files under s.Dir. Files
// with invalid frontmatter are skipped with a warning rather than failing the
// whole load.
func (s FileSource) Definitions() ([]*Definition, error) {
	if strings.TrimSpace(s.Dir) == "" {
		return nil, nil
	}
	files, err := discover(s.Dir)
	if err != nil {
		return nil, err
	}
	out := make([]*Definition, 0, len(files))
	for _, file := range files {
		def, err := loadFile(file.path, teamOf(file.path, file.base))
		if err != nil {
			logging.Warn("skipping invalid agent file", "file", file.path, "error", err)
			continue
		}
		out = append(out, def)
	}
	return out, nil
}

// AgentConfig is the inline declaration of an agent, used both by the "agents"
// map of the JSON configuration and, conceptually, by the YAML frontmatter of
// markdown files (the two share field names). Prompt is the v2 "system" text.
type AgentConfig struct {
	Description     string           `json:"description,omitempty" yaml:"description,omitempty"`
	Mode            string           `json:"mode,omitempty" yaml:"mode,omitempty"`
	Hidden          bool             `json:"hidden,omitempty" yaml:"hidden,omitempty"`
	Disabled        bool             `json:"disabled,omitempty" yaml:"disabled,omitempty"`
	Team            string           `json:"team,omitempty" yaml:"team,omitempty"`
	Model           string           `json:"model,omitempty" yaml:"model,omitempty"`
	MaxTokens       int64            `json:"maxTokens,omitempty" yaml:"maxTokens,omitempty"`
	ReasoningEffort string           `json:"reasoningEffort,omitempty" yaml:"reasoningEffort,omitempty"`
	Temperature     float64          `json:"temperature,omitempty" yaml:"temperature,omitempty"`
	Tools           map[string]bool  `json:"tools,omitempty" yaml:"tools,omitempty"`
	Permissions     []PermissionRule `json:"permissions,omitempty" yaml:"permissions,omitempty"`
	Color           string           `json:"color,omitempty" yaml:"color,omitempty"`
	// System is the inline prompt (v2). It may be a "{file:./path}" reference,
	// resolved by the caller relative to the config file that declares it.
	System string `json:"system,omitempty" yaml:"-"`
}

// JSONSource turns a set of inline agent declarations into definitions. BaseDir
// is used to resolve "{file:...}" prompts relative to the config file.
type JSONSource struct {
	Agents map[string]AgentConfig
	// BaseDir resolves relative "{file:...}" prompt references.
	BaseDir string
	// ResolvePrompt optionally expands the prompt text (e.g. "{file:...}") using
	// BaseDir; when nil the prompt is used as-is.
	ResolvePrompt func(prompt, baseDir string) string
}

// Definitions converts the inline agent declarations into definitions, in a
// stable (sorted) order.
func (s JSONSource) Definitions() ([]*Definition, error) {
	if len(s.Agents) == 0 {
		return nil, nil
	}
	names := make([]string, 0, len(s.Agents))
	for name := range s.Agents {
		names = append(names, name)
	}
	sort.Strings(names)

	out := make([]*Definition, 0, len(names))
	for _, name := range names {
		out = append(out, DefFromConfig(name, s.Agents[name], s.BaseDir, s.ResolvePrompt))
	}
	return out, nil
}

// DefFromConfig converts a config-facing AgentConfig into a Definition. An
// invalid mode falls back to ModeAll (with a warning); an empty name is not
// possible since the map key is authoritative.
func DefFromConfig(name string, def AgentConfig, baseDir string, resolve func(string, string) string) *Definition {
	mode := Mode(strings.TrimSpace(def.Mode))
	switch mode {
	case "", ModePrimary, ModeSubagent, ModeAll:
		// ok
	default:
		logging.Warn("invalid mode in agent definition, defaulting to all",
			"agent", name, "mode", def.Mode)
		mode = ModeAll
	}

	d := &Definition{
		Name:            name,
		Description:     strings.TrimSpace(def.Description),
		Mode:            mode,
		Hidden:          def.Hidden,
		Disabled:        def.Disabled,
		Team:            strings.TrimSpace(def.Team),
		Model:           strings.TrimSpace(def.Model),
		MaxTokens:       def.MaxTokens,
		ReasoningEffort: strings.ToLower(strings.TrimSpace(def.ReasoningEffort)),
		Temperature:     def.Temperature,
		Tools:           def.Tools,
		Permissions:     def.Permissions,
		Color:           strings.TrimSpace(def.Color),
	}
	if prompt := strings.TrimSpace(def.System); prompt != "" {
		if resolve != nil {
			prompt = resolve(prompt, baseDir)
		}
		d.SetPrompt(prompt)
	}
	return d
}

// discoveredFile is one markdown file together with the agent(s) root it was
// found under, which is what defines its department.
type discoveredFile struct {
	path string
	base string
}

// teamOf returns the department of an agent file: the directory path between
// the agent(s) root and the file ("" when the file sits at the root).
func teamOf(path, base string) string {
	rel, err := filepath.Rel(base, filepath.Dir(path))
	if err != nil {
		return ""
	}
	if rel == "." || strings.HasPrefix(rel, "..") {
		return ""
	}
	return filepath.ToSlash(rel)
}

// discover returns the markdown files under dir/{agent,agents}.
func discover(dir string) ([]discoveredFile, error) {
	seen := make(map[string]bool)
	var files []discoveredFile
	for _, sub := range sourceDirs {
		root := filepath.Join(dir, sub)
		pattern := filepath.Join(root, "**", "*.md")
		matches, err := doublestar.FilepathGlob(pattern)
		if err != nil {
			return nil, err
		}
		for _, m := range matches {
			if seen[m] {
				continue
			}
			if info, err := os.Stat(m); err == nil && !info.IsDir() {
				seen[m] = true
				files = append(files, discoveredFile{path: m, base: root})
			}
		}
	}
	return files, nil
}

func loadFile(path, team string) (*Definition, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	header, _ := SplitFrontmatter(string(data))
	fm := frontmatter{}
	if strings.TrimSpace(header) != "" {
		if err := yaml.Unmarshal([]byte(header), &fm); err != nil {
			return nil, fmt.Errorf("invalid frontmatter: %w", err)
		}
	}

	name := strings.TrimSpace(fm.Name)
	if name == "" {
		base := filepath.Base(path)
		name = strings.TrimSuffix(base, filepath.Ext(base))
	}

	mode := Mode(strings.TrimSpace(fm.Mode))
	switch mode {
	case "", ModePrimary, ModeSubagent, ModeAll:
		// ok
	default:
		return nil, fmt.Errorf("invalid mode %q", fm.Mode)
	}

	effort := strings.ToLower(strings.TrimSpace(fm.ReasoningEffort))

	// Frontmatter wins; otherwise the directory below agent(s)/ is the team.
	finalTeam := strings.TrimSpace(fm.Team)
	if finalTeam == "" {
		finalTeam = team
	}

	return &Definition{
		Name:            name,
		Description:     strings.TrimSpace(fm.Description),
		Mode:            mode,
		Hidden:          fm.Hidden,
		Disabled:        fm.Disabled,
		Team:            finalTeam,
		Model:           strings.TrimSpace(fm.Model),
		MaxTokens:       fm.MaxTokens,
		ReasoningEffort: effort,
		Temperature:     fm.Temperature,
		Tools:           fm.Tools,
		Permissions:     fm.Permissions,
		Color:           strings.TrimSpace(fm.Color),
		promptPath:      path,
	}, nil
}

// SplitFrontmatter splits a markdown document into its YAML frontmatter (without
// the --- delimiters) and its body. When there is no frontmatter the header is
// empty and the body is the full document.
func SplitFrontmatter(content string) (header, body string) {
	content = strings.ReplaceAll(content, "\r\n", "\n")
	if !strings.HasPrefix(content, "---\n") {
		return "", content
	}
	rest := content[len("---\n"):]
	idx := strings.Index(rest, "\n---")
	if idx < 0 {
		return "", content
	}
	header = rest[:idx]
	body = strings.TrimPrefix(rest[idx+len("\n---"):], "\n")
	return header, body
}
