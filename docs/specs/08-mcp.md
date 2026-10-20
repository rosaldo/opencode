# Spec 08 — MCP: HTTP/streamable, resources/prompts, OAuth e ciclo de vida

Onda 2. Depende de: 11 (chave `mcp` na config, opcional). Toca 09 (gestão na TUI).

## Objetivo

Modernizar o suporte a MCP: transporte HTTP/streamable além de stdio/sse, uso de **resources** e
**prompts** além de tools, OAuth para servidores remotos, **ciclo de vida** (restart/enable/disable),
`timeout` e escopo por-agente.

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
  formato (`type: local|remote`, `command` como array, `enabled`, `timeout` global/por-server).
  Preferir **adicionar** `enabled`/`timeout` ao `MCPServer` atual e mapear `local|remote`.

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

## Riscos

- OAuth adiciona complexidade e superfície de segurança — limitar a servers explícitos.
- Dois formatos de config (`mcpServers` legado vs `mcp`) — garantir merge sem ambiguidade.
- Escopo grande; considerar 08a (HTTP + lifecycle) e 08b (resources/prompts/OAuth).
