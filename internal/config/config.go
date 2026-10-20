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
)

// MCPServer defines the configuration for a Model Control Protocol server.
type MCPServer struct {
	Command string            `json:"command"`
	Env     []string          `json:"env"`
	Args    []string          `json:"args"`
	Type    MCPType           `json:"type"`
	URL     string            `json:"url"`
	Headers map[string]string `json:"headers"`
}

type AgentName string

const (
	AgentCoder      AgentName = "coder"
	AgentSummarizer AgentName = "summarizer"
	AgentTask       AgentName = "task"
	AgentTitle      AgentName = "title"
)

// Agent defines configuration for different LLM models and their token limits.
type Agent struct {
	Model           models.ModelID `json:"model"`
	MaxTokens       int64          `json:"maxTokens"`
	ReasoningEffort string         `json:"reasoningEffort"` // For openai models low,medium,heigh
}

// AgentDefinition is the config-facing description of a custom agent declared
// under the "agent" key of the JSON configuration. It mirrors the frontmatter
// accepted in agent markdown files (Spec 01).
type AgentDefinition struct {
	Description     string          `json:"description,omitempty"`
	Mode            string          `json:"mode,omitempty"`
	Hidden          bool            `json:"hidden,omitempty"`
	Disabled        bool            `json:"disabled,omitempty"`
	Team            string          `json:"team,omitempty"`
	Model           string          `json:"model,omitempty"`
	MaxTokens       int64           `json:"maxTokens,omitempty"`
	ReasoningEffort string          `json:"reasoningEffort,omitempty"`
	Temperature     float64         `json:"temperature,omitempty"`
	Tools           map[string]bool `json:"tools,omitempty"`
	Permissions     map[string]any  `json:"permissions,omitempty"`
	Color           string          `json:"color,omitempty"`
	// Prompt is either inline text or a "{file:./path}" reference resolved
	// relative to the configuration file that declares it.
	Prompt string `json:"prompt,omitempty"`
}

// Provider defines configuration for an LLM provider.
type Provider struct {
	APIKey   string `json:"apiKey"`
	Disabled bool   `json:"disabled"`
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
}

// ShellConfig defines the configuration for the shell used by the bash tool.
type ShellConfig struct {
	Path string   `json:"path,omitempty"`
	Args []string `json:"args,omitempty"`
}

// Config is the main configuration structure for the application.
type Config struct {
	Data         Data                              `json:"data"`
	WorkingDir   string                            `json:"wd,omitempty"`
	MCPServers   map[string]MCPServer              `json:"mcpServers,omitempty"`
	Providers    map[models.ModelProvider]Provider `json:"providers,omitempty"`
	LSP          map[string]LSPConfig              `json:"lsp,omitempty"`
	Agents       map[AgentName]Agent               `json:"agents,omitempty"`
	// Agent declares custom agent definitions keyed by name (Spec 05).
	Agent map[string]AgentDefinition `json:"agent,omitempty"`
	// DefaultAgent selects the primary agent used at boot. Falls back to
	// "coder" when unset or invalid.
	DefaultAgent string `json:"defaultAgent,omitempty"`
	Debug        bool   `json:"debug,omitempty"`
	DebugLSP     bool                              `json:"debugLSP,omitempty"`
	ContextPaths []string                          `json:"contextPaths,omitempty"`
	TUI          TUIConfig                         `json:"tui"`
	Shell        ShellConfig                       `json:"shell,omitempty"`
	AutoCompact  bool                              `json:"autoCompact,omitempty"`
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

	// Read global config
	if err := readConfig(viper.ReadInConfig()); err != nil {
		return cfg, err
	}

	// Load and merge local config
	mergeLocalConfig(workingDir)

	setProviderDefaults()

	// Apply configuration to the struct
	if err := viper.Unmarshal(cfg); err != nil {
		return cfg, fmt.Errorf("failed to unmarshal config: %w", err)
	}

	applyDefaultValues()
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
		cfg.Agents = make(map[AgentName]Agent)
	}

	// Override the max tokens for title agent
	cfg.Agents[AgentTitle] = Agent{
		Model:     cfg.Agents[AgentTitle].Model,
		MaxTokens: 80,
	}

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
// global directories and the project directory, layering in the custom agent
// definitions declared in the global and local JSON configuration files.
func loadAgentDefinitions(workingDir string) error {
	globalDefs := loadConfigDefinitions(configFileDirs())
	localDefs := loadConfigDefinitions([]string{workingDir})

	reg, err := agentdef.Load(agentdef.Options{
		GlobalDirs:         agentSearchDirs(),
		GlobalDefinitions:  globalDefs,
		ProjectDir:         filepath.Join(workingDir, defaultDataDirectory),
		ProjectDefinitions: localDefs,
		Builtins:           builtinDefinitions(),
	})
	if err != nil {
		return err
	}
	agentdef.Set(reg)

	// Resolve the configured default agent (falls back to "coder").
	if cfg != nil {
		applyDefaultAgent(reg, cfg.DefaultAgent)
	}
	return nil
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
	logging.Warn("invalid defaultAgent, falling back to coder", "defaultAgent", name)
}

// configFileDirs returns the directories that may hold a global ".opencode.json",
// mirroring viper's search paths.
func configFileDirs() []string {
	var dirs []string
	home, err := os.UserHomeDir()
	if err == nil && home != "" {
		dirs = append(dirs, home)
	}
	if xdg := os.Getenv("XDG_CONFIG_HOME"); xdg != "" {
		dirs = append(dirs, filepath.Join(xdg, appName))
	} else if home != "" {
		dirs = append(dirs, filepath.Join(home, ".config", appName))
	}
	return dirs
}

// loadConfigDefinitions reads the "agent" map from the ".opencode.json" file in
// each dir (in order, later wins) and converts them into agent definitions. The
// prompt of each definition is resolved relative to the directory it was
// declared in.
func loadConfigDefinitions(dirs []string) []*agentdef.Definition {
	byName := make(map[string]*agentdef.Definition)
	order := make([]string, 0)
	for _, dir := range dirs {
		path := filepath.Join(dir, fmt.Sprintf(".%s.json", appName))
		data, err := os.ReadFile(path)
		if err != nil {
			continue
		}
		var fileCfg struct {
			Agent map[string]AgentDefinition `json:"agent"`
		}
		if err := json.Unmarshal(data, &fileCfg); err != nil {
			logging.Warn("failed to parse agent definitions from config", "file", path, "error", err)
			continue
		}
		for name, def := range fileCfg.Agent {
			if _, seen := byName[name]; !seen {
				order = append(order, name)
			}
			byName[name] = definitionFromConfig(name, def, dir)
		}
	}
	out := make([]*agentdef.Definition, 0, len(order))
	for _, name := range order {
		out = append(out, byName[name])
	}
	return out
}

// definitionFromConfig converts a config-facing AgentDefinition into an
// agentdef.Definition, resolving the "{file:...}" prompt form relative to baseDir.
func definitionFromConfig(name string, def AgentDefinition, baseDir string) *agentdef.Definition {
	mode := agentdef.Mode(strings.TrimSpace(def.Mode))
	switch mode {
	case "", agentdef.ModePrimary, agentdef.ModeSubagent, agentdef.ModeAll:
		// ok
	default:
		logging.Warn("invalid mode in agent definition, defaulting to all",
			"agent", name, "mode", def.Mode)
		mode = agentdef.ModeAll
	}

	d := &agentdef.Definition{
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
	if prompt := strings.TrimSpace(def.Prompt); prompt != "" {
		d.SetPrompt(resolvePrompt(prompt, baseDir))
	}
	return d
}

// resolvePrompt expands the "{file:./path}" prompt form into the file contents
// (path relative to baseDir). Any other value is returned as inline text.
func resolvePrompt(prompt, baseDir string) string {
	const prefix = "{file:"
	if !strings.HasPrefix(prompt, prefix) || !strings.HasSuffix(prompt, "}") {
		return prompt
	}
	rel := strings.TrimSpace(strings.TrimSuffix(strings.TrimPrefix(prompt, prefix), "}"))
	if rel == "" {
		return ""
	}
	path := rel
	if !filepath.IsAbs(path) {
		path = filepath.Join(baseDir, rel)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		logging.Warn("failed to read agent prompt file", "file", path, "error", err)
		return ""
	}
	return string(data)
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

// configureViper sets up viper's configuration paths and environment variables.
func configureViper() {
	viper.SetConfigName(fmt.Sprintf(".%s", appName))
	viper.SetConfigType("json")
	viper.AddConfigPath("$HOME")
	viper.AddConfigPath(fmt.Sprintf("$XDG_CONFIG_HOME/%s", appName))
	viper.AddConfigPath(fmt.Sprintf("$HOME/.config/%s", appName))
	viper.SetEnvPrefix(strings.ToUpper(appName))
	viper.AutomaticEnv()
}

// setDefaults configures default values for configuration options.
func setDefaults(debug bool) {
	viper.SetDefault("data.directory", defaultDataDirectory)
	viper.SetDefault("contextPaths", defaultContextPaths)
	viper.SetDefault("tui.theme", "opencode")
	viper.SetDefault("autoCompact", true)

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

// readConfig handles the result of reading a configuration file.
func readConfig(err error) error {
	if err == nil {
		return nil
	}

	// It's okay if the config file doesn't exist
	if _, ok := err.(viper.ConfigFileNotFoundError); ok {
		return nil
	}

	return fmt.Errorf("failed to read config: %w", err)
}

// mergeLocalConfig loads and merges configuration from the local directory.
func mergeLocalConfig(workingDir string) {
	local := viper.New()
	local.SetConfigName(fmt.Sprintf(".%s", appName))
	local.SetConfigType("json")
	local.AddConfigPath(workingDir)

	// Merge local config if it exists
	if err := local.ReadInConfig(); err == nil {
		viper.MergeConfigMap(local.AllSettings())
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
func validateAgent(cfg *Config, name AgentName, agent Agent) error {
	// Check if model exists
	// TODO:	If a copilot model is specified, but model is not found,
	// 		 	it might be new model. The https://api.githubcopilot.com/models
	// 		 	endpoint should be queried to validate if the model is supported.
	model, modelExists := models.SupportedModels[agent.Model]
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
	} else if providerCfg.Disabled || providerCfg.APIKey == "" {
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
		if providerCfg.APIKey == "" && !providerCfg.Disabled {
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
	inherit := cfg.Agents[AgentCoder].Model
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
func setDefaultModelForAgent(agent AgentName) bool {
	if hasCopilotCredentials() {
		maxTokens := int64(5000)
		if agent == AgentTitle {
			maxTokens = 80
		}

		cfg.Agents[agent] = Agent{
			Model:     models.CopilotGPT4o,
			MaxTokens: maxTokens,
		}
		return true
	}
	// Check providers in order of preference
	if apiKey := os.Getenv("ANTHROPIC_API_KEY"); apiKey != "" {
		maxTokens := int64(5000)
		if agent == AgentTitle {
			maxTokens = 80
		}
		cfg.Agents[agent] = Agent{
			Model:     models.Claude37Sonnet,
			MaxTokens: maxTokens,
		}
		return true
	}

	if apiKey := os.Getenv("OPENAI_API_KEY"); apiKey != "" {
		var model models.ModelID
		maxTokens := int64(5000)
		reasoningEffort := ""

		switch agent {
		case AgentTitle:
			model = models.GPT41Mini
			maxTokens = 80
		case AgentTask:
			model = models.GPT41Mini
		default:
			model = models.GPT41
		}

		// Check if model supports reasoning
		if modelInfo, ok := models.SupportedModels[model]; ok && modelInfo.CanReason {
			reasoningEffort = "medium"
		}

		cfg.Agents[agent] = Agent{
			Model:           model,
			MaxTokens:       maxTokens,
			ReasoningEffort: reasoningEffort,
		}
		return true
	}

	if apiKey := os.Getenv("OPENROUTER_API_KEY"); apiKey != "" {
		var model models.ModelID
		maxTokens := int64(5000)
		reasoningEffort := ""

		switch agent {
		case AgentTitle:
			model = models.OpenRouterClaude35Haiku
			maxTokens = 80
		case AgentTask:
			model = models.OpenRouterClaude37Sonnet
		default:
			model = models.OpenRouterClaude37Sonnet
		}

		// Check if model supports reasoning
		if modelInfo, ok := models.SupportedModels[model]; ok && modelInfo.CanReason {
			reasoningEffort = "medium"
		}

		cfg.Agents[agent] = Agent{
			Model:           model,
			MaxTokens:       maxTokens,
			ReasoningEffort: reasoningEffort,
		}
		return true
	}

	if apiKey := os.Getenv("GEMINI_API_KEY"); apiKey != "" {
		var model models.ModelID
		maxTokens := int64(5000)

		if agent == AgentTitle {
			model = models.Gemini25Flash
			maxTokens = 80
		} else {
			model = models.Gemini25
		}

		cfg.Agents[agent] = Agent{
			Model:     model,
			MaxTokens: maxTokens,
		}
		return true
	}

	if apiKey := os.Getenv("GROQ_API_KEY"); apiKey != "" {
		maxTokens := int64(5000)
		if agent == AgentTitle {
			maxTokens = 80
		}

		cfg.Agents[agent] = Agent{
			Model:     models.QWENQwq,
			MaxTokens: maxTokens,
		}
		return true
	}

	if hasAWSCredentials() {
		maxTokens := int64(5000)
		if agent == AgentTitle {
			maxTokens = 80
		}

		cfg.Agents[agent] = Agent{
			Model:           models.BedrockClaude37Sonnet,
			MaxTokens:       maxTokens,
			ReasoningEffort: "medium", // Claude models support reasoning
		}
		return true
	}

	if hasVertexAICredentials() {
		var model models.ModelID
		maxTokens := int64(5000)

		if agent == AgentTitle {
			model = models.VertexAIGemini25Flash
			maxTokens = 80
		} else {
			model = models.VertexAIGemini25
		}

		cfg.Agents[agent] = Agent{
			Model:     model,
			MaxTokens: maxTokens,
		}
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

	existingAgentCfg := cfg.Agents[agentName]

	model, ok := models.SupportedModels[modelID]
	if !ok {
		return fmt.Errorf("model %s not supported", modelID)
	}

	maxTokens := existingAgentCfg.MaxTokens
	if model.DefaultMaxTokens > 0 {
		maxTokens = model.DefaultMaxTokens
	}

	newAgentCfg := Agent{
		Model:           modelID,
		MaxTokens:       maxTokens,
		ReasoningEffort: existingAgentCfg.ReasoningEffort,
	}
	cfg.Agents[agentName] = newAgentCfg

	if err := validateAgent(cfg, agentName, newAgentCfg); err != nil {
		// revert config update on failure
		cfg.Agents[agentName] = existingAgentCfg
		return fmt.Errorf("failed to update agent model: %w", err)
	}

	return updateCfgFile(func(config *Config) {
		if config.Agents == nil {
			config.Agents = make(map[AgentName]Agent)
		}
		config.Agents[agentName] = newAgentCfg
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
	if agentCfg, ok := cfg.Agents[name]; ok && agentCfg.Model != "" {
		return agentCfg.Model
	}
	if def, ok := agentdef.Get().Get(string(name)); ok && def.Model != "" {
		if id, ok := models.ResolveModelID(def.Model); ok {
			return id
		}
	}
	return cfg.Agents[AgentCoder].Model
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
