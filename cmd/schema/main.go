package main

import (
	"encoding/json"
	"fmt"
	"os"
	"sort"

	"github.com/opencode-ai/opencode/internal/config"
	"github.com/opencode-ai/opencode/internal/llm/models"
)

// JSONSchemaType represents a JSON Schema type
type JSONSchemaType struct {
	Type                 string           `json:"type,omitempty"`
	Description          string           `json:"description,omitempty"`
	Properties           map[string]any   `json:"properties,omitempty"`
	Required             []string         `json:"required,omitempty"`
	AdditionalProperties any              `json:"additionalProperties,omitempty"`
	Enum                 []any            `json:"enum,omitempty"`
	Items                map[string]any   `json:"items,omitempty"`
	OneOf                []map[string]any `json:"oneOf,omitempty"`
	AnyOf                []map[string]any `json:"anyOf,omitempty"`
	Default              any              `json:"default,omitempty"`
}

func main() {
	schema := generateSchema()

	// Pretty print the schema
	encoder := json.NewEncoder(os.Stdout)
	encoder.SetIndent("", "  ")
	if err := encoder.Encode(schema); err != nil {
		fmt.Fprintf(os.Stderr, "Error encoding schema: %v\n", err)
		os.Exit(1)
	}
}

func generateSchema() map[string]any {
	schema := map[string]any{
		"$schema":     "http://json-schema.org/draft-07/schema#",
		"title":       "OpenCode Configuration",
		"description": "Configuration schema for the OpenCode application",
		"type":        "object",
		"properties":  map[string]any{},
	}

	// Add Data configuration
	schema["properties"].(map[string]any)["data"] = map[string]any{
		"type":        "object",
		"description": "Storage configuration",
		"properties": map[string]any{
			"directory": map[string]any{
				"type":        "string",
				"description": "Directory where application data is stored",
				"default":     ".opencode",
			},
		},
		"required": []string{"directory"},
	}

	// Add working directory
	schema["properties"].(map[string]any)["wd"] = map[string]any{
		"type":        "string",
		"description": "Working directory for the application",
	}

	// Add debug flags
	schema["properties"].(map[string]any)["debug"] = map[string]any{
		"type":        "boolean",
		"description": "Enable debug mode",
		"default":     false,
	}

	schema["properties"].(map[string]any)["debugLSP"] = map[string]any{
		"type":        "boolean",
		"description": "Enable LSP debug mode",
		"default":     false,
	}

	schema["properties"].(map[string]any)["contextPaths"] = map[string]any{
		"type":        "array",
		"description": "Context paths for the application",
		"items": map[string]any{
			"type": "string",
		},
		"default": []string{
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
		},
	}

	schema["properties"].(map[string]any)["tui"] = map[string]any{
		"type":        "object",
		"description": "Terminal User Interface configuration",
		"properties": map[string]any{
			"theme": map[string]any{
				"type":        "string",
				"description": "TUI theme name",
				"default":     "opencode",
				"enum": []string{
					"opencode",
					"catppuccin",
					"dracula",
					"flexoki",
					"gruvbox",
					"monokai",
					"onedark",
					"tokyonight",
					"tron",
				},
			},
		},
	}

	// Add MCP servers
	schema["properties"].(map[string]any)["mcpServers"] = map[string]any{
		"type":        "object",
		"description": "Model Control Protocol server configurations",
		"additionalProperties": map[string]any{
			"type":        "object",
			"description": "MCP server configuration",
			"properties": map[string]any{
				"command": map[string]any{
					"type":        "string",
					"description": "Command to execute for the MCP server",
				},
				"env": map[string]any{
					"type":        "array",
					"description": "Environment variables for the MCP server",
					"items": map[string]any{
						"type": "string",
					},
				},
				"args": map[string]any{
					"type":        "array",
					"description": "Command arguments for the MCP server",
					"items": map[string]any{
						"type": "string",
					},
				},
				"type": map[string]any{
					"type":        "string",
					"description": "Type of MCP server",
					"enum":        []string{"stdio", "sse"},
					"default":     "stdio",
				},
				"url": map[string]any{
					"type":        "string",
					"description": "URL for SSE type MCP servers",
				},
				"headers": map[string]any{
					"type":        "object",
					"description": "HTTP headers for SSE type MCP servers",
					"additionalProperties": map[string]any{
						"type": "string",
					},
				},
			},
			"required": []string{"command"},
		},
	}

	// Add providers
	providerSchema := map[string]any{
		"type":        "object",
		"description": "LLM provider configurations",
		"additionalProperties": map[string]any{
			"type":        "object",
			"description": "Provider configuration",
			"properties": map[string]any{
				"apiKey": map[string]any{
					"type":        "string",
					"description": "API key for the provider",
				},
				"disabled": map[string]any{
					"type":        "boolean",
					"description": "Whether the provider is disabled",
					"default":     false,
				},
				"settings": map[string]any{
					"type":        "object",
					"description": "v2 provider connection settings",
					"properties": map[string]any{
						"apiKey": map[string]any{
							"type":        "string",
							"description": "API key for the provider",
						},
						"baseURL": map[string]any{
							"type":        "string",
							"description": "Override the provider API base URL",
						},
						"headers": map[string]any{
							"type":                 "object",
							"description":          "Extra HTTP headers sent with every request",
							"additionalProperties": map[string]any{"type": "string"},
						},
						"transport": map[string]any{
							"type":        "string",
							"description": "Transport to use",
							"enum":        []string{"http", "websocket"},
						},
					},
				},
				"models": map[string]any{
					"type":        "object",
					"description": "Declare or override models for this provider",
					"additionalProperties": map[string]any{
						"type": "object",
						"properties": map[string]any{
							"modelID": map[string]any{
								"type":        "string",
								"description": "Provider API model id",
							},
							"name": map[string]any{
								"type":        "string",
								"description": "Display name",
							},
							"limit": map[string]any{
								"type": "object",
								"properties": map[string]any{
									"context": map[string]any{"type": "integer", "description": "Context window in tokens"},
									"output":  map[string]any{"type": "integer", "description": "Default max output tokens"},
								},
							},
							"options": map[string]any{
								"type":                 "object",
								"description":          "Provider-specific model options",
								"additionalProperties": true,
							},
						},
					},
				},
			},
		},
	}

	// Add known providers
	knownProviders := []string{
		string(models.ProviderAnthropic),
		string(models.ProviderOpenAI),
		string(models.ProviderGemini),
		string(models.ProviderGROQ),
		string(models.ProviderOpenRouter),
		string(models.ProviderBedrock),
		string(models.ProviderAzure),
		string(models.ProviderVertexAI),
	}

	providerSchema["additionalProperties"].(map[string]any)["properties"].(map[string]any)["provider"] = map[string]any{
		"type":        "string",
		"description": "Provider type",
		"enum":        knownProviders,
	}

	schema["properties"].(map[string]any)["providers"] = providerSchema

	// Add agents (v2). Every agent — builtin override or custom — is described
	// by the same definition, whether it comes from the "agents" map of the
	// JSON config or from a markdown file's frontmatter (Spec 13).
	modelEnum := []string{}
	for modelID := range models.SupportedModels {
		modelEnum = append(modelEnum, string(modelID))
	}
	sort.Strings(modelEnum)

	agentDefSchema := map[string]any{
		"type":        "object",
		"description": "Agent definition (mirrors agent markdown frontmatter)",
		"properties": map[string]any{
			"description": map[string]any{
				"type":        "string",
				"description": "Short description shown in the agent switcher",
			},
			"mode": map[string]any{
				"type":        "string",
				"description": "How the agent may be used",
				"enum":        []string{"primary", "subagent", "all"},
			},
			"hidden": map[string]any{
				"type":        "boolean",
				"description": "Whether the agent is hidden from the switcher",
				"default":     false,
			},
			"disabled": map[string]any{
				"type":        "boolean",
				"description": "Whether the agent is disabled",
				"default":     false,
			},
			"team": map[string]any{
				"type":        "string",
				"description": "Department the agent belongs to",
			},
			"model": map[string]any{
				"type":        "string",
				"description": "Model ID for the agent (bare ID or provider/model)",
				"enum":        modelEnum,
			},
			"maxTokens": map[string]any{
				"type":        "integer",
				"description": "Maximum tokens for the agent",
				"minimum":     1,
			},
			"reasoningEffort": map[string]any{
				"type":        "string",
				"description": "Reasoning effort for models that support it (OpenAI, Anthropic)",
				"enum":        []string{"low", "medium", "high"},
			},
			"temperature": map[string]any{
				"type":        "number",
				"description": "Sampling temperature for the agent",
			},
			"tools": map[string]any{
				"type":        "object",
				"description": "Allow/deny tools by name; \"*\" is a wildcard",
				"additionalProperties": map[string]any{
					"type": "boolean",
				},
			},
			"permissions": map[string]any{
				"type":        "array",
				"description": "Ordered per-agent permission policy; the first matching rule wins",
				"items": map[string]any{
					"type":     "object",
					"required": []string{"action", "effect"},
					"properties": map[string]any{
						"action": map[string]any{
							"type":        "string",
							"description": "Permission key (shell, edit, read, list, webfetch, task) or tool/server name",
						},
						"resource": map[string]any{
							"type":        "string",
							"description": "Glob matched against the request value (command, subagent name); empty matches anything",
						},
						"effect": map[string]any{
							"type":        "string",
							"description": "Outcome when the rule matches",
							"enum":        []string{"allow", "ask", "deny"},
						},
					},
				},
			},
			"system": map[string]any{
				"type":        "string",
				"description": "Inline prompt text or a {file:./path} reference resolved relative to the config file",
			},
			"color": map[string]any{
				"type":        "string",
				"description": "Display color for the agent",
			},
		},
	}

	agentProperties := map[string]any{}
	knownAgents := []string{
		string(config.AgentCoder),
		string(config.AgentTask),
		string(config.AgentTitle),
	}
	for _, agentName := range knownAgents {
		agentProperties[agentName] = map[string]any{
			"$ref": "#/definitions/agent",
		}
	}

	// "agents" holds builtin overrides and custom agents alike.
	schema["properties"].(map[string]any)["agents"] = map[string]any{
		"type":                 "object",
		"description":          "Agent definitions keyed by agent name",
		"properties":           agentProperties,
		"additionalProperties": agentDefSchema,
	}

	// "agent" is the deprecated v1 spelling, read as a fallback.
	schema["properties"].(map[string]any)["agent"] = map[string]any{
		"type":                 "object",
		"description":          "Deprecated: use \"agents\"",
		"additionalProperties": agentDefSchema,
	}

	schema["definitions"] = map[string]any{
		"agent": agentDefSchema,
	}

	// Default agent selection, with the deprecated v1 spelling.
	schema["properties"].(map[string]any)["default_agent"] = map[string]any{
		"type":        "string",
		"description": "Primary agent selected at boot (falls back to \"coder\")",
	}
	schema["properties"].(map[string]any)["defaultAgent"] = map[string]any{
		"type":        "string",
		"description": "Deprecated: use \"default_agent\"",
	}

	// Top-level model selection (Spec 11b).
	schema["properties"].(map[string]any)["model"] = map[string]any{
		"type":        "string",
		"description": "Primary model (bare id or provider/model); applied to the primary agent unless it sets its own model",
		"enum":        modelEnum,
	}
	schema["properties"].(map[string]any)["small_model"] = map[string]any{
		"type":        "string",
		"description": "Model for small tasks (session title, summarisation); applied unless those agents set their own model",
		"enum":        modelEnum,
	}

	// Auxiliary v2 keys (Spec 11c): parsed and validated here; behaviour lives in
	// the owning spec.
	schema["properties"].(map[string]any)["instructions"] = map[string]any{
		"type":        "array",
		"description": "Additional paths or URLs supplying ambient instructions",
		"items":       map[string]any{"type": "string"},
	}
	schema["properties"].(map[string]any)["commands"] = map[string]any{
		"type":        "object",
		"description": "Named slash command definitions",
		"additionalProperties": map[string]any{
			"type":     "object",
			"required": []string{"template"},
			"properties": map[string]any{
				"template":    map[string]any{"type": "string"},
				"description": map[string]any{"type": "string"},
				"agent":       map[string]any{"type": "string"},
				"model":       map[string]any{"type": "string"},
				"variant":     map[string]any{"type": "string"},
				"subtask":     map[string]any{"type": "boolean"},
			},
		},
	}
	schema["properties"].(map[string]any)["references"] = map[string]any{
		"type":        "object",
		"description": "Named local directories or Git repositories available as external context",
		"additionalProperties": map[string]any{
			"anyOf": []map[string]any{
				{"type": "string"},
				{
					"type":     "object",
					"required": []string{"repository"},
					"properties": map[string]any{
						"repository":  map[string]any{"type": "string"},
						"branch":      map[string]any{"type": "string"},
						"description": map[string]any{"type": "string"},
						"hidden":      map[string]any{"type": "boolean"},
					},
				},
				{
					"type":     "object",
					"required": []string{"path"},
					"properties": map[string]any{
						"path":        map[string]any{"type": "string"},
						"description": map[string]any{"type": "string"},
						"hidden":      map[string]any{"type": "boolean"},
					},
				},
			},
		},
	}
	schema["properties"].(map[string]any)["skills"] = map[string]any{
		"description": "Additional paths or URLs to discover skills from",
		"anyOf": []map[string]any{
			{"type": "array", "items": map[string]any{"type": "string"}},
			{
				"type": "object",
				"properties": map[string]any{
					"paths": map[string]any{"type": "array", "items": map[string]any{"type": "string"}},
					"urls":  map[string]any{"type": "array", "items": map[string]any{"type": "string"}},
				},
			},
		},
	}
	schema["properties"].(map[string]any)["formatter"] = map[string]any{
		"description": "Enable built-in formatters or configure formatter overrides",
		"anyOf": []map[string]any{
			{"type": "boolean"},
			{
				"type": "object",
				"additionalProperties": map[string]any{
					"type": "object",
					"properties": map[string]any{
						"disabled":    map[string]any{"type": "boolean"},
						"command":     map[string]any{"type": "array", "items": map[string]any{"type": "string"}},
						"environment": map[string]any{"type": "object", "additionalProperties": map[string]any{"type": "string"}},
						"extensions":  map[string]any{"type": "array", "items": map[string]any{"type": "string"}},
					},
				},
			},
		},
	}
	schema["properties"].(map[string]any)["watcher"] = map[string]any{
		"type":        "object",
		"description": "Filesystem watcher configuration",
		"properties": map[string]any{
			"ignore": map[string]any{"type": "array", "items": map[string]any{"type": "string"}},
		},
	}
	schema["properties"].(map[string]any)["snapshots"] = map[string]any{
		"type":        "boolean",
		"description": "Enable snapshots used for undo and revert behaviour",
	}
	schema["properties"].(map[string]any)["media"] = map[string]any{
		"type":        "object",
		"description": "Media processing configuration",
		"properties": map[string]any{
			"image": map[string]any{
				"type": "object",
				"properties": map[string]any{
					"auto_resize":      map[string]any{"type": "boolean"},
					"max_width":        map[string]any{"type": "integer"},
					"max_height":       map[string]any{"type": "integer"},
					"max_base64_bytes": map[string]any{"type": "integer"},
				},
			},
		},
	}
	schema["properties"].(map[string]any)["tool_output"] = map[string]any{
		"type":        "object",
		"description": "Tool output truncation thresholds",
		"properties": map[string]any{
			"max_lines": map[string]any{"type": "integer", "description": "Maximum lines before truncation", "default": 2000},
			"max_bytes": map[string]any{"type": "integer", "description": "Maximum bytes before truncation", "default": 51200},
		},
	}
	schema["properties"].(map[string]any)["websearch"] = map[string]any{
		"description": "Web search provider selection",
		"anyOf": []map[string]any{
			{"type": "boolean"},
			{"type": "object", "properties": map[string]any{"provider": map[string]any{"type": "string"}}},
		},
	}
	schema["properties"].(map[string]any)["compaction"] = map[string]any{
		"type":        "object",
		"description": "Conversation compaction behaviour",
		"properties": map[string]any{
			"auto":   map[string]any{"type": "boolean", "default": true},
			"keep":   map[string]any{"type": "object", "properties": map[string]any{"tokens": map[string]any{"type": "integer"}}},
			"buffer": map[string]any{"type": "integer"},
		},
	}
	schema["properties"].(map[string]any)["warming"] = map[string]any{
		"description": "Keep recently active sessions warm with transient model requests",
		"anyOf":       []map[string]any{{"type": "boolean"}, {"type": "object"}},
	}
	schema["properties"].(map[string]any)["worktree"] = map[string]any{
		"type":        "object",
		"description": "Directory defaults for local worktree creation",
		"properties": map[string]any{
			"directory": map[string]any{"type": "string"},
		},
	}
	schema["properties"].(map[string]any)["update"] = map[string]any{
		"type":        "string",
		"description": "Disable updates, notify when one is available, or install automatically",
		"enum":        []string{"disable", "notify", "auto"},
	}
	schema["properties"].(map[string]any)["username"] = map[string]any{
		"type":        "string",
		"description": "Username displayed in conversations and used for telemetry identity",
	}
	schema["properties"].(map[string]any)["permissions"] = map[string]any{
		"type":        "array",
		"description": "Ordered global tool permission rules; the first matching rule wins",
		"items": map[string]any{
			"type":     "object",
			"required": []string{"action", "effect"},
			"properties": map[string]any{
				"action":   map[string]any{"type": "string"},
				"resource": map[string]any{"type": "string"},
				"effect":   map[string]any{"type": "string", "enum": []string{"allow", "ask", "deny"}},
			},
		},
	}

	// Shell configuration (fork: path + args).
	schema["properties"].(map[string]any)["shell"] = map[string]any{
		"type":        "object",
		"description": "Shell used for terminal and shell tool execution",
		"properties": map[string]any{
			"path": map[string]any{"type": "string", "description": "Shell executable path"},
			"args": map[string]any{"type": "array", "items": map[string]any{"type": "string"}},
		},
	}
	schema["properties"].(map[string]any)["autoCompact"] = map[string]any{
		"type":        "boolean",
		"description": "Automatically compact the context when it is full",
		"default":     true,
	}

	// Add LSP configuration
	schema["properties"].(map[string]any)["lsp"] = map[string]any{
		"type":        "object",
		"description": "Language Server Protocol configurations",
		"additionalProperties": map[string]any{
			"type":        "object",
			"description": "LSP configuration for a language",
			"properties": map[string]any{
				"disabled": map[string]any{
					"type":        "boolean",
					"description": "Whether the LSP is disabled",
					"default":     false,
				},
				"command": map[string]any{
					"type":        "string",
					"description": "Command to execute for the LSP server",
				},
				"args": map[string]any{
					"type":        "array",
					"description": "Command arguments for the LSP server",
					"items": map[string]any{
						"type": "string",
					},
				},
				"options": map[string]any{
					"type":        "object",
					"description": "Additional options for the LSP server",
				},
			},
			"required": []string{"command"},
		},
	}

	return schema
}
