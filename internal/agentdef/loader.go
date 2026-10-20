package agentdef

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/bmatcuk/doublestar/v4"
	"github.com/opencode-ai/opencode/internal/logging"
	"gopkg.in/yaml.v3"
)

// sourceDirs are the directory names scanned for agent markdown files.
var sourceDirs = []string{"agent", "agents"}

// Options configures agent discovery.
type Options struct {
	// GlobalDirs are scanned first (lowest precedence).
	GlobalDirs []string
	// ProjectDir is scanned last (highest precedence, overrides global).
	ProjectDir string
	// Builtins are registered before any file and act as defaults.
	Builtins []*Definition
}

// frontmatter mirrors the YAML header accepted in agent markdown files.
type frontmatter struct {
	Name            string          `yaml:"name"`
	Description     string          `yaml:"description"`
	Mode            string          `yaml:"mode"`
	Hidden          bool            `yaml:"hidden"`
	Disabled        bool            `yaml:"disabled"`
	Team            string          `yaml:"team"`
	Model           string          `yaml:"model"`
	MaxTokens       int64           `yaml:"maxTokens"`
	ReasoningEffort string          `yaml:"reasoningEffort"`
	Temperature     float64         `yaml:"temperature"`
	Tools           map[string]bool `yaml:"tools"`
	Permissions     map[string]any  `yaml:"permissions"`
	Color           string          `yaml:"color"`
}

// Load discovers agent definitions from builtins, global dirs and the project
// dir, in increasing order of precedence. Only frontmatter is parsed here;
// prompt bodies are read lazily via Definition.Prompt. Files with invalid
// frontmatter are skipped with a warning rather than failing the whole load.
func Load(opts Options) (*Registry, error) {
	reg := NewRegistry()
	for _, d := range opts.Builtins {
		if err := reg.Register(d); err != nil {
			return nil, err
		}
	}

	dirs := make([]string, 0, len(opts.GlobalDirs)+1)
	for _, d := range opts.GlobalDirs {
		if strings.TrimSpace(d) != "" {
			dirs = append(dirs, d)
		}
	}
	if strings.TrimSpace(opts.ProjectDir) != "" {
		dirs = append(dirs, opts.ProjectDir)
	}

	for _, dir := range dirs {
		files, err := discover(dir)
		if err != nil {
			logging.Warn("failed to scan agent directory", "dir", dir, "error", err)
			continue
		}
		for _, file := range files {
			def, err := loadFile(file.path, teamOf(file.path, file.base))
			if err != nil {
				logging.Warn("skipping invalid agent file", "file", file.path, "error", err)
				continue
			}
			if err := reg.Register(def); err != nil {
				logging.Warn("skipping duplicate agent", "file", file.path, "error", err)
			}
		}
	}
	return reg, nil
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
