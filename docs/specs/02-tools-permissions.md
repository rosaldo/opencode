# Spec 02 — Tools e permissões por agente

Fase 2. Depende de: [01-registry-loader.md](01-registry-loader.md).

## Objetivo

Permitir que cada agente declare **quais tools** pode usar (allow/deny com wildcard) e
**permissões** (ask/allow/deny), em vez das listas fixas de coder/task.

## Contexto atual

- `CoderAgentTools` (`internal/llm/agent/tools.go:14-41`) e `TaskAgentTools` (`tools.go:43-50`).
- MCP tools globais em `GetMcpTools` (`internal/llm/agent/mcp-tools.go:169-201`).
- Serviço de permissão: `internal/permission/permission.go:35-42` (interface) e `Request`
  (`permission.go:74-108`). Não há política por agente hoje; cada tool chama `Request`.
- Bash tool cria requisições de permissão a partir do comando (ver `internal/llm/tools/bash.go`).

## Design

### Filtro de tools

Novo `internal/llm/agent/tools_filter.go`:

```go
type AgentToolDeps struct {
    Permissions permission.Service
    Sessions    session.Service
    Messages    message.Service
    History     history.Service
    LspClients  map[string]*lsp.Client
}

// FullToolSet = union de CoderAgentTools + TaskAgentTools (tools locais + MCP + diagnostics)
func FullToolSet(ctx context.Context, d AgentToolDeps) []tools.BaseTool

// TaskToolSet = conjunto read-only do subagente builtin task.
func TaskToolSet(d AgentToolDeps) []tools.BaseTool

// ToolsForDefinition escolhe o conjunto base, envolve o permission.Service com a
// política do agente e remove as tools negadas.
func ToolsForDefinition(def *agentdef.Definition, deps AgentToolDeps) []tools.BaseTool
```

- **Conjunto base**: agente custom = full set; builtin `task` = read-only;
  builtins `title`/`summarizer` = vazio. Builtin `coder` = full set.
- `tools` usa **nomes de tool** (paridade com o TS): `{"write": false, "edit": false,
  "bash": false}` remove essas tools; `{"view": true}` lê-se como allow list (só ela);
  `"*"` é o default; wildcard (`mymcp_*`) casa por nome.
- Matching por nome com wildcard, "mais específico vence" (exato > prefixo literal
  mais longo > `*`). Ver desvio 2 abaixo.
- MCP tools (`GetMcpTools`) entram no conjunto e podem ser bloqueadas (`mymcp_*: false`).
- `CoderAgentTools`/`TaskAgentTools` viraram wrappers finos de `FullToolSet`/`TaskToolSet`.

### Permissões por agente

`agentdef.Definition.Permissions` (mapa) com chaves = **permission keys**, valores =
`allow|ask|deny` (ou `map[string]string`/`map[string]any` para `bash`, padrão→ação, ex.:
`{"*":"ask","git *":"allow"}`). Chaves conhecidas (paridade com o TS): `bash`, `edit`
(gate de write/edit/patch), `read` (view), `list` (ls), `webfetch` (fetch/sourcegraph),
`task` (agent), `lsp` (diagnostics); tools MCP mantêm o próprio nome `servidor_tool` como
chave. `permission.PermissionKey` faz o mapeamento tool→chave.

Integração:
- Threading do agente ativo até a checagem via **policy enforcer** que envolve o
  `permission.Service`: `permission.NewAgentScoped(base Service, policy Policy)`
  (`internal/permission/scoped.go`). As tools locais recebem o service já escopado
  (construído em `ToolsForDefinition`).
- Regras avaliadas por **especificidade** (exato > prefixo literal mais longo > `*`).
  Ver desvio 2.
- `deny` → `Request` retorna `false` sem publicar evento (equivale a "permission denied"
  já tratado em `agent.go:396-401`). `allow` → `true` sem UI. `ask` → comportamento atual
  (`permission.go:100-107`).
- `bash` com mapa: o comando casa contra os globs via `permission.MatchWildcard`
  (`*` casa também com `/`, então `git *` casa com `git -C /tmp status`). O comando é
  lido de `params` via `permission.CommandProvider`
  (`BashPermissionsParams.PermissionCommand`).

### `ReadOnly` derivado

Opcional: derivar um conjunto `read`-only (view/glob/grep/ls/sourcegraph) para agentes de
análise, expondo helpers `ReadOnlyToolNames()` para uso pela Spec 04/ícones.

## Mudanças (arquivos)

- **Novo**: `internal/llm/agent/tools_filter.go` (+ `tools_filter_test.go`).
- **Novo**: `internal/permission/policy.go` (`Policy`, `ParsePolicy`, `Decide`,
  `PermissionKey`, `MatchWildcard`) e `internal/permission/scoped.go` (`AgentScoped`)
  (+ `policy_test.go`, que também cobre o scoped service).
- `internal/llm/agent/tools.go`: `CoderAgentTools`/`TaskAgentTools` viraram wrappers.
- `internal/llm/agent/mcp-tools.go`: o cache passou a guardar só metadados (ver desvio 3).
- `internal/llm/tools/bash.go`: `BashPermissionsParams.PermissionCommand`.
- `internal/app/app.go`: o coder é construído com `ToolsForDefinition`.
- `internal/llm/agent/agent.go`: **não mudou** — `NewAgent` continua recebendo a lista
  de tools pronta.
- `internal/agentdef/definition.go`: já expõe `Tools`/`Permissions` (Spec 01).

## Implementação (fase 2)

Feito: filtro por `tools` (deny list / allow list / wildcard / `*`), política de
`permissions` por permission key (incluindo mapa de comandos do bash), service escopado
`AgentScoped`, MCP no filtro, coder/task embutidos inalterados, wiring do coder em
`app.go`.

### Desvios em relação ao texto original do spec

1. **Dois namespaces distintos** (como no TS): `tools` casa por *nome da tool*,
   `permissions` por *permission key* (`edit` cobre write/edit/patch). Por isso
   `tools: {edit: false}` **não** remove `write`/`patch` — para isso use
   `permissions: {edit: deny}` (que mantém a tool visível, mas nega a permissão) ou
   liste as três em `tools`.
2. **"Mais específico vence" no lugar de "última regra vence"**: `Definition.Tools` e
   `Permissions` são maps em Go, que não preservam ordem de inserção (o TS preserva).
   A precedência passou a ser exato > prefixo literal mais longo > `*`, que é
   determinística e resolve o caso comum do TS (`{"*":"ask","git *":"allow"}`) da mesma
   forma — e de forma menos surpreendente quando o padrão mais largo vem depois.
3. **Cache de MCP tools por metadados**: antes o cache guardava as tools já construídas,
   com o `permission.Service` preso a quem chamasse primeiro; com um service por agente
   isso atribuiria a política de um agente a todos os outros. O cache passou a guardar
   só nome/schema/config e o service é religado a cada chamada.
4. **`GetMcpTools` não panica mais sem config** (antes `config.Get().MCPServers` com
   `cfg == nil` derrubava o processo; agora retorna vazio). Isso também deixa os testes
   do pacote `agent` sem depender de config carregada.
5. **`deny` é final**: rejeita sem perguntar mesmo para sessões auto-aprovadas. Um
   `ask` continua honrando o "sempre permitir da sessão" do fluxo atual.

Limitações conhecidas: glob de bash não expande `$HOME`/variáveis (igual ao TS); a
política não é reavaliada em sessões já em andamento ao trocar de agente (a Fase 4 cria
um provider novo por troca, mas a lista de tools é montada por agente).

## Critérios de aceite

1. Agente com `tools: {write:false, edit:false, bash:false}` não recebe essas tools.
2. `tools: nil` → mesmo conjunto do coder (back-compat).
3. Wildcard (`mymcp_*: false`) remove todas as tools daquele MCP.
4. `permissions.edit: deny` faz `edit`/`write`/`patch` falharem com "permission denied".
5. `permissions.bash: {"*":"ask","git *":"allow"}` → `git status` roda sem UI; outros pedem.
6. `coder`/`task` embutidos mantêm o comportamento atual.

## Testes

- `permission`: `TestParsePolicy`, `TestPolicyDecide` (precedência, deny/allow/ask, a
  chave `edit` cobrindo write/patch, wildcard, mapa do bash lendo o comando via
  `CommandProvider`), `TestMatchWildcard`, `TestPermissionKey`, `TestNewAgentScoped`
  (deny/allow sem tocar na base, `ask` e sem match caem no fluxo atual, policy vazia
  devolve o service original).
- `llm/agent`: `TestToolEnabled`, `TestFilterTools`, `TestToolsForDefinition*` (coder
  full set, task read-only, custom herda full set, deny list, allow list, wildcard de
  MCP), `TestToolsForDefinitionPolicyDeniesWrite` e `...DeniesBash` (integração: a tool
  continua sendo oferecida ao modelo, mas retorna `permission.ErrorPermissionDenied`).

Nota sobre o critério 5: `git status` já é tratado como somente-leitura pela bash tool
(`safeReadOnlyCommands`) e nem chega a pedir permissão; a cobertura do mapa do bash está
em `TestPolicyDecide` (comandos fora da lista segura).

## Riscos

- Toque em `permission.Service` afeta todas as tools; manter a interface atual e apenas
  adicionar uma implementação escopada, minimizando diffs.
- Precisão de glob em comandos shell: `bash` é texto livre; documentar limitação (sem expansão
  de `$HOME`, igual ao TS).
