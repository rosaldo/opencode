# Spec 08 — MCP v2: `mcp.servers` local/remote, HTTP, resources/prompts, OAuth

Onda 2. Depende de: 11 (config v2). Toca 09 (gestão na TUI).

## Objetivo

Adotar o modelo de MCP v2 (`mcp.servers` com `type: local|remote`, `timeout`), transporte
HTTP/streamable além de stdio/sse, uso de **resources**/**prompts**, OAuth para remotos e
**ciclo de vida** (restart/enable/disable).

## Estado atual (pós-fase 5)

- Transportes: `stdio` e `sse` (`internal/config/config.go:23-26`; `internal/llm/agent/mcp-tools.go`
  stdio `:108,187`, sse `:118,199`); auth SSE via `headers`.
- Só **tools**: nome prefixado `%s_%s` (`:41`), gating de permissão (`:92`), defs cacheadas
  (`:149,214-223`), incluídas em `FullToolSet` (`tools_filter.go:50`).
- Boot pré-carrega tools com timeout de 30s: `cmd/root.go:195-207`.
- Cliente criado **por chamada** com `defer c.Close()` (`:49`) — sem reuso/restart.
- AUSENTE: HTTP/streamable (mcp-go presente, mas transporte não usado), resources/prompts, OAuth,
  ciclo de vida, `enabled`, TUI de MCP, escopo por-agente.

## Design

### Config: bloco `mcp`

```jsonc
{
  "mcp": {
    "timeout": 30000,
    "servers": {
      "playwright": { "type": "local", "command": ["bunx", "@playwright/mcp"], "enabled": true },
      "jira": { "type": "remote", "url": "https://jira.example.com/mcp", "enabled": false }
    }
  }
}
```

- Compat: manter `mcpServers` atual (command/type stdio|sse/env/headers/url) **e** aceitar o novo
  formato v2 (`mcp.servers` com `type: local|remote`, `command` como array, `enabled`, `timeout`
  global/por-server). Mapear `local`→stdio, `remote`→http/sse.

### Transportes

- Adicionar HTTP/streamable client (mcp-go) para `type: remote`/`http`, com headers e OAuth.

### Resources e prompts

- Além de `tools/list`+`tools/call`, consumir `resources/list`, `resources/read`, `prompts/list`,
  `prompts/get`.
- Expor resources como contexto/attachments e prompts como slash-commands (definir superfície
  mínima nesta fase; expansão fica para 09).

### OAuth

- Para servidores remotos com OAuth: fluxo de autorização (callback local) + armazenamento de token
  em `data.directory`. Reusar padrão do copilot token exchange como referência.

### Ciclo de vida

- Cliente **persistente** por servidor (não por chamada), com restart em falha, `enabled` na config,
  e um gerenciador (`MCPManager`) com estado por servidor.

## Mudanças (arquivos)

- `internal/llm/agent/mcp-tools.go`: transporte HTTP, resources/prompts, cliente persistente.
- Novo `internal/llm/agent/mcp_manager.go`: ciclo de vida, estado, restart.
- `internal/config/config.go`: `mcp` (novo) + extensões ao `MCPServer`; `MCPServers` compat.
- `cmd/root.go`: usar o manager no boot.
- `opencode-schema.json`, `cmd/schema/main.go`: `mcp`, `enabled`, `timeout`.

## Critérios de aceite

1. `mcpServers` atual continua funcionando.
2. `type: remote`/`http` conecta via HTTP/streamable (teste com servidor MCP falso).
3. Servidor desabilitado (`enabled:false`) não é iniciado nem expõe tools.
4. `timeout` global e por-server aplicados.
5. Falha de servidor não derruba o app; restart sob demanda.
6. Tools MCP continuam gateadas por permissão; nome prefixado estável.
7. (Se possível no escopo) resource e prompt MCP são listados e utilizáveis.

## Testes

- `internal/llm/agent`: cliente HTTP com servidor MCP fake (tools/resources/prompts).
- `internal/config`: parse de `mcp` e compat `mcpServers`.
- Manager: enabled/disabled, restart.

## Status

- **Verificado**: `mcp` (v2) decodificado do mapa bruto (`internal/config/mcp.go`): `mcp.timeout`
  global + `mcp.servers` com `type: local|remote|http`, `command` string ou array, `enabled` e
  `timeout` por-server. `local`→`stdio`, `remote`→`http`. Compat: `mcpServers` legado mantido e
  vence em caso de nome repetido. `MCPServer.IsEnabled()`.
- **Transporte HTTP/streamable**: `mcp-go` atualizado para **v0.37.0** (client `NewStreamableHttpClient`
  + test server); SSE/stdio mantidos.
- **Ciclo de vida** (`internal/llm/agent/mcp_manager.go`): `MCPManager` com cliente **persistente**
  por servidor (não mais por chamada), criação lazy, `Restart` em falha e `CloseAll` no shutdown
  (`internal/app/app.go`). Servidores `enabled:false` não são gerenciados nem expõem tools.
- **Resources/prompts** (critério 7, superfície mínima): `ListMCPResources`/`ReadMCPResource`,
  `ListMCPPrompts`/`GetMCPPrompt`. Integração com o loop do agente/slash-commands fica para a 09.
- `cel`/OAuth dos remotos **não** implementado nesta fase (fica em 08b): hoje o `mcp-go` aceita
  `WithHTTPOAuth`, mas o fluxo de callback/token-store não foi ligado.
- `opencode-schema.json`/`cmd/schema` emitem `mcp` + `enabled`/`timeout`.
- Testes: `internal/config/mcp_test.go` (parse v2, legado, precedência) e
  `internal/llm/agent/mcp_test.go` (servidor MCP HTTP fake: tools/resources/prompts + disabled).

## Riscos

- OAuth adiciona complexidade e superfície de segurança — limitar a servers explícitos.
- Dois formatos de config (`mcpServers` legado vs `mcp`) — garantir merge sem ambiguidade.
- Escopo grande; considerar 08a (HTTP + lifecycle) e 08b (resources/prompts/OAuth).
