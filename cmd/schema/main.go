package main

import (
	"encoding/json"
	"fmt"
	"os"

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
