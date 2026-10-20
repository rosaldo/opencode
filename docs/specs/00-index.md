# Specs — Agentes dinâmicos no opencode (Go)

Índice e âncora de retomada. **Leia este arquivo primeiro ao retomar o trabalho.**

## Objetivo

Permitir carregar **um número não limitado de agentes instalados** (definidos em arquivos e/ou no
mapa `agents` do config), seguindo o modelo do opencode em TypeScript, garantindo que **apenas o
prompt do agente ativo seja enviado ao modelo** (nunca todos juntos). Sem teto estrutural.

Base: fork Go `github.com/opencode-ai/opencode`, HEAD `73ee493` (= tag `v0.0.55` + 4 commits).
Hoje existem **4 agentes fixos** (`coder`, `summarizer`, `task`, `title`) — enum em
`internal/config/config.go:39-44`.

> **Onda 2** (fases 6–12, `Planejado`): paridade com o opencode TS **v2** nas áreas que não são
> agentes — providers, permissions/policies, MCP, TUI, plugins, config e integração opencode.ai.
> Índice em [06-12-roadmap.md](06-12-roadmap.md). Alvo é a **v2** (não a 1.19.3); convenções de
> schema v2 estão no fim do roadmap.

## Decisões de arquitetura (fixadas)

1. **Registro vs. instanciação** (igual ao TS): carregar só *metadados* de todos os agentes;
   construir `provider.Provider` **sob demanda** apenas para o agente ativo (e cachear).
2. **Prompt lazy**: no boot lê-se apenas o *frontmatter* dos `.md`; o corpo (prompt) só é lido
   na primeira ativação do agente. Um só prompt entra no contexto, via
   `provider.WithSystemMessage` em `internal/llm/agent/agent.go:731`.
3. **Novo pacote `internal/agentdef`** (pacote `agentdef`): dados + registry + loader, sem
   depender de `config`/`prompt`/`llm`. Direção de dependências:
   `config → agentdef`, `prompt → config + agentdef`, `llm/agent → config + prompt + agentdef`.
   Isso evita ciclos de import.
4. **Builtins continuam no switch** de `prompt.GetAgentPrompt` (prompts variam por provider,
   ex.: `CoderPrompt(provider)`); o registry apenas os registra com `Builtin=true` e o
   `GetAgentPrompt` decide builtin vs. custom.
5. **Back-compat total**: configs existentes e os 4 agentes seguem funcionando sem mudanças.

## Fases

| # | Spec | Escopo | Status | Depende de |
|---|------|--------|--------|-----------|
| 1 | [01-registry-loader.md](01-registry-loader.md) | Núcleo: `agentdef` (`Definition`/`Registry`/`Source`), loader de `.md`, prompt lazy, integração no `createAgentProvider` | Verificado (revisado p/ fonte única) | — |
| 2 | [02-tools-permissions.md](02-tools-permissions.md) | Tools e permissões por agente (`ToolsForDefinition`) | Verificado | 01 |
| 3 | [03-subagents.md](03-subagents.md) | Subagentes por nome na tool `agent` | Verificado | 01, 02 |
| 4 | [04-tui-switcher.md](04-tui-switcher.md) | Switcher (Tab) + `@`-mention, `AgentManager` | Verificado (4a + 4b) | 01, 02, 03 |
| 5 | [05-config-compat.md](05-config-compat.md) | Config JSON (`agent`, `defaultAgent`), schema, validação | Verificado | 01 |

### Onda 2 — paridade com o opencode TS (v2)

Índice da onda: [06-12-roadmap.md](06-12-roadmap.md). Áreas que **não** são agentes. **Alvo: a
geração v2** (`https://opencode.ai/config.json`), não a 1.19.3.

| # | Spec | Escopo | Status | Depende de |
|---|------|--------|--------|-----------|
| 6 | [06-providers.md](06-providers.md) | Providers v2: `providers.<p>.models/settings` (baseURL/headers), `model`/`small_model`, catálogo | Planejado | 11 |
| 7 | [07-permissions.md](07-permissions.md) | `permissions` (array por recurso) + `policies`, `external_directory`, gating ampliado | Planejado | 11 |
| 8 | [08-mcp.md](08-mcp.md) | `mcp.servers` local/remote, HTTP/streamable, resources/prompts, OAuth, ciclo de vida | Planejado | 11 |
| 9 | [09-tui.md](09-tui.md) | TUI v2: keybinds (`tui.json`/`cli.json`), mouse/scroll, sessões, paste | Planejado | 11, 07 |
| 10 | [10-plugins.md](10-plugins.md) | `plugins`: loader, protocolo (stdio), hooks, custom tools, UI | Planejado | 11, 08 |
| 11 | [11-config.md](11-config.md) | **Config v2 (base)**: `opencode.json(c)`, `$schema`, `{env:}`/`{file:}`, chaves v2, schema gerador | Verificado | — |
| 12 | [12-opencode-ai.md](12-opencode-ai.md) | serve/share/web, ACP | Planejado | 06, 11, 09 |
| 13 | [13-agents-v2.md](13-agents-v2.md) | **Agents v2**: fonte única (`Source`), mapa `agents`, `default_agent`, `system` inline, permissões array v2 | Verificado | 01, 11 |

Ordem sugerida: **11 → 13 → 06 → 07 → 08 → 09 → 10 → 12** (ver roadmap). A 13 consolida a
costura JSON↔agentdef deixada pela fase 5 (elimina a duplicação `Options.GlobalDefinitions/
ProjectDefinitions`) e migra `permissions` para o array v2.

Status possíveis: `Planejado` → `Em andamento` → `Implementado` → `Verificado`.

## Notas de execução

- **Verificação (fase 1)**: `go build ./...` OK; `go test` OK em `internal/agentdef`,
  `internal/llm/models`, `internal/llm/prompt`, `internal/config`, `internal/llm/agent`.
- **Verificação (fase 2)**: `go build ./...` OK; `go test ./...` OK em todos os pacotes
  exceto a falha pré-existente abaixo. Novos testes: `internal/permission`
  (`policy_test.go`) e `internal/llm/agent` (`tools_filter_test.go`). Desvios e
  limitações da fase 2 estão documentados em `docs/specs/02-tools-permissions.md`
  (seção "Implementação (fase 2)").
- **Verificação (fase 3)**: `go build ./...` OK; `go test ./...` OK em **todos** os
  pacotes (a falha pré-existente de `TestLsTool_Run` foi corrigida: o teste agora
  carrega o config, que os paths relativos exigem). Novos testes:
  `internal/llm/agent/agent-tool_test.go` (elegibilidade, cadeia de delegação,
  lista/descrição, `subagentByName`, tool ausente quando não há alvos) e
  `TestPolicyTask` em `internal/permission/policy_test.go`. UI de chat exibe o
  subagente nos params da tool. Decisões de design e desvios documentados em
  `docs/specs/03-subagents.md` (seções "Restrições de delegação" e
  "Implementação (fase 3)"): elegibilidade não depende de `mode` nem de anúncio
  (um orquestrador lança qualquer agente registrado, em paralelo); recursão é
  limitada pela cadeia de delegação (`DelegationChain`), não por profundidade
  arbitária; a descrição anuncia por **departamento** (`Definition.Team`, do
  frontmatter `team:` ou da subpasta em `agent(s)/`): o departamento do chamador
  com descrições, os outros só por nome agrupado; sem diálogo de permissão na
  delegação (`ask` = allow); e sem teste de integração do `Run` (sem harness de
  DB/provider nos testes).
- **Verificação (fase 4a)**: `go build ./...` OK; `go test ./...` OK (flake pré-existente
  e intermitente em `TestGetContextFromPaths` — ordem concorrente em
  `processContextPaths`, upstream; não relacionado). Novos testes:
  `internal/app/agent_manager_test.go` (`Switch` valida primário/all vs.
  subagent/hidden/disabled/desconhecido; switch não instancia; cache devolve a
  mesma instância) e `internal/tui/components/dialog/agents_test.go` (filtro
  fuzzy por nome/descrição, case-insensitive, ranking, reset de seleção).
  Entregue: `AgentManager` (lazy + fan-in de eventos), diálogo de agentes
  (Tab/Ctrl+A, busca fuzzy), status bar `agente · modelo`, model dialog
  aplicado ao agente ativo, title/summarize generalizados para primários.
  Pendente (4b): `@`-mention de subagentes. Desvios documentados em
  `docs/specs/04-tui-switcher.md` (seção "Implementação (fase 4a)").
- **Verificação (fase 4b)**: `go build ./...` OK; `go test ./...` OK. Novo teste:
  `internal/tui/components/chat/mention_test.go` (`splitMention`: leading, meio de
  frase, só menção, nome com ponto/underscore, sem menção, e-mail não casando,
  primeira menção vence). Entregue: `@nome` na mensagem valida contra o registry
  (`SubagentLookup`), roda o agente numa **sessão filha** (`AgentManager.RunSubagent`
  via `CreateTaskSession`) e abre a sessão filha para exibir a saída; envio normal e
  `Ctrl+E` cobertos; menção desconhecida é enviada como prosa. Autocomplete textual
  do `@` **não** implementado (o `@` já é gatilho do diálogo de arquivos; fusão de
  completions ficou fora do escopo) — documentado no spec 04.
## Mapa de arquivos (existentes que serão tocados)

- `internal/config/config.go` — enum/struct `Agent`, `Agents`, `Validate`, `Load`. (Specs 1, 5)
- `internal/llm/prompt/prompt.go` — `GetAgentPrompt`. (Spec 1)
- `internal/llm/agent/agent.go` — `NewAgent`, `createAgentProvider`, `Update`. (Specs 1, 4)
- `internal/llm/agent/tools.go` — `CoderAgentTools`, `TaskAgentTools`. (Spec 2)
- `internal/llm/agent/agent-tool.go` — tool `agent`. (Spec 3)
- `internal/llm/agent/mcp-tools.go` — `GetMcpTools` (cache por metadados; tools MCP entram no filtro). (Spec 2)
- `internal/permission/permission.go` — interface `Service`. (Spec 2)
- `internal/app/app.go` — wiring, `Agents *AgentManager`. (Specs 1, 4)
- `internal/tui/tui.go`, `internal/tui/components/core/status.go`,
  `internal/tui/components/dialog/models.go`. (Spec 4)
- `opencode-schema.json`, `.opencode.json` — schema de config. (Spec 5)

## Arquivos novos

- `internal/agentdef/definition.go`, `registry.go`, `loader.go`, `loader_test.go`.
- `internal/llm/models/resolve.go`, `resolve_test.go`. (Spec 1)
- `internal/llm/agent/tools_filter.go` (+ `tools_filter_test.go`). (Spec 2)
- `internal/llm/agent/agent-tool_test.go`. (Spec 3)
- `internal/permission/policy.go`, `scoped.go` (+ `policy_test.go`). (Spec 2)
- `internal/tui/components/dialog/agents.go`. (Spec 4)
- `internal/app/agent_manager.go`. (Spec 4)
- `docs/specs/0X-*.md`, `AGENTS.md`.

## Protocolo de retomada

1. Abrir este índice; ver a coluna **Status**.
2. Abrir o spec da fase em andamento (seção "Mudanças" e "Critérios de aceite").
3. Ao concluir: rodar `go build ./...` e `go test ./...`, atualizar o Status aqui.
4. Não marcar `Verificado` sem testes passando (ver "Testes" de cada spec).
