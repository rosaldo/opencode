# Specs — Onda 2: paridade com o opencode TS (v2)

Índice da segunda onda. **Leia o `00-index.md` primeiro** (onda 1: agentes dinâmicos, fases 1–5,
`Verificado`).

## Alvo

Paridade com a geração **v2** do opencode TS, conforme o schema oficial
`https://opencode.ai/config.json` e os guias `opencode.ai/v2/docs/*`. O objetivo é fechar as
lacunas do fork Go (`v0.0.55`) nas áreas que **não** são agentes: **providers, permissions (+
policies), MCP, TUI, plugins, config e integração opencode.ai**.

> **Atenção — alvo é a v2, não a 1.19.3.** Nomes de chaves e formatos mudaram entre a 1.19.3, a v1
> recente e a v2. As specs abaixo miram a **v2**. Um resumo das diferenças que importam está na
> seção "Schema v2 — convenções" no fim deste arquivo.

Diagnóstico (inventário no HEAD pós-fase-5, ver seção "Estado atual" de cada spec).
> **Status atual: todas as áreas abaixo estão `Verificado`.** A tabela registra o ponto de partida
> (o que faltava antes da onda 2); o status vivo está no `00-index.md`.

| Área | Estado no HEAD pós-fase-5 (histórico) | Spec |
|------|--------------|------|
| Providers | era PARCIAL — hoje v2 completo + provider nativo `opencode` (Zen) e `/connect` | [06](06-providers.md) |
| Permissions/Policies | era PARCIAL — hoje `permissions` array + `policies` | [07](07-permissions.md) |
| MCP | era PARCIAL — hoje `mcp.servers` local/remote, streamable HTTP, resources/prompts, OAuth, ciclo de vida | [08](08-mcp.md) |
| TUI | era PARCIAL — hoje mouse, keybinds, sessões, paste, imagens, `/connect` | [09](09-tui.md) |
| Plugins | era AUSENTE — hoje loader, hooks, custom tools, UI | [10](10-plugins.md) |
| Config | era PARCIAL — hoje `opencode.json(c)` v2, interpolação, schema | [11](11-config.md) |
| opencode.ai | era AUSENTE — hoje serve/share, web UI, ACP e zen | [12](12-opencode-ai.md) |
| Agents v2 | era PARCIAL — hoje fonte única `Source`, mapa `agents`, `permissions` array | [13](13-agents-v2.md) |

## Ordem sugerida

1. **11 Config v2** — base: `opencode.json(c)`, `$schema`, `{env:}`/`{file:}`, chaves v2, gerador
   de schema. Tudo depende disto.
2. **13 Agents v2** — consolida a costura JSON↔`agentdef` (fonte única `Source`), mapa `agents`,
   `default_agent`, `system` inline e `permissions` array v2. Remove o "remendo" da fase 5.
3. **06 Providers** — `providers.<p>.models/settings`, `model`/`small_model`, catálogo.
4. **07 Permissions + Policies** — `permissions` array + `policies` (usa o tipo da Spec 13).
5. **08 MCP** — `mcp.servers` local/remote, HTTP, lifecycle.
6. **09 TUI** — keybinds (`tui.json`/`cli.json`), mouse, sessões, paste.
7. **10 Plugins** — loader, protocolo, hooks, tools.
8. **12 opencode.ai** — serve/share/ACP (e web).

## Convenções

- Protocolo do `00-index.md`: uma fase por spec, `go build ./... && go test ./...` ao concluir,
  atualizar o **Status** só com testes verdes.
- Manter a direção de dependências de `internal/agentdef`.
- Back-compat obrigatória com o formato atual (`.opencode.json`; `agent`/`defaultAgent` são lidos
  como *deprecados* e alimentam a **mesma** fonte única — Spec 13 —, sem segundo caminho de código).

## Schema v2 — convenções (diferenças que importam)

| Tema | v1 atual no Go | v2 (alvo) |
|------|----------------|-----------|
| Arquivo | `.opencode.json` | `opencode.json` / `opencode.jsonc` (`$schema: https://opencode.ai/config.json`) |
| Diretórios | `$HOME`, XDG/opencode | global `~/.config/opencode/`, projeto na raiz **e** `.opencode/`, sistema `/etc/opencode` |
| Agente default | `defaultAgent` | `default_agent` |
| Agentes | `agents` (builtins) + `agent` (custom) | `agents` (mapa único, builtin+custom) — Spec 13 |
| Prompt do agente | corpo do `.md` | `system` (inline no JSON) ou corpo do `.md` — Spec 13 |
| Permissões | `agent.<n>.permissions` (mapa) | `permissions` (array `{action,resource,effect}`) — Spec 13 |
| Políticas | — | `policies` (`provider.use`, `tool.use`, MCP/skill/plugin) |
| Ação shell | `bash` | `shell` |
| MCP | `mcpServers` | `mcp.servers` (`type: local\|remote`, `timeout`) |
| Providers | `providers.<p>.apiKey/disabled` | `providers.<p>.models/settings/headers` |
| Plugins | — | `plugins` |
| Modelos | `agents.<n>.model` | topo `model` / `small_model`, e `providers.<p>.models` |
| Interpolação | — | `{env:VAR}` e `{file:path}` em strings |
| Outras chaves | — | `commands`, `instructions`, `references`, `skills`, `formatter`, `watcher`, `snapshots`, `media`, `tool_output`, `websearch`, `compaction`, `warming`, `worktree`, `update`, `username`, `shell` |

