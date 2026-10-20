// Package config manages application configuration from various sources.
package config

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/opencode-ai/opencode/internal/agentdef"
	"github.com/opencode-ai/opencode/internal/llm/models"
	"github.com/opencode-ai/opencode/internal/logging"
	"github.com/spf13/viper"
)

// MCPType defines the type of MCP (Model Control Protocol) server.
type MCPType string

// Supported MCP types
const (
	MCPStdio MCPType = "stdio"
	MCPSse   MCPType = "sse"
	// MCPLocal and MCPRemote are the v2 spellings; local behaves like stdio,
	// remote uses the streamable HTTP transport (Spec 08).
	MCPLocal  MCPType = "local"
	MCPRemote MCPType = "remote"
	MCPHTTP   MCPType = "http"
)

// MCPServer defines the configuration for a Model Control Protocol server.
type MCPServer struct {
	Command string            `json:"command"`
	Env     []string          `json:"env"`
	Args    []string          `json:"args"`
	Type    MCPType           `json:"type"`
	URL     string            `json:"url"`
	Headers map[string]string `json:"headers"`
	// Enabled is the v2 flag; nil means enabled. A disabled server is neither
	// started nor exposed (Spec 08).
	Enabled *bool `json:"enabled,omitempty"`
	// Timeout in milliseconds, overriding the global mcp.timeout.
	Timeout int `json:"timeout,omitempty"`
}

// IsEnabled reports whether the server should be started.
func (s MCPServer) IsEnabled() bool {
	return s.Enabled == nil || *s.Enabled
}

type AgentName string

const (
	AgentCoder      AgentName = "coder"
	AgentSummarizer AgentName = "summarizer"
	AgentTask       AgentName = "task"
	AgentTitle      AgentName = "title"
)

// AgentDefinition is the config-facing description of a custom agent declared
// under the "agents" (v2) key of the JSON configuration. It is an alias of the
// agentdef.AgentConfig shared with the markdown frontmatter, so both forms parse
// into the same agentdef.Definition (Spec 13).
type AgentDefinition = agentdef.AgentConfig

// Provider defines configuration for an LLM provider.
type Provider struct {
	// APIKey is the v1 flat spelling, kept for back-compat; it maps onto
	// Settings.APIKey when the latter is empty.
	APIKey   string `json:"apiKey"`
	Disabled bool   `json:"disabled"`
	// Settings holds the v2 connection settings. baseURL/headers/transport are
	// passed through to the provider SDKs (Spec 06).
	Settings ProviderSettings `json:"settings,omitempty"`
	// Models declares/overrides models for this provider (Spec 06b).
	Models map[string]ProviderModel `json:"models,omitempty"`
}

// ProviderSettings are the v2 per-provider connection settings.
type ProviderSettings struct {
	APIKey    string            `json:"apiKey,omitempty"`
	BaseURL   string            `json:"baseURL,omitempty"`
	Headers   map[string]string `json:"headers,omitempty"`
	Transport string            `json:"transport,omitempty"`
}

// ProviderModel declares or overrides a model in the provider catalog (Spec 06b).
type ProviderModel struct {
	ModelID string         `json:"modelID,omitempty"`
	Name    string         `json:"name,omitempty"`
	Limit   ModelLimit     `json:"limit,omitempty"`
	Options map[string]any `json:"options,omitempty"`
}

// ModelLimit bounds a model's tokens (v2 providers.<p>.models.<m>.limit).
type ModelLimit struct {
	Context int64 `json:"context,omitempty"`
	Output  int64 `json:"output,omitempty"`
}

// EffectiveAPIKey returns the API key from Settings, falling back to the
// deprecated flat APIKey field.
func (p Provider) EffectiveAPIKey() string {
	if p.Settings.APIKey != "" {
		return p.Settings.APIKey
	}
	return p.APIKey
}

// Data defines storage configuration.
type Data struct {
	Directory string `json:"directory,omitempty"`
}

// LSPConfig defines configuration for Language Server Protocol integration.
type LSPConfig struct {
	Disabled bool     `json:"enabled"`
	Command  string   `json:"command"`
	Args     []string `json:"args"`
	Options  any      `json:"options"`
}

// TUIConfig defines the configuration for the Terminal User Interface.
type TUIConfig struct {
	Theme string `json:"theme,omitempty"`
	// Keybinds maps a keybind id (e.g. "session_list") to a binding (e.g.
	// "ctrl+s", "<leader>a", or "none" to disable). See internal/tui (Spec 09).
	Keybinds map[string]string `json:"keybinds,omitempty"`
}

// ShellConfig defines the configuration for the shell used by the bash tool.
type ShellConfig struct {
	Path string   `json:"path,omitempty"`
	Args []string `json:"args,omitempty"`
}

// Config is the main configuration structure for the application.
type Config struct {
	Data       Data                 `json:"data"`
	WorkingDir string               `json:"wd,omitempty"`
	MCPServers map[string]MCPServer `json:"mcpServers,omitempty"`
	// MCPTimeout is the global mcp.timeout in milliseconds (Spec 08).
	MCPTimeout int                               `json:"-" mapstructure:"-"`
	Providers  map[models.ModelProvider]Provider `json:"providers,omitempty"`
	LSP        map[string]LSPConfig              `json:"lsp,omitempty"`
	// Agents is the unified agent map (opencode v2): builtin agents ("coder",
	// "task", "title", "summarizer") and custom agents share one namespace and
	// one shape (AgentConfig), mirroring the markdown frontmatter (Spec 13).
	Agents map[string]AgentDefinition `json:"agents,omitempty"`
	// Agent is the pre-v2 (single) custom agent map. It is read for
	// back-compatibility and merged into the same definitions; use "agents".
	//
	// Deprecated: use Agents.
	Agent map[string]AgentDefinition `json:"agent,omitempty"`
	// DefaultAgent selects the primary agent used at boot. Falls back to
	// "coder" when unset or invalid. Its v2 key is "default_agent"; the v1
	// "defaultAgent" spelling is read as a deprecated alias.
	DefaultAgent string `json:"default_agent,omitempty" mapstructure:"default_agent"`
	// Model is the v2 primary model ("provider/model" or a bare id). It is
	// applied to the primary agent unless that agent sets its own model.
	Model string `json:"model,omitempty"`
	// SmallModel is the v2 model for small/auxiliary tasks (session titles and
	// summarisation), applied unless those agents set their own model.
	SmallModel string `json:"small_model,omitempty" mapstructure:"small_model"`
	// DefaultAgentV1 is the deprecated v1 spelling ("defaultAgent"). It is read
	// only to back-fill DefaultAgent when the v2 key is absent.
	//
	// Deprecated: use DefaultAgent ("default_agent").
	DefaultAgentV1 string      `json:"defaultAgent,omitempty" mapstructure:"defaultagent"`
	Debug          bool        `json:"debug,omitempty"`
	DebugLSP       bool        `json:"debugLSP,omitempty"`
	ContextPaths   []string    `json:"contextPaths,omitempty"`
	TUI            TUIConfig   `json:"tui"`
	Shell          ShellConfig `json:"shell,omitempty"`
	AutoCompact    bool        `json:"autoCompact,omitempty"`
	// Share controls conversation sharing (Spec 12): "manual", "auto" or
	// "disabled". Empty is treated as "disabled".
	Share string `json:"share,omitempty"`

	// ---- Auxiliary v2 keys (Spec 11c): parsed/validated, behaviour lives in
	// the owning spec. They are decoded from the raw merged map (aux.go) so their
	// polymorphic JSON shapes are honoured; viper must skip them.

	Instructions []string                  `json:"instructions,omitempty" mapstructure:"-"`
	Commands     map[string]Command        `json:"commands,omitempty" mapstructure:"-"`
	References   map[string]Reference      `json:"references,omitempty" mapstructure:"-"`
	Skills       SkillPaths                `json:"skills,omitempty" mapstructure:"-"`
	Formatter    FormatterConfig           `json:"formatter,omitempty" mapstructure:"-"`
	Watcher      WatcherConfig             `json:"watcher,omitempty" mapstructure:"-"`
	Snapshots    *bool                     `json:"snapshots,omitempty" mapstructure:"-"`
	Media        MediaConfig               `json:"media,omitempty" mapstructure:"-"`
	ToolOutput   ToolOutputConfig          `json:"tool_output,omitempty" mapstructure:"-"`
	Websearch    WebsearchConfig           `json:"websearch,omitempty" mapstructure:"-"`
	Compaction   CompactionConfig          `json:"compaction,omitempty" mapstructure:"-"`
	Warming      WarmingConfig             `json:"warming,omitempty" mapstructure:"-"`
	Worktree     WorktreeConfig            `json:"worktree,omitempty" mapstructure:"-"`
	Update       string                    `json:"update,omitempty" mapstructure:"-"`
	Username     string                    `json:"username,omitempty" mapstructure:"-"`
	Permissions  []agentdef.PermissionRule `json:"permissions,omitempty" mapstructure:"-"`
	// Policies is the top-level governance list (provider.use, tool.use, MCP/
	// skill/plugin blocking). Evaluated like permissions: last matching wins,
	// and the highest-precedence config file comes last (Spec 07).
	Policies []agentdef.PermissionRule `json:"policies,omitempty" mapstructure:"-"`

	// Plugins is the ordered list of plugins to load (Spec 10). A bare string is
	// a package/path; an object may set options; a "-name" entry disables a
	// plugin discovered by convention.
	Plugins []PluginSpec `json:"plugins,omitempty" mapstructure:"-"`
}

// Application constants
const (
	defaultDataDirectory = ".opencode"
	defaultLogLevel      = "info"
	appName              = "opencode"

	MaxTokensFallbackDefault = 4096
)

var defaultContextPaths = []string{
	".github/copilot-instructions.md",
	".cursorrules",
	".cursor/rules/",
	"CLAUDE.md",
	"CLAUDE.local.md",
	"opencode.md",
	"opencode.local.md",
	"OpenCode.md",
	"OpenCode.local.md",
	"OPENCODE.md",
	"OPENCODE.local.md",
}

// Global configuration instance
var cfg *Config

// Load initializes the configuration from environment variables and config files.
// If debug is true, debug mode is enabled and log level is set to debug.
// It returns an error if configuration loading fails.
func Load(workingDir string, debug bool) (*Config, error) {
	if cfg != nil {
		return cfg, nil
	}

	cfg = &Config{
		WorkingDir: workingDir,
		MCPServers: make(map[string]MCPServer),
		Providers:  make(map[models.ModelProvider]Provider),
		LSP:        make(map[string]LSPConfig),
	}

	configureViper()
	setDefaults(debug)

	// Read every config file (v2 opencode.json(c) + legacy .opencode.json) in
	// precedence order, applying JSONC and string interpolation, then feed the
	// merged map to viper (Spec 11a).
	merged := readConfigMaps(workingDir)
	// viper lowercases every map key in place, which would corrupt model names
	// and header names; keep the raw map pristine for the aux/provider readers.
	viper.MergeConfigMap(deepCopyMap(merged))

	setProviderDefaults()

	// Apply configuration to the struct
	if err := viper.Unmarshal(cfg); err != nil {
		return cfg, fmt.Errorf("failed to unmarshal config: %w", err)
	}

	// Auxiliary v2 keys (polymorphic JSON shapes) are decoded from the raw
	// merged map so their custom UnmarshalJSON is honoured (Spec 11c).
	if err := applyAuxConfig(merged); err != nil {
		return cfg, fmt.Errorf("failed to load auxiliary config: %w", err)
	}

	// providers.<p>.settings/models are decoded from the raw map too: model
	// names contain dots, which viper would split into nested keys (Spec 06).
	applyProviderSettings(merged)

	// The v2 mcp block (servers with local/remote types, enabled, timeout) is
	// merged into the legacy MCPServers map (Spec 08).
	applyMCPConfig(merged)

	// Merge models declared under providers.<p>.models into the catalog so they
	// resolve like built-ins and their limits feed token budgets (Spec 06b).
	registerProviderModels()

	applyDefaultValues()
	applyModelSelection()
	defaultLevel := slog.LevelInfo
	if cfg.Debug {
		defaultLevel = slog.LevelDebug
	}
	if os.Getenv("OPENCODE_DEV_DEBUG") == "true" {
		loggingFile := fmt.Sprintf("%s/%s", cfg.Data.Directory, "debug.log")
		messagesPath := fmt.Sprintf("%s/%s", cfg.Data.Directory, "messages")

		// if file does not exist create it
		if _, err := os.Stat(loggingFile); os.IsNotExist(err) {
			if err := os.MkdirAll(cfg.Data.Directory, 0o755); err != nil {
				return cfg, fmt.Errorf("failed to create directory: %w", err)
			}
			if _, err := os.Create(loggingFile); err != nil {
				return cfg, fmt.Errorf("failed to create log file: %w", err)
			}
		}

		if _, err := os.Stat(messagesPath); os.IsNotExist(err) {
			if err := os.MkdirAll(messagesPath, 0o756); err != nil {
				return cfg, fmt.Errorf("failed to create directory: %w", err)
			}
		}
		logging.MessageDir = messagesPath

		sloggingFileWriter, err := os.OpenFile(loggingFile, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o666)
		if err != nil {
			return cfg, fmt.Errorf("failed to open log file: %w", err)
		}
		// Configure logger
		logger := slog.New(slog.NewTextHandler(sloggingFileWriter, &slog.HandlerOptions{
			Level: defaultLevel,
		}))
		slog.SetDefault(logger)
	} else {
		// Configure logger
		logger := slog.New(slog.NewTextHandler(logging.NewWriter(), &slog.HandlerOptions{
			Level: defaultLevel,
		}))
		slog.SetDefault(logger)
	}

	// Validate configuration
	if err := Validate(); err != nil {
		return cfg, fmt.Errorf("config validation failed: %w", err)
	}

	if cfg.Agents == nil {
		cfg.Agents = make(map[string]AgentDefinition)
	}

	// Override the max tokens for title agent
	title := cfg.Agents[string(AgentTitle)]
	title.MaxTokens = 80
	cfg.Agents[string(AgentTitle)] = title

	// Discover and register agent definitions (builtins + global + project).
	if err := loadAgentDefinitions(workingDir); err != nil {
		logging.Warn("failed to load agent definitions", "error", err)
	}
	validateDefinitions()
	return cfg, nil
}

// agentSearchDirs returns the global directories scanned for agent markdown
// files, mirroring the config search paths used by viper.
func agentSearchDirs() []string {
	var dirs []string
	home, err := os.UserHomeDir()
	if err == nil && home != "" {
		dirs = append(dirs, filepath.Join(home, ".opencode"))
	}
	xdg := os.Getenv("XDG_CONFIG_HOME")
	if xdg == "" && home != "" {
		xdg = filepath.Join(home, ".config")
	}
	if xdg != "" {
		dirs = append(dirs, filepath.Join(xdg, appName))
	}
	return dirs
}

// loadAgentDefinitions builds the process-wide agent registry from builtins,
// global directories and the project directory. Every input is an
// agentdef.Source registered in precedence order (later wins):
//
//	builtins < global markdown < config files (JSON, in precedence order) < project markdown
//
// Markdown and the JSON "agents"/"agent" maps share one Definition type, so
// there is a single code path (Spec 01, Spec 13). The JSON config files are
// themselves ordered by configFiles (global < ancestors < .opencode < legacy <
// system), so a project config wins over a global one (Spec 11a).
func loadAgentDefinitions(workingDir string) error {
	builtins := staticSource(builtinDefinitions())

	var globalFiles []agentdef.Source
	for _, dir := range agentSearchDirs() {
		globalFiles = append(globalFiles, agentdef.FileSource{Dir: dir})
	}

	projectDir := filepath.Join(workingDir, defaultDataDirectory)

	sources := []agentdef.Source{builtins}
	sources = append(sources, globalFiles...)
	sources = append(sources, jsonSources(workingDir)...)
	sources = append(sources, agentdef.FileSource{Dir: projectDir})

	reg, err := agentdef.Load(sources...)
	if err != nil {
		return err
	}
	agentdef.Set(reg)

	// Resolve the configured default agent (falls back to "coder").
	if cfg != nil {
		applyDefaultAgent(reg, cfg.effectiveDefaultAgent())
	}
	return nil
}

// staticSource adapts a fixed slice of definitions to the Source interface.
type staticSource []*agentdef.Definition

func (s staticSource) Definitions() ([]*agentdef.Definition, error) { return s, nil }

// effectiveDefaultAgent returns the configured default agent, honouring the
// deprecated v1 "defaultAgent" spelling when the v2 "default_agent" is unset.
func (c *Config) effectiveDefaultAgent() string {
	if strings.TrimSpace(c.DefaultAgent) != "" {
		return c.DefaultAgent
	}
	if strings.TrimSpace(c.DefaultAgentV1) != "" {
		logging.Warn("config key \"defaultAgent\" is deprecated, use \"default_agent\"")
		return c.DefaultAgentV1
	}
	return ""
}

// applyDefaultAgent selects the preferred default agent on reg. A name that is
// missing or disabled is warned about and ignored, leaving the registry default
// ("coder") in place.
func applyDefaultAgent(reg *agentdef.Registry, name string) {
	if strings.TrimSpace(name) == "" {
		return
	}
	if d, ok := reg.Get(name); ok && !d.Disabled {
		reg.SetDefault(name)
		return
	}
	logging.Warn("invalid default agent, falling back to coder", "agent", name)
}

// jsonSources reads the agent definitions declared in the configuration files
// (v2 opencode.json(c) and the legacy ".opencode.json"), in precedence order, and
// returns one Source per file. Strings in each declaration are interpolated
// relative to the directory of the file that declared them.
func jsonSources(workingDir string) []agentdef.Source {
	var sources []agentdef.Source
	for _, file := range configFiles(workingDir) {
		agents, err := readAgentConfigsFrom(file.path)
		if err != nil {
			logging.Warn("failed to read agent definitions from config", "file", file.path, "error", err)
			continue
		}
		if len(agents) == 0 {
			continue
		}
		sources = append(sources, agentdef.JSONSource{
			Agents:        interpolateAgentConfigs(agents, file.dir),
			BaseDir:       file.dir,
			ResolvePrompt: resolvePrompt,
		})
	}
	return sources
}

// readAgentConfigsFrom returns the merged agent declarations of one config file,
// combining the v2 "agents" map with the deprecated "agent" map (v2 wins). It
// tolerates JSONC (comments and trailing commas).
func readAgentConfigsFrom(path string) (map[string]AgentDefinition, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	var fileCfg struct {
		Agents map[string]AgentDefinition `json:"agents"`
		Agent  map[string]AgentDefinition `json:"agent"`
	}
	if err := json.Unmarshal([]byte(stripJsonc(string(data))), &fileCfg); err != nil {
		return nil, err
	}
	if len(fileCfg.Agent) > 0 {
		logging.Warn("config key \"agent\" is deprecated, use \"agents\"", "file", path)
	}
	out := make(map[string]AgentDefinition, len(fileCfg.Agents)+len(fileCfg.Agent))
	for name, def := range fileCfg.Agent {
		out[name] = def
	}
	for name, def := range fileCfg.Agents {
		out[name] = def
	}
	return out, nil
}

// interpolateAgentConfigs applies {env:}/{file:} interpolation to every string
// field of each agent declaration, using baseDir for relative paths (Spec 11a).
func interpolateAgentConfigs(agents map[string]AgentDefinition, baseDir string) map[string]AgentDefinition {
	out := make(map[string]AgentDefinition, len(agents))
	for name, def := range agents {
		def.Description = interpolate(def.Description, baseDir)
		def.Mode = interpolate(def.Mode, baseDir)
		def.Team = interpolate(def.Team, baseDir)
		def.Model = interpolate(def.Model, baseDir)
		def.ReasoningEffort = interpolate(def.ReasoningEffort, baseDir)
		def.Color = interpolate(def.Color, baseDir)
		def.System = interpolate(def.System, baseDir)
		out[name] = def
	}
	return out
}

// readAgentConfigs returns the agent declarations of the legacy ".opencode.json"
// in dir (back-compat path used by tests).
func readAgentConfigs(dir string) (map[string]AgentDefinition, error) {
	return readAgentConfigsFrom(filepath.Join(dir, legacyConfigFilename))
}

// resolvePrompt expands the interpolation placeholders in an agent prompt,
// resolving "{file:...}" relative to baseDir (the directory of the config file
// that declared it). See interpolate.
func resolvePrompt(prompt, baseDir string) string {
	return interpolate(prompt, baseDir)
}

// builtinDefinitions returns the definitions for the agents built into opencode.
// Their prompts are resolved by the prompt package.
func builtinDefinitions() []*agentdef.Definition {
	return []*agentdef.Definition{
		{
			Name:        string(AgentCoder),
			Description: "Primary coding agent with full tool access",
			Mode:        agentdef.ModePrimary,
			Builtin:     true,
		},
		{
			Name:        string(AgentTask),
			Description: "Read-only subagent for searching the codebase",
			Mode:        agentdef.ModeSubagent,
			Hidden:      true,
			Builtin:     true,
		},
		{
			Name:        string(AgentTitle),
			Description: "Generates short session titles",
			Mode:        agentdef.ModeSubagent,
			Hidden:      true,
			Builtin:     true,
		},
		{
			Name:        string(AgentSummarizer),
			Description: "Summarizes session context for compaction",
			Mode:        agentdef.ModeSubagent,
			Hidden:      true,
			Builtin:     true,
		},
	}
}

// configureViper sets up viper's environment variables. Configuration files are
// discovered separately by configFiles (Spec 11a) and merged in.
func configureViper() {
	viper.SetEnvPrefix(strings.ToUpper(appName))
	viper.AutomaticEnv()
}

// setDefaults configures default values for configuration options.
func setDefaults(debug bool) {
	viper.SetDefault("data.directory", defaultDataDirectory)
	viper.SetDefault("contextPaths", defaultContextPaths)
	viper.SetDefault("tui.theme", "opencode")
	viper.SetDefault("autoCompact", true)
	viper.SetDefault("share", "disabled")

	// Set default shell from environment or fallback to /bin/bash
	shellPath := os.Getenv("SHELL")
	if shellPath == "" {
		shellPath = "/bin/bash"
	}
	viper.SetDefault("shell.path", shellPath)
	viper.SetDefault("shell.args", []string{"-l"})

	if debug {
		viper.SetDefault("debug", true)
		viper.Set("log.level", "debug")
	} else {
		viper.SetDefault("debug", false)
		viper.SetDefault("log.level", defaultLogLevel)
	}
}

// setProviderDefaults configures LLM provider defaults based on provider provided by
// environment variables and configuration file.
func setProviderDefaults() {
	// Set all API keys we can find in the environment
	// Note: Viper does not default if the json apiKey is ""
	if apiKey := os.Getenv("ANTHROPIC_API_KEY"); apiKey != "" {
		viper.SetDefault("providers.anthropic.apiKey", apiKey)
	}
	if apiKey := os.Getenv("OPENAI_API_KEY"); apiKey != "" {
		viper.SetDefault("providers.openai.apiKey", apiKey)
	}
	if apiKey := os.Getenv("GEMINI_API_KEY"); apiKey != "" {
		viper.SetDefault("providers.gemini.apiKey", apiKey)
	}
	if apiKey := os.Getenv("GROQ_API_KEY"); apiKey != "" {
		viper.SetDefault("providers.groq.apiKey", apiKey)
	}
	if apiKey := os.Getenv("OPENROUTER_API_KEY"); apiKey != "" {
		viper.SetDefault("providers.openrouter.apiKey", apiKey)
	}
	if apiKey := os.Getenv("XAI_API_KEY"); apiKey != "" {
		viper.SetDefault("providers.xai.apiKey", apiKey)
	}
	if apiKey := os.Getenv("AZURE_OPENAI_ENDPOINT"); apiKey != "" {
		// api-key may be empty when using Entra ID credentials – that's okay
		viper.SetDefault("providers.azure.apiKey", os.Getenv("AZURE_OPENAI_API_KEY"))
	}
	if apiKey, err := LoadGitHubToken(); err == nil && apiKey != "" {
		viper.SetDefault("providers.copilot.apiKey", apiKey)
		if viper.GetString("providers.copilot.apiKey") == "" {
			viper.Set("providers.copilot.apiKey", apiKey)
		}
	}

	// Use this order to set the default models
	// 1. Copilot
	// 2. Anthropic
	// 3. OpenAI
	// 4. Google Gemini
	// 5. Groq
	// 6. OpenRouter
	// 7. AWS Bedrock
	// 8. Azure
	// 9. Google Cloud VertexAI

	// copilot configuration
	if key := viper.GetString("providers.copilot.apiKey"); strings.TrimSpace(key) != "" {
		viper.SetDefault("agents.coder.model", models.CopilotGPT4o)
		viper.SetDefault("agents.summarizer.model", models.CopilotGPT4o)
		viper.SetDefault("agents.task.model", models.CopilotGPT4o)
		viper.SetDefault("agents.title.model", models.CopilotGPT4o)
		return
	}

	// Anthropic configuration
	if key := viper.GetString("providers.anthropic.apiKey"); strings.TrimSpace(key) != "" {
		viper.SetDefault("agents.coder.model", models.Claude4Sonnet)
		viper.SetDefault("agents.summarizer.model", models.Claude4Sonnet)
		viper.SetDefault("agents.task.model", models.Claude4Sonnet)
		viper.SetDefault("agents.title.model", models.Claude4Sonnet)
		return
	}

	// OpenAI configuration
	if key := viper.GetString("providers.openai.apiKey"); strings.TrimSpace(key) != "" {
		viper.SetDefault("agents.coder.model", models.GPT41)
		viper.SetDefault("agents.summarizer.model", models.GPT41)
		viper.SetDefault("agents.task.model", models.GPT41Mini)
		viper.SetDefault("agents.title.model", models.GPT41Mini)
		return
	}

	// Google Gemini configuration
	if key := viper.GetString("providers.gemini.apiKey"); strings.TrimSpace(key) != "" {
		viper.SetDefault("agents.coder.model", models.Gemini25)
		viper.SetDefault("agents.summarizer.model", models.Gemini25)
		viper.SetDefault("agents.task.model", models.Gemini25Flash)
		viper.SetDefault("agents.title.model", models.Gemini25Flash)
		return
	}

	// Groq configuration
	if key := viper.GetString("providers.groq.apiKey"); strings.TrimSpace(key) != "" {
		viper.SetDefault("agents.coder.model", models.QWENQwq)
		viper.SetDefault("agents.summarizer.model", models.QWENQwq)
		viper.SetDefault("agents.task.model", models.QWENQwq)
		viper.SetDefault("agents.title.model", models.QWENQwq)
		return
	}

	// OpenRouter configuration
	if key := viper.GetString("providers.openrouter.apiKey"); strings.TrimSpace(key) != "" {
		viper.SetDefault("agents.coder.model", models.OpenRouterClaude37Sonnet)
		viper.SetDefault("agents.summarizer.model", models.OpenRouterClaude37Sonnet)
		viper.SetDefault("agents.task.model", models.OpenRouterClaude37Sonnet)
		viper.SetDefault("agents.title.model", models.OpenRouterClaude35Haiku)
		return
	}

	// XAI configuration
	if key := viper.GetString("providers.xai.apiKey"); strings.TrimSpace(key) != "" {
		viper.SetDefault("agents.coder.model", models.XAIGrok3Beta)
		viper.SetDefault("agents.summarizer.model", models.XAIGrok3Beta)
		viper.SetDefault("agents.task.model", models.XAIGrok3Beta)
		viper.SetDefault("agents.title.model", models.XAiGrok3MiniFastBeta)
		return
	}

	// AWS Bedrock configuration
	if hasAWSCredentials() {
		viper.SetDefault("agents.coder.model", models.BedrockClaude37Sonnet)
		viper.SetDefault("agents.summarizer.model", models.BedrockClaude37Sonnet)
		viper.SetDefault("agents.task.model", models.BedrockClaude37Sonnet)
		viper.SetDefault("agents.title.model", models.BedrockClaude37Sonnet)
		return
	}

	// Azure OpenAI configuration
	if os.Getenv("AZURE_OPENAI_ENDPOINT") != "" {
		viper.SetDefault("agents.coder.model", models.AzureGPT41)
		viper.SetDefault("agents.summarizer.model", models.AzureGPT41)
		viper.SetDefault("agents.task.model", models.AzureGPT41Mini)
		viper.SetDefault("agents.title.model", models.AzureGPT41Mini)
		return
	}

	// Google Cloud VertexAI configuration
	if hasVertexAICredentials() {
		viper.SetDefault("agents.coder.model", models.VertexAIGemini25)
		viper.SetDefault("agents.summarizer.model", models.VertexAIGemini25)
		viper.SetDefault("agents.task.model", models.VertexAIGemini25Flash)
		viper.SetDefault("agents.title.model", models.VertexAIGemini25Flash)
		return
	}
}

// hasAWSCredentials checks if AWS credentials are available in the environment.
func hasAWSCredentials() bool {
	// Check for explicit AWS credentials
	if os.Getenv("AWS_ACCESS_KEY_ID") != "" && os.Getenv("AWS_SECRET_ACCESS_KEY") != "" {
		return true
	}

	// Check for AWS profile
	if os.Getenv("AWS_PROFILE") != "" || os.Getenv("AWS_DEFAULT_PROFILE") != "" {
		return true
	}

	// Check for AWS region
	if os.Getenv("AWS_REGION") != "" || os.Getenv("AWS_DEFAULT_REGION") != "" {
		return true
	}

	// Check if running on EC2 with instance profile
	if os.Getenv("AWS_CONTAINER_CREDENTIALS_RELATIVE_URI") != "" ||
		os.Getenv("AWS_CONTAINER_CREDENTIALS_FULL_URI") != "" {
		return true
	}

	return false
}

// hasVertexAICredentials checks if VertexAI credentials are available in the environment.
func hasVertexAICredentials() bool {
	// Check for explicit VertexAI parameters
	if os.Getenv("VERTEXAI_PROJECT") != "" && os.Getenv("VERTEXAI_LOCATION") != "" {
		return true
	}
	// Check for Google Cloud project and location
	if os.Getenv("GOOGLE_CLOUD_PROJECT") != "" && (os.Getenv("GOOGLE_CLOUD_REGION") != "" || os.Getenv("GOOGLE_CLOUD_LOCATION") != "") {
		return true
	}
	return false
}

func hasCopilotCredentials() bool {
	// Check for explicit Copilot parameters
	if token, _ := LoadGitHubToken(); token != "" {
		return true
	}
	return false
}

// applyProviderSettings overlays the v2 providers.<p>.{settings,models} keys
// decoded from the raw merged map onto cfg.Providers. json is used (not viper)
// because model names may contain dots, which viper would split into nested
// keys (Spec 06).
func applyProviderSettings(merged map[string]any) {
	if cfg == nil {
		return
	}
	rawProviders, ok := merged["providers"]
	if !ok {
		return
	}
	b, err := json.Marshal(rawProviders)
	if err != nil {
		return
	}
	var decoded map[string]struct {
		Settings ProviderSettings         `json:"settings"`
		Models   map[string]ProviderModel `json:"models"`
	}
	if err := json.Unmarshal(b, &decoded); err != nil {
		logging.Warn("failed to parse providers config", "error", err)
		return
	}
	if cfg.Providers == nil {
		cfg.Providers = make(map[models.ModelProvider]Provider)
	}
	for name, pc := range decoded {
		key := models.ModelProvider(name)
		existing := cfg.Providers[key]
		existing.Settings = pc.Settings
		existing.Models = pc.Models
		cfg.Providers[key] = existing
	}
}

// registerProviderModels merges models declared under providers.<p>.models into
// the model catalog (Spec 06b). The model is registered under both the canonical
// "<provider>.<key>" id and, when free, the bare "<key>" id. limit.context feeds
// the context window and limit.output the default max tokens.
func registerProviderModels() {
	if cfg == nil {
		return
	}
	for providerName, providerCfg := range cfg.Providers {
		for key, m := range providerCfg.Models {
			id := string(providerName) + "." + key
			if m.ModelID == "" && key == "" {
				continue
			}
			model := models.Model{
				ID:       models.ModelID(id),
				Name:     m.Name,
				Provider: providerName,
				APIModel: m.ModelID,
			}
			if model.APIModel == "" {
				model.APIModel = key
			}
			if model.Name == "" {
				model.Name = key
			}
			model.ContextWindow = m.Limit.Context
			model.DefaultMaxTokens = m.Limit.Output
			models.RegisterModel(model)
		}
	}
}

// applyModelSelection wires the v2 top-level "model" and "small_model" keys to
// the agents they govern (Spec 11b):
//
//   - "model" sets the primary agent's model (the configured default agent,
//     falling back to "coder") unless that agent sets its own "agents.<name>.model";
//     subagents without their own model inherit the same value.
//   - "small_model" sets the session-title and summariser models unless those
//     agents set their own model.
//
// An explicit per-agent model always wins. An unresolvable top-level model is
// warned about and ignored (leaving the provider-derived default in place).
func applyModelSelection() {
	if cfg == nil {
		return
	}

	setIfUnset := func(agent string, ref string, key string) {
		if ref == "" || viper.InConfig("agents."+agent+".model") {
			return
		}
		id, ok := models.ResolveModelID(ref)
		if !ok {
			logging.Warn("unsupported model in config, keeping default", "key", key, "model", ref)
			return
		}
		def := cfg.Agents[agent]
		def.Model = string(id)
		cfg.Agents[agent] = def
	}

	if cfg.Model != "" {
		primary := cfg.effectiveDefaultAgent()
		if strings.TrimSpace(primary) == "" {
			primary = string(AgentCoder)
		}
		setIfUnset(primary, cfg.Model, "model")
		// Subagents without an explicit model inherit the primary model,
		// matching the "model" = default-for-everything semantics of v2.
		for name, def := range cfg.Agents {
			if name == primary || name == string(AgentTitle) || name == string(AgentSummarizer) {
				continue
			}
			if def.Mode == string(agentdef.ModeSubagent) {
				setIfUnset(name, cfg.Model, "model")
			}
		}
	}

	if cfg.SmallModel != "" {
		setIfUnset(string(AgentTitle), cfg.SmallModel, "small_model")
		setIfUnset(string(AgentSummarizer), cfg.SmallModel, "small_model")
	}
}

// applyDefaultValues sets default values for configuration fields that need processing.
func applyDefaultValues() {
	// Set default MCP type if not specified
	for k, v := range cfg.MCPServers {
		if v.Type == "" {
			v.Type = MCPStdio
			cfg.MCPServers[k] = v
		}
	}
}

// It validates model IDs and providers, ensuring they are supported.
func validateAgent(cfg *Config, name string, agent AgentDefinition) error {
	// Resolve "provider/model" references to a bare ModelID first.
	if agent.Model != "" {
		if id, ok := models.ResolveModelID(agent.Model); ok {
			agent.Model = string(id)
			cfg.Agents[name] = agent
		}
	}
	// Check if model exists
	// TODO:	If a copilot model is specified, but model is not found,
	// 		 	it might be new model. The https://api.githubcopilot.com/models
	// 		 	endpoint should be queried to validate if the model is supported.
	model, modelExists := models.SupportedModels[models.ModelID(agent.Model)]
	if !modelExists {
		logging.Warn("unsupported model configured, reverting to default",
			"agent", name,
			"configured_model", agent.Model)

		// Set default model based on available providers
		if setDefaultModelForAgent(name) {
			logging.Info("set default model for agent", "agent", name, "model", cfg.Agents[name].Model)
		} else {
			return fmt.Errorf("no valid provider available for agent %s", name)
		}
		return nil
	}

	// Check if provider for the model is configured
	provider := model.Provider
	providerCfg, providerExists := cfg.Providers[provider]

	if !providerExists {
		// Provider not configured, check if we have environment variables
		apiKey := getProviderAPIKey(provider)
		if apiKey == "" {
			logging.Warn("provider not configured for model, reverting to default",
				"agent", name,
				"model", agent.Model,
				"provider", provider)

			// Set default model based on available providers
			if setDefaultModelForAgent(name) {
				logging.Info("set default model for agent", "agent", name, "model", cfg.Agents[name].Model)
			} else {
				return fmt.Errorf("no valid provider available for agent %s", name)
			}
		} else {
			// Add provider with API key from environment
			cfg.Providers[provider] = Provider{
				APIKey: apiKey,
			}
			logging.Info("added provider from environment", "provider", provider)
		}
	} else if providerCfg.Disabled || providerCfg.EffectiveAPIKey() == "" {
		// Provider is disabled or has no API key
		logging.Warn("provider is disabled or has no API key, reverting to default",
			"agent", name,
			"model", agent.Model,
			"provider", provider)

		// Set default model based on available providers
		if setDefaultModelForAgent(name) {
			logging.Info("set default model for agent", "agent", name, "model", cfg.Agents[name].Model)
		} else {
			return fmt.Errorf("no valid provider available for agent %s", name)
		}
	}

	// Validate max tokens
	if agent.MaxTokens <= 0 {
		logging.Warn("invalid max tokens, setting to default",
			"agent", name,
			"model", agent.Model,
			"max_tokens", agent.MaxTokens)

		// Update the agent with default max tokens
		updatedAgent := cfg.Agents[name]
		if model.DefaultMaxTokens > 0 {
			updatedAgent.MaxTokens = model.DefaultMaxTokens
		} else {
			updatedAgent.MaxTokens = MaxTokensFallbackDefault
		}
		cfg.Agents[name] = updatedAgent
	} else if model.ContextWindow > 0 && agent.MaxTokens > model.ContextWindow/2 {
		// Ensure max tokens doesn't exceed half the context window (reasonable limit)
		logging.Warn("max tokens exceeds half the context window, adjusting",
			"agent", name,
			"model", agent.Model,
			"max_tokens", agent.MaxTokens,
			"context_window", model.ContextWindow)

		// Update the agent with adjusted max tokens
		updatedAgent := cfg.Agents[name]
		updatedAgent.MaxTokens = model.ContextWindow / 2
		cfg.Agents[name] = updatedAgent
	}

	// Validate reasoning effort for models that support reasoning
	if model.CanReason && provider == models.ProviderOpenAI || provider == models.ProviderLocal {
		if agent.ReasoningEffort == "" {
			// Set default reasoning effort for models that support it
			logging.Info("setting default reasoning effort for model that supports reasoning",
				"agent", name,
				"model", agent.Model)

			// Update the agent with default reasoning effort
			updatedAgent := cfg.Agents[name]
			updatedAgent.ReasoningEffort = "medium"
			cfg.Agents[name] = updatedAgent
		} else {
			// Check if reasoning effort is valid (low, medium, high)
			effort := strings.ToLower(agent.ReasoningEffort)
			if effort != "low" && effort != "medium" && effort != "high" {
				logging.Warn("invalid reasoning effort, setting to medium",
					"agent", name,
					"model", agent.Model,
					"reasoning_effort", agent.ReasoningEffort)

				// Update the agent with valid reasoning effort
				updatedAgent := cfg.Agents[name]
				updatedAgent.ReasoningEffort = "medium"
				cfg.Agents[name] = updatedAgent
			}
		}
	} else if !model.CanReason && agent.ReasoningEffort != "" {
		// Model doesn't support reasoning but reasoning effort is set
		logging.Warn("model doesn't support reasoning but reasoning effort is set, ignoring",
			"agent", name,
			"model", agent.Model,
			"reasoning_effort", agent.ReasoningEffort)

		// Update the agent to remove reasoning effort
		updatedAgent := cfg.Agents[name]
		updatedAgent.ReasoningEffort = ""
		cfg.Agents[name] = updatedAgent
	}

	return nil
}

// Validate checks if the configuration is valid and applies defaults where needed.
func Validate() error {
	if cfg == nil {
		return fmt.Errorf("config not loaded")
	}

	// Validate agent models
	for name, agent := range cfg.Agents {
		if err := validateAgent(cfg, name, agent); err != nil {
			return err
		}
	}

	// Validate providers
	for provider, providerCfg := range cfg.Providers {
		if providerCfg.EffectiveAPIKey() == "" && !providerCfg.Disabled {
			fmt.Printf("provider has no API key, marking as disabled %s", provider)
			logging.Warn("provider has no API key, marking as disabled", "provider", provider)
			providerCfg.Disabled = true
			cfg.Providers[provider] = providerCfg
		}
	}

	// Validate LSP configurations
	for language, lspConfig := range cfg.LSP {
		if lspConfig.Command == "" && !lspConfig.Disabled {
			logging.Warn("LSP configuration has no command, marking as disabled", "language", language)
			lspConfig.Disabled = true
			cfg.LSP[language] = lspConfig
		}
	}

	return nil
}

// validateDefinitions checks custom agent definitions loaded from JSON. A model
// that cannot be resolved is warned about and made to inherit the default
// agent's model; an invalid maxTokens is replaced with the model default. It
// never aborts, matching the lenient handling of builtin agents.
func validateDefinitions() {
	if cfg == nil {
		return
	}
	inherit := models.ModelID(cfg.Agents[string(AgentCoder)].Model)
	for _, def := range agentdef.Get().List() {
		if def.Builtin {
			continue
		}
		var modelInfo models.Model
		if def.Model == "" {
			def.Model = string(inherit)
			modelInfo = models.SupportedModels[inherit]
		} else if id, ok := models.ResolveModelID(def.Model); ok {
			def.Model = string(id)
			modelInfo = models.SupportedModels[id]
		} else {
			logging.Warn("unsupported model in agent definition, inheriting default",
				"agent", def.Name, "configured_model", def.Model, "inherited_model", inherit)
			def.Model = string(inherit)
			modelInfo = models.SupportedModels[inherit]
		}

		if def.MaxTokens <= 0 {
			if modelInfo.DefaultMaxTokens > 0 {
				def.MaxTokens = modelInfo.DefaultMaxTokens
			} else {
				def.MaxTokens = MaxTokensFallbackDefault
			}
		}
	}
}

// getProviderAPIKey gets the API key for a provider from environment variables
func getProviderAPIKey(provider models.ModelProvider) string {
	switch provider {
	case models.ProviderAnthropic:
		return os.Getenv("ANTHROPIC_API_KEY")
	case models.ProviderOpenAI:
		return os.Getenv("OPENAI_API_KEY")
	case models.ProviderGemini:
		return os.Getenv("GEMINI_API_KEY")
	case models.ProviderGROQ:
		return os.Getenv("GROQ_API_KEY")
	case models.ProviderAzure:
		return os.Getenv("AZURE_OPENAI_API_KEY")
	case models.ProviderOpenRouter:
		return os.Getenv("OPENROUTER_API_KEY")
	case models.ProviderBedrock:
		if hasAWSCredentials() {
			return "aws-credentials-available"
		}
	case models.ProviderVertexAI:
		if hasVertexAICredentials() {
			return "vertex-ai-credentials-available"
		}
	}
	return ""
}

// setDefaultModelForAgent sets a default model for an agent based on available providers
func setDefaultModelForAgent(agent string) bool {
	setModel := func(model models.ModelID, maxTokens int64, effort string) {
		def := cfg.Agents[agent]
		def.Model = string(model)
		def.MaxTokens = maxTokens
		def.ReasoningEffort = effort
		cfg.Agents[agent] = def
	}

	if hasCopilotCredentials() {
		maxTokens := int64(5000)
		if agent == string(AgentTitle) {
			maxTokens = 80
		}

		setModel(models.CopilotGPT4o, maxTokens, "")
		return true
	}
	// Check providers in order of preference
	if apiKey := os.Getenv("ANTHROPIC_API_KEY"); apiKey != "" {
		maxTokens := int64(5000)
		if agent == string(AgentTitle) {
			maxTokens = 80
		}
		setModel(models.Claude37Sonnet, maxTokens, "")
		return true
	}

	if apiKey := os.Getenv("OPENAI_API_KEY"); apiKey != "" {
		var model models.ModelID
		maxTokens := int64(5000)
		reasoningEffort := ""

		switch agent {
		case string(AgentTitle):
			model = models.GPT41Mini
			maxTokens = 80
		case string(AgentTask):
			model = models.GPT41Mini
		default:
			model = models.GPT41
		}

		// Check if model supports reasoning
		if modelInfo, ok := models.SupportedModels[model]; ok && modelInfo.CanReason {
			reasoningEffort = "medium"
		}

		setModel(model, maxTokens, reasoningEffort)
		return true
	}

	if apiKey := os.Getenv("OPENROUTER_API_KEY"); apiKey != "" {
		var model models.ModelID
		maxTokens := int64(5000)
		reasoningEffort := ""

		switch agent {
		case string(AgentTitle):
			model = models.OpenRouterClaude35Haiku
			maxTokens = 80
		case string(AgentTask):
			model = models.OpenRouterClaude37Sonnet
		default:
			model = models.OpenRouterClaude37Sonnet
		}

		// Check if model supports reasoning
		if modelInfo, ok := models.SupportedModels[model]; ok && modelInfo.CanReason {
			reasoningEffort = "medium"
		}

		setModel(model, maxTokens, reasoningEffort)
		return true
	}

	if apiKey := os.Getenv("GEMINI_API_KEY"); apiKey != "" {
		var model models.ModelID
		maxTokens := int64(5000)

		if agent == string(AgentTitle) {
			model = models.Gemini25Flash
			maxTokens = 80
		} else {
			model = models.Gemini25
		}

		setModel(model, maxTokens, "")
		return true
	}

	if apiKey := os.Getenv("GROQ_API_KEY"); apiKey != "" {
		maxTokens := int64(5000)
		if agent == string(AgentTitle) {
			maxTokens = 80
		}

		setModel(models.QWENQwq, maxTokens, "")
		return true
	}

	if hasAWSCredentials() {
		maxTokens := int64(5000)
		if agent == string(AgentTitle) {
			maxTokens = 80
		}

		setModel(models.BedrockClaude37Sonnet, maxTokens, "medium") // Claude models support reasoning
		return true
	}

	if hasVertexAICredentials() {
		var model models.ModelID
		maxTokens := int64(5000)

		if agent == string(AgentTitle) {
			model = models.VertexAIGemini25Flash
			maxTokens = 80
		} else {
			model = models.VertexAIGemini25
		}

		setModel(model, maxTokens, "")
		return true
	}

	return false
}

func updateCfgFile(updateCfg func(config *Config)) error {
	if cfg == nil {
		return fmt.Errorf("config not loaded")
	}

	// Get the config file path
	configFile := viper.ConfigFileUsed()
	var configData []byte
	if configFile == "" {
		homeDir, err := os.UserHomeDir()
		if err != nil {
			return fmt.Errorf("failed to get home directory: %w", err)
		}
		configFile = filepath.Join(homeDir, fmt.Sprintf(".%s.json", appName))
		logging.Info("config file not found, creating new one", "path", configFile)
		configData = []byte(`{}`)
	} else {
		// Read the existing config file
		data, err := os.ReadFile(configFile)
		if err != nil {
			return fmt.Errorf("failed to read config file: %w", err)
		}
		configData = data
	}

	// Parse the JSON
	var userCfg *Config
	if err := json.Unmarshal(configData, &userCfg); err != nil {
		return fmt.Errorf("failed to parse config file: %w", err)
	}

	updateCfg(userCfg)

	// Write the updated config back to file
	updatedData, err := json.MarshalIndent(userCfg, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal config: %w", err)
	}

	if err := os.WriteFile(configFile, updatedData, 0o644); err != nil {
		return fmt.Errorf("failed to write config file: %w", err)
	}

	return nil
}

// Get returns the current configuration.
// It's safe to call this function multiple times.
func Get() *Config {
	return cfg
}

// WorkingDirectory returns the current working directory from the configuration.
func WorkingDirectory() string {
	if cfg == nil {
		panic("config not loaded")
	}
	return cfg.WorkingDir
}

func UpdateAgentModel(agentName AgentName, modelID models.ModelID) error {
	if cfg == nil {
		panic("config not loaded")
	}

	name := string(agentName)
	existingAgentCfg := cfg.Agents[name]

	model, ok := models.SupportedModels[modelID]
	if !ok {
		return fmt.Errorf("model %s not supported", modelID)
	}

	maxTokens := existingAgentCfg.MaxTokens
	if model.DefaultMaxTokens > 0 {
		maxTokens = model.DefaultMaxTokens
	}

	newAgentCfg := existingAgentCfg
	newAgentCfg.Model = string(modelID)
	newAgentCfg.MaxTokens = maxTokens
	cfg.Agents[name] = newAgentCfg

	if err := validateAgent(cfg, name, newAgentCfg); err != nil {
		// revert config update on failure
		cfg.Agents[name] = existingAgentCfg
		return fmt.Errorf("failed to update agent model: %w", err)
	}

	return updateCfgFile(func(config *Config) {
		if config.Agents == nil {
			config.Agents = make(map[string]AgentDefinition)
		}
		config.Agents[name] = newAgentCfg
	})
}

// AgentModelID resolves the effective model for an agent by name: the agent's
// config entry (including overrides applied through the model dialog) wins,
// then the model declared in its definition, then the coder model. Used by the
// TUI to display the model of whichever agent is active.
func AgentModelID(name AgentName) models.ModelID {
	if cfg == nil {
		return ""
	}
	if agentCfg, ok := cfg.Agents[string(name)]; ok && agentCfg.Model != "" {
		return models.ModelID(agentCfg.Model)
	}
	if def, ok := agentdef.Get().Get(string(name)); ok && def.Model != "" {
		if id, ok := models.ResolveModelID(def.Model); ok {
			return id
		}
	}
	return models.ModelID(cfg.Agents[string(AgentCoder)].Model)
}

// UpdateTheme updates the theme in the configuration and writes it to the config file.
func UpdateTheme(themeName string) error {
	if cfg == nil {
		return fmt.Errorf("config not loaded")
	}

	// Update the in-memory config
	cfg.TUI.Theme = themeName

	// Update the file config
	return updateCfgFile(func(config *Config) {
		config.TUI.Theme = themeName
	})
}

// Tries to load Github token from all possible locations
func LoadGitHubToken() (string, error) {
	// First check environment variable
	if token := os.Getenv("GITHUB_TOKEN"); token != "" {
		return token, nil
	}

	// Get config directory
	var configDir string
	if xdgConfig := os.Getenv("XDG_CONFIG_HOME"); xdgConfig != "" {
		configDir = xdgConfig
	} else if runtime.GOOS == "windows" {
		if localAppData := os.Getenv("LOCALAPPDATA"); localAppData != "" {
			configDir = localAppData
		} else {
			configDir = filepath.Join(os.Getenv("HOME"), "AppData", "Local")
		}
	} else {
		configDir = filepath.Join(os.Getenv("HOME"), ".config")
	}

	// Try both hosts.json and apps.json files
	filePaths := []string{
		filepath.Join(configDir, "github-copilot", "hosts.json"),
		filepath.Join(configDir, "github-copilot", "apps.json"),
	}

	for _, filePath := range filePaths {
		data, err := os.ReadFile(filePath)
		if err != nil {
			continue
		}

		var config map[string]map[string]interface{}
		if err := json.Unmarshal(data, &config); err != nil {
			continue
		}

		for key, value := range config {
			if strings.Contains(key, "github.com") {
				if oauthToken, ok := value["oauth_token"].(string); ok {
					return oauthToken, nil
				}
			}
		}
	}

	return "", fmt.Errorf("GitHub token not found in standard locations")
}
