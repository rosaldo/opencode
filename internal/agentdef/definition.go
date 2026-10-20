// Package agentdef holds the dynamic registry of agent definitions. It is
// intentionally free of dependencies on config, prompt or the llm runtimes so
// that those packages can depend on it without creating import cycles.
package agentdef

import (
	"os"
	"sync"
)

// Mode controls how an agent can be used.
type Mode string

const (
	// ModePrimary agents are directly selectable by the user.
	ModePrimary Mode = "primary"
	// ModeSubagent agents are invoked by other agents via the task tool.
	ModeSubagent Mode = "subagent"
	// ModeAll agents can be used both as primary and subagent (default).
	ModeAll Mode = "all"
)

// PermissionRule is one entry of an agent's permissions list (opencode v2).
// The list is ordered: the first rule whose Action and Resource match a request
// wins, so more specific rules must come first.
type PermissionRule struct {
	// Action is the permission being gated: "shell", "edit", "read", "list",
	// "webfetch", "task" or a tool/server name.
	Action string `json:"action" yaml:"action"`
	// Resource is an optional glob the request value is matched against (for
	// example a shell command or a subagent name). Empty matches everything.
	Resource string `json:"resource,omitempty" yaml:"resource,omitempty"`
	// Effect is "allow", "ask" or "deny".
	Effect string `json:"effect" yaml:"effect"`
}

// Definition describes an agent discovered from a markdown file or provided by
// configuration. Prompt bodies are loaded lazily via Prompt so that booting with
// hundreds of agents stays cheap.
type Definition struct {
	Name        string
	Description string
	Mode        Mode
	Hidden      bool
	Disabled    bool
	Builtin     bool
	// Team groups agents into departments for delegation announcements. Empty
	// means the agent belongs to no department. Files under agent(s)/<dir>/...
	// inherit the directory name when frontmatter does not set it.
	Team string

	Model           string // "provider/model" or a bare model ID; empty inherits
	MaxTokens       int64
	ReasoningEffort string
	Temperature     float64

	Tools       map[string]bool  // allow/deny by tool name; "*" wildcard
	Permissions []PermissionRule // ordered per-agent permission policy (v2)
	Color       string

	promptPath string

	promptOnce sync.Once
	promptText string
	promptErr  error

	body    string
	hasBody bool
}

// SourcePath returns the markdown file the definition was loaded from, if any.
func (d *Definition) SourcePath() string { return d.promptPath }

// SetPrompt sets the prompt body directly, bypassing file loading. Used for
// agents defined inline in configuration.
func (d *Definition) SetPrompt(text string) {
	d.body = text
	d.hasBody = true
}

// Prompt returns the system prompt body for the agent, reading the source file
// on first use and caching the result. Builtin agents return an empty string;
// their prompts are resolved by the prompt package.
func (d *Definition) Prompt() (string, error) {
	if d.Builtin {
		return "", nil
	}
	d.promptOnce.Do(func() {
		if d.hasBody {
			d.promptText = d.body
			return
		}
		if d.promptPath == "" {
			return
		}
		data, err := os.ReadFile(d.promptPath)
		if err != nil {
			d.promptErr = err
			return
		}
		_, body := SplitFrontmatter(string(data))
		if body == "" {
			body = string(data)
		}
		d.promptText = body
	})
	return d.promptText, d.promptErr
}
