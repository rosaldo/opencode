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

## Status

- **10a loader + manifesto** — Verificado. `Config.Plugins []PluginSpec` (`internal/config/aux.go`;
  string, `{package,options}` e `"-name"` desabilita). Pacote **novo** `internal/plugin`:
  `Loader` descobre em `<projeto>/.opencode/plugins`, `~/.config/opencode/plugins` e
  `/etc/opencode/plugins`; lê `plugin.json` (`Manifest`: name/version/command/hooks/tools);
  plugins por path explícito, por nome (busca nos diretórios) ou no `PATH`; ordem explícita
  primeiro; `"-name"` remove; falha de um plugin vira aviso e não derruba o resto. Testes:
  `internal/plugin/loader_test.go`, `internal/config/plugins_test.go`. Schema emite `plugins`.
- **10b protocolo + hooks** — Verificado. Modelo **(A) processo externo via stdio JSON-RPC**
  (`client.go`): `Start`, `Call`, `RunHook`, `Close`; linhas JSON com `id`/`method`/`params`/`result`.
  `Manager` (`manager.go`): `Load` (isola falhas de start), `RunToolBeforeExecute`
  (**primeiro que nega vence**, mutações mescladas), `RunToolAfterExecute`, `RunSessionStart`,
  `RunChatParams` (último model/params vence); timeout de 5s por hook; singleton
  `SetManager`/`GetManager`. Integração no loop de tools (`internal/llm/agent/agent.go`): o hook
  `tool.before_execute` pode **negar** (vira erro de tool) ou **mutar** o input; `tool.after_execute`
  é notificado. Boot/close em `cmd/root.go`/`internal/app` (`initPlugins`, `Shutdown`). Testes:
  `internal/plugin/client_test.go` (executável fake via `TestHelperProcess`; deny de `bash`,
  mutate, chat.params, isolamento).
- **10c custom tools + UI** — Verificado. Tools declaradas por plugins são registradas no conjunto
  de tools do agente e executadas com gating de permissão; há um diálogo de plugins na TUI.
  - Protocolo: `Client.CallTool` (método `tool/call`, `{name,arguments}` → `{content,isError}`).
  - `Manager.Tools()` (metadados: `ToolMeta` com `FullName()` = `plugin_<plugin>_<tool>`),
    `Manager.CallTool(ctx, fullName, args)` (timeout de 5s) e `Manager.Summaries()` (nome, hooks,
    tools — usado pela UI).
  - `internal/llm/agent/plugin-tools.go`: `pluginTool` implementa `tools.BaseTool` (análoga a
    `mcpTool`), pedindo permissão via `permission.Service` antes de executar; `GetPluginTools`
    recebe o serviço de permissão do agente. Injetada em `FullToolSet`
    (`tools_filter.go`) após as tools de MCP.
  - TUI: `dialog.PluginsDialog` (novo `internal/tui/components/dialog/plugins.go`) lista plugins
    com hooks/tools; aberto pelo comando `/plugins` (ou na command dialog) via
    `ShowPluginsDialogMsg`, fechado com `esc`/`q` (`ClosePluginsDialogMsg`).
  - Testes: `internal/llm/agent/plugin-tools_test.go` (injeção, nome/params, negação por política),
    `internal/plugin/client_test.go` (`Tools`/`CallTool`/`Summaries` com executável fake),
    `internal/tui/components/dialog/plugins_test.go` (render, estado vazio, fechar).

## Riscos

- Modelo de execução é a decisão central — prototipar (A) antes de comprometer API.
- Segurança: plugins executam código; exigir opt-in explícito e documentar confiança.
- Escopo amplo; fase pode ser dividida: 10a (loader+protocolo), 10b (hooks), 10c (tools+UI).
