package agentdef

import (
	"github.com/opencode-ai/opencode/internal/logging"
)

// frontmatter mirrors the YAML header accepted in agent markdown files. It
// embeds AgentConfig so the JSON and YAML forms share one field set, and only
// adds the markdown-specific "name". There is no prompt field: the document
// body is the prompt (matching opencode v2 and Claude Code).
type frontmatter struct {
	Name        string `yaml:"name"`
	AgentConfig `yaml:",inline"`
}

// Load builds a registry from the given sources, registering them in order.
// Later sources override earlier ones with the same agent name, so the caller
// expresses precedence by ordering the sources (builtins < global < project).
// Invalid definitions are skipped with a warning rather than failing the load.
func Load(sources ...Source) (*Registry, error) {
	reg := NewRegistry()
	for _, src := range sources {
		if src == nil {
			continue
		}
		defs, err := src.Definitions()
		if err != nil {
			logging.Warn("failed to load agent source", "error", err)
			continue
		}
		for _, d := range defs {
			if err := reg.Register(d); err != nil {
				logging.Warn("skipping invalid agent definition", "agent", d.Name, "error", err)
			}
		}
	}
	return reg, nil
}
