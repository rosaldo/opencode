# Spec 10 — Plugins: hooks, custom tools e integrações

Onda 2. Depende de: 11 (chave `plugins`) e 08 (hooks de tool/MCP). Ordem: por último antes de 12.

## Objetivo

Introduzir um mecanismo de plugins como no opencode TS: descoberta em
`.opencode/plugins/` e `~/.config/opencode/plugins/`, carregamento por nome de pacote, opções,
hooks de ciclo de vida e custom tools. No Go não há runtime JS — definir o modelo viável.

## Estado atual

- **AUSENTE.** Nenhum mecanismo de plugin/hook. Único match de "plugin" é um header em
  `copilot.go:168`, o README e o `.gitignore`.

## Design

### Superfície de config

```jsonc
{
  "plugins": [
    "opencode-example-plugin",
    { "package": "./plugins/local", "options": { "enabled": true } }
  ]
}
```

- `Config.Plugins []PluginSpec` (json `plugins`), ordenado; `"-name"` desabilita.
- Descoberta local em `<project>/.opencode/plugins/` e `~/.config/opencode/plugins/`.

### Modelo de execução (Go)

Duas opções; decidir na fase com um protótipo:

- **(A) Processos externos via protocolo** (preferida): plugin é um executável que fala um
  protocolo simples (JSON-RPC/stdio) — host e plugin definem um conjunto de **hooks** e **tools**.
  Independente de linguagem, alinhado ao `mcp-go` já presente.
- **(B) Go plugins nativos** (`plugin` package): limita a plugins compilados na mesma
  toolchain/versão de Go — viável só para extensões internas.

Recomendação: (A) como base, com um manifesto (`plugin.json`) declarando hooks e tools.

### Hooks

- Pontos mínimos: `session.start`, `message.before_send`, `tool.before_execute` (permite
  allow/deny/mutate), `tool.after_execute`, `chat.params` (altera model/temperature).
- Hooks de tool interagem com a spec 07 (permissão) — um hook pode negar/mutar.

### Custom tools

- Plugins registram tools que entram no `FullToolSet` (`internal/llm/agent/tools_filter.go`) com
  nome prefixado e gating de permissão, como MCP.

## Mudanças (arquivos)

- Novo pacote `internal/plugin`: manifesto, loader (local + pacote), protocolo (stdio), registry
  de hooks/tools.
- `internal/config/config.go`: `Plugin []PluginSpec`.
- `internal/llm/agent/agent.go`/`tools_filter.go`: injetar hooks e tools de plugins.
- `cmd/root.go`: carregar plugins no boot, ligar hooks.
- `internal/tui`: diálogo de plugins (listar/instalar/toggle) — coordena com 09.
- `opencode-schema.json`, `cmd/schema/main.go`: `plugins`.

## Critérios de aceite

1. Sem `plugins` na config, comportamento inalterado.
2. Plugin local (executável + manifesto) é carregado e seus hooks disparam.
3. Hook `tool.before_execute` consegue **negar** uma chamada de tool.
4. Tool declarada por plugin aparece para o modelo e passa por permissão.
5. `"-name"` desabilita um plugin; ordem respeitada.
6. Falha de plugin não derruba o app (isolamento).

## Testes

- `internal/plugin`: loader (descoberta local/pacote), manifesto, ordem/disable.
- Protocolo: teste com plugin fake (executável de teste) disparando hook e registrando tool.
- Integração: hook negando um `bash`.

## Riscos

- Modelo de execução é a decisão central — prototipar (A) antes de comprometer API.
- Segurança: plugins executam código; exigir opt-in explícito e documentar confiança.
- Escopo amplo; fase pode ser dividida: 10a (loader+protocolo), 10b (hooks), 10c (tools+UI).
