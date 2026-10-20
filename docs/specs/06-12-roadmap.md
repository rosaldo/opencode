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

Diagnóstico (inventário no HEAD pós-fase-5, ver seção "Estado atual" de cada spec):

| Área | Estado no Go | Spec |
|------|--------------|------|
| Providers | PARCIAL — 11 providers hard-coded; falta `providers.<p>.models/settings/headers` e `model`/`small_model` | [06](06-providers.md) |
| Permissions/Policies | PARCIAL — só por-agente; falta `permissions` (array de regras por recurso) e `policies` | [07](07-permissions.md) |
| MCP | PARCIAL — stdio/sse; falta `mcp.servers` `local/remote`, HTTP/streamable, resources/prompts, OAuth, ciclo de vida | [08](08-mcp.md) |
| TUI | PARCIAL — sem mouse, sem keybinds configuráveis, sem busca/rename/delete de sessão, sem paste | [09](09-tui.md) |
| Plugins | AUSENTE — nenhum mecanismo de plugin/hook | [10](10-plugins.md) |
| Config | PARCIAL — formato/localização divergentes; faltam muitas chaves v2 e o gerador de schema está desatualizado | [11](11-config.md) |
| opencode.ai | AUSENTE — sem share, serve/web, ACP | [12](12-opencode-ai.md) |
| Agents v2 | PARCIAL — registry/lazy OK, mas JSON em `agent` (singular) e permissions em mapa; falta a fonte única `Source`, `agents` e `permissions` array | [13](13-agents-v2.md) |

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

