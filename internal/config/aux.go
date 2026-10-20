package config

import (
	"encoding/json"
	"strings"

	"github.com/opencode-ai/opencode/internal/agentdef"
	"github.com/opencode-ai/opencode/internal/logging"
)

// This file holds the auxiliary v2 config keys (Spec 11c). They are parsed and
// validated here but their behaviour belongs to the spec that owns each feature
// (commands/instructions → prompt & slash commands, formatter/watcher/media/
// tool_output → tool runtime, plugins → Spec 10, mcp → Spec 08, etc.).

// Command is a named slash-command template (v2 "commands").
type Command struct {
	Template    string `json:"template"`
	Description string `json:"description,omitempty"`
	Agent       string `json:"agent,omitempty"`
	Model       string `json:"model,omitempty"`
	Variant     string `json:"variant,omitempty"`
	Subtask     bool   `json:"subtask,omitempty"`
}

// Reference is a named local directory or git repository available as external
// context (v2 "references"). The v2 shape accepts a bare string, a git object
// or a local object; all are normalised into this struct.
type Reference struct {
	Path        string `json:"path,omitempty"`
	Repository  string `json:"repository,omitempty"`
	Branch      string `json:"branch,omitempty"`
	Description string `json:"description,omitempty"`
	Hidden      bool   `json:"hidden,omitempty"`
}

// UnmarshalJSON accepts a string ("path" or "repository"), or a local/git object.
func (r *Reference) UnmarshalJSON(data []byte) error {
	var s string
	if err := json.Unmarshal(data, &s); err == nil {
		*r = Reference{Path: s}
		return nil
	}
	type ref Reference
	var v ref
	if err := json.Unmarshal(data, &v); err != nil {
		return err
	}
	*r = Reference(v)
	return nil
}

// SkillPaths is the v2 "skills" key: a list of paths/URLs, or an object with
// "paths" and "urls" (the deprecated object form). Both normalise to Paths+URLs.
type SkillPaths struct {
	Paths []string `json:"paths,omitempty"`
	URLs  []string `json:"urls,omitempty"`
}

// UnmarshalJSON accepts a bare array or the {paths, urls} object.
func (s *SkillPaths) UnmarshalJSON(data []byte) error {
	var list []string
	if err := json.Unmarshal(data, &list); err == nil {
		s.Paths = list
		return nil
	}
	type skills SkillPaths
	var v skills
	if err := json.Unmarshal(data, &v); err != nil {
		return err
	}
	*s = SkillPaths(v)
	return nil
}

// FormatterConfig is the v2 "formatter" key: disabled/enabled as a boolean, or a
// per-formatter override map.
type FormatterConfig struct {
	// Enabled is set when the key is a bare boolean.
	Enabled *bool `json:"-"`
	// Formatters holds the per-name overrides when the key is an object.
	Formatters map[string]FormatterEntry `json:"-"`
}

// FormatterEntry configures one formatter (v2 Config.Formatter.EntryEncoded).
type FormatterEntry struct {
	Disabled    bool              `json:"disabled,omitempty"`
	Command     []string          `json:"command,omitempty"`
	Environment map[string]string `json:"environment,omitempty"`
	Extensions  []string          `json:"extensions,omitempty"`
}

func (f *FormatterConfig) UnmarshalJSON(data []byte) error {
	var b bool
	if err := json.Unmarshal(data, &b); err == nil {
		f.Enabled = &b
		return nil
	}
	var m map[string]FormatterEntry
	if err := json.Unmarshal(data, &m); err != nil {
		return err
	}
	f.Formatters = m
	return nil
}

// WatcherConfig is the v2 "watcher" key.
type WatcherConfig struct {
	Ignore []string `json:"ignore,omitempty"`
}

// MediaConfig is the v2 "media" key.
type MediaConfig struct {
	Image ImageConfig `json:"image,omitempty"`
}

// ImageConfig bounds image attachments (v2 media.image).
type ImageConfig struct {
	AutoResize    *bool `json:"auto_resize,omitempty"`
	MaxWidth      int   `json:"max_width,omitempty"`
	MaxHeight     int   `json:"max_height,omitempty"`
	MaxBase64Size int   `json:"max_base64_bytes,omitempty"`
}

// ToolOutputConfig truncates tool output (v2 "tool_output").
type ToolOutputConfig struct {
	MaxLines int `json:"max_lines,omitempty"`
	MaxBytes int `json:"max_bytes,omitempty"`
}

// WebsearchConfig is the v2 "websearch" key: disabled as a bare false, or a
// provider selection object.
type WebsearchConfig struct {
	Disabled *bool  `json:"-"`
	Provider string `json:"provider,omitempty"`
}

func (w *WebsearchConfig) UnmarshalJSON(data []byte) error {
	var b bool
	if err := json.Unmarshal(data, &b); err == nil {
		// v2: "websearch": false disables web search.
		disabled := !b
		w.Disabled = &disabled
		return nil
	}
	type websearch WebsearchConfig
	var v websearch
	if err := json.Unmarshal(data, &v); err != nil {
		return err
	}
	*w = WebsearchConfig(v)
	return nil
}

// CompactionConfig is the v2 "compaction" key.
type CompactionConfig struct {
	Auto   *bool          `json:"auto,omitempty"`
	Keep   CompactionKeep `json:"keep,omitempty"`
	Buffer int            `json:"buffer,omitempty"`
}

// CompactionKeep bounds retained tokens during compaction.
type CompactionKeep struct {
	Tokens int `json:"tokens,omitempty"`
}

// WarmingConfig is the v2 "warming" key: a bare boolean or an object.
type WarmingConfig struct {
	Enabled *bool `json:"-"`
}

func (w *WarmingConfig) UnmarshalJSON(data []byte) error {
	var b bool
	if err := json.Unmarshal(data, &b); err == nil {
		w.Enabled = &b
		return nil
	}
	// Any object form means "enabled with defaults"; keep Enabled at nil.
	var m map[string]any
	if err := json.Unmarshal(data, &m); err != nil {
		return err
	}
	enabled := true
	w.Enabled = &enabled
	return nil
}

// WorktreeConfig is the v2 "worktree" key.
type WorktreeConfig struct {
	Directory string `json:"directory,omitempty"`
}

// PluginSpec is one entry of the v2 "plugins" list (Spec 10). It accepts either
// a bare package/path string or an object {"package": ..., "options": {...}}.
// A package starting with "-" disables the named plugin.
type PluginSpec struct {
	Package  string         `json:"package"`
	Options  map[string]any `json:"options,omitempty"`
	Disabled bool           `json:"-"`
}

// UnmarshalJSON accepts a string or the {package, options} object form.
func (p *PluginSpec) UnmarshalJSON(data []byte) error {
	var s string
	if err := json.Unmarshal(data, &s); err == nil {
		p.setPackage(s)
		return nil
	}
	type spec PluginSpec
	var v spec
	if err := json.Unmarshal(data, &v); err != nil {
		return err
	}
	*p = PluginSpec(v)
	p.setPackage(v.Package)
	return nil
}

func (p *PluginSpec) setPackage(name string) {
	if strings.HasPrefix(name, "-") {
		p.Disabled = true
		p.Package = strings.TrimPrefix(name, "-")
		return
	}
	p.Package = name
}

// auxConfig mirrors the auxiliary keys of Config so they can be decoded from the
// raw merged map with json (honouring the custom UnmarshalJSON on polymorphic
// types).
type auxConfig struct {
	Instructions []string                  `json:"instructions,omitempty"`
	Commands     map[string]Command        `json:"commands,omitempty"`
	References   map[string]Reference      `json:"references,omitempty"`
	Skills       SkillPaths                `json:"skills,omitempty"`
	Formatter    FormatterConfig           `json:"formatter,omitempty"`
	Watcher      WatcherConfig             `json:"watcher,omitempty"`
	Snapshots    *bool                     `json:"snapshots,omitempty"`
	Media        MediaConfig               `json:"media,omitempty"`
	ToolOutput   ToolOutputConfig          `json:"tool_output,omitempty"`
	Websearch    WebsearchConfig           `json:"websearch,omitempty"`
	Compaction   CompactionConfig          `json:"compaction,omitempty"`
	Warming      WarmingConfig             `json:"warming,omitempty"`
	Worktree     WorktreeConfig            `json:"worktree,omitempty"`
	Update       string                    `json:"update,omitempty"`
	Username     string                    `json:"username,omitempty"`
	Permissions  []agentdef.PermissionRule `json:"permissions,omitempty"`
	Policies     []agentdef.PermissionRule `json:"policies,omitempty"`
	Plugins      []PluginSpec              `json:"plugins,omitempty"`
}

// applyAuxConfig decodes the auxiliary v2 keys from the merged config map into
// cfg, then applies their defaults and validates them.
func applyAuxConfig(merged map[string]any) error {
	if cfg == nil {
		return nil
	}
	raw, err := json.Marshal(merged)
	if err != nil {
		return err
	}
	var aux auxConfig
	if err := json.Unmarshal(raw, &aux); err != nil {
		return err
	}

	cfg.Instructions = aux.Instructions
	cfg.Commands = aux.Commands
	cfg.References = aux.References
	cfg.Skills = aux.Skills
	cfg.Formatter = aux.Formatter
	cfg.Watcher = aux.Watcher
	cfg.Snapshots = aux.Snapshots
	cfg.Media = aux.Media
	cfg.ToolOutput = aux.ToolOutput
	cfg.Websearch = aux.Websearch
	cfg.Compaction = aux.Compaction
	cfg.Warming = aux.Warming
	cfg.Worktree = aux.Worktree
	cfg.Update = aux.Update
	cfg.Username = aux.Username
	cfg.Permissions = aux.Permissions
	cfg.Policies = aux.Policies
	cfg.Plugins = aux.Plugins

	applyAuxDefaults()
	validateAux()
	return nil
}

// Default tool-output truncation thresholds, matching opencode v2.
const (
	defaultToolOutputMaxLines = 2000
	defaultToolOutputMaxBytes = 51200
)

// applyAuxDefaults fills in v2 defaults for the auxiliary keys.
func applyAuxDefaults() {
	if cfg.ToolOutput.MaxLines <= 0 {
		cfg.ToolOutput.MaxLines = defaultToolOutputMaxLines
	}
	if cfg.ToolOutput.MaxBytes <= 0 {
		cfg.ToolOutput.MaxBytes = defaultToolOutputMaxBytes
	}
	if cfg.Compaction.Auto == nil {
		auto := true
		cfg.Compaction.Auto = &auto
	}
}

// validateAux warns about and normalises invalid auxiliary keys. It never
// aborts, matching the lenient handling elsewhere.
func validateAux() {
	switch cfg.Update {
	case "", "disable", "notify", "auto":
	default:
		logging.Warn("invalid update value, ignoring", "value", cfg.Update)
		cfg.Update = ""
	}

	for name, cmd := range cfg.Commands {
		if cmd.Template == "" {
			logging.Warn("command has no template, ignoring", "command", name)
			delete(cfg.Commands, name)
		}
	}

	for name, ref := range cfg.References {
		if ref.Path == "" && ref.Repository == "" {
			logging.Warn("reference has neither path nor repository, ignoring", "reference", name)
			delete(cfg.References, name)
		}
	}
}
