# Spec 03 — Subagentes por nome (tool `agent`)

Fase 3. Depende de: [01](01-registry-loader.md), [02](02-tools-permissions.md).

## Objetivo

Generalizar a tool `agent` para invocar **qualquer subagente** do registry por nome (em vez do
único `task` fixo), listando subagentes elegíveis na descrição da tool — como o `Task` do TS.

## Contexto atual

- Tool `agent`: `internal/llm/agent/agent-tool.go`. `AgentParams` só tem `Prompt`
  (`agent-tool.go:25-27`); `Run` cria sempre `NewAgent(config.AgentTask, ..., TaskAgentTools(...))`
  (`agent-tool.go:57`).
- Descrição e lista de tools do subagente são **hardcoded** (`agent-tool.go:32`).
- Subagente cria sessão filha via `CreateTaskSession` (`agent-tool.go:62`).

## Design

### Parâmetros

```go
type AgentParams struct {
    Prompt    string `json:"prompt"`
    Subagent  string `json:"subagent,omitempty"`  // nome; vazio = "task"
    // Description string `json:"description,omitempty"` // opcional (paridade TS)
}
```

### Descrição dinâmica

`Info()` monta a descrição a partir de `agentdef.Get().ListByMode(ModeSubagent, includeHidden=false)`
cujo `Hidden=false`, filtrando por `permission.task` do agente **chamador** (Spec 02):
- Lista "nome — description" de cada subagente elegível.
- Se não houver subagentes elegíveis → tool indisponível (não incluir em `toolsForDefinition`).
- A tool precisa saber o agente chamador: passar o `Definition` do chamador no construtor
  (`NewAgentTool(..., caller *agentdef.Definition)`).

### Run

- `name := params.Subagent` (default `"task"`).
- `def, ok := agentdef.Get().Get(name)`; se `!ok` → erro "unknown subagent".
- Verificar `permission.task` do chamador contra `name` (deny → erro).
- Tools do subagente: `toolsForDefinition(def, deps)` (Spec 02).
- Criar: `NewAgent(name, b.sessions, b.messages, toolsForDefinition(def, deps))`.
- Manter `CreateTaskSession` e a soma de custo à sessão pai (`agent-tool.go:62-96`).
- Passar o `permission.Service` escopado ao subagente (Spec 02).

### Back-compat

- Sem `subagent` → comportamento atual (`task`).
- Garantir que `task` está no registry (builtin) — Spec 01.

### Restrições de delegação

- **Elegibilidade não depende de anúncio nem de `mode`**: qualquer agente registrado
  pode ser lançado por nome (um orquestrador pode rodar qualquer um dos seus 200+
  agentes, inclusive vários em paralelo — várias tool uses numa mensagem). `mode`
  decide quem pode *ser* agente de sessão (switcher), não quem pode ser *lançado*.
- Não são invocáveis: agents `disabled`, os helpers internos `title`/`summarizer`, o
  próprio chamador e anything já na **cadeia de delegação** (`DelegationChain` no
  `AgentToolDeps`): é isso que limita recursão (a→b→a é recusado), sem número
  arbitrário de profundidade. Os limites reais são tokens/hardware do provider.
- Agentes em modo `subagent` são folhas: não recebem a tool `agent` (não delegam de
  novo).
- Hidden continua **invocável**, mas não é anunciado na descrição.
- A **descrição anuncia por departamento** (`team`): os agentes do mesmo `team` do
  chamador entram com uma linha de descrição cada ("Your department"); os demais
  entram agrupados por departamento, só os nomes ("Other departments"). Sem número
  arbitrário: 500–1000 agentes custam só nomes no prompt, e nenhum agente fica oculto.
  Quem não tem `team` cai em "(no department)". A tool funciona mesmo sem a lista
  (ex.: roster no prompt do orquestrador).

## Implementação (fase 3)

Feito: `AgentParams.Subagent`, `Info()` com lista dinâmica de subagentes, `Run`
resolvendo o alvo e checando a política `task` do chamador, `NewAgentTool(caller, deps)`,
tool `agent` só incluída em `FullToolSet` quando há algo para delegar, e o nome do
subagente como prefixo nos params renderizados do chat
(`internal/tui/components/chat/message.go`).

`permission.Policy` foi generalizado: qualquer chave pode receber mapa de padrões
(antes só `bash`), e `Policy.Task(nome)` avalia a chave `task` contra o nome do
subagente. Assim `permissions: {task: {"*":"deny","reviewer":"allow"}}` vale tanto para
a descrição quanto para a invocação.

### Desvios em relação ao texto original do spec

1. **A tool `agent` não chama `permissions.Request`** (não abre diálogo). `deny` é
   aplicado pela `Policy.Task`; `ask` é tratado como `allow`. Sem política explícita o
   comportamento continua o de antes: nenhuma pergunta por delegação (critério 2).
2. **Recursão limitada pela cadeia, não por profundidade arbitrária**: `Run` passa ao
   filho os deps com `DelegationChain` estendida (cadeia + agente atual); o filho não
   pode lançar ninguém da cadeia. Agentes em modo `subagent` continuam sem a tool.
3. `title`/`summarizer` não são invocáveis e não recebem a tool (antes eram
   "subagentes" só por causa do `mode`).
4. **Anúncio por departamento, sem teto**: `Definition.Team` (frontmatter `team:`, ou
   a subpasta abaixo de `agent(s)/` quando o frontmatter não define) agrupa os
   agentes; a descrição da tool lista o departamento do chamador com descrições e os
   outros só por nome, agrupados. Nenhum agente fica invisível e qualquer nome
   registrado é aceito mesmo fora da lista.
5. **Tools do subagente**: `ToolsForDefinition(target, deps)` — o `deps` pode carregar o
   service já escopado ao chamador, e a política do *alvo* (mais interna) prevalece
   sobre a do chamador para as tools do alvo.
6. **Teste de integração do `Run`** (sessão filha + soma de custo) não foi escrito:
   exige harness de DB e configuração de provider LLM, que o repositório não tem em
   testes. O caminho do `Run` em si não mudou, só a forma do agente ser construído.

## Mudanças (arquivos)

- `internal/llm/agent/agent-tool.go`: `AgentParams`, `Info()`, `Run`, `NewAgentTool`.
- `internal/llm/agent/tools_filter.go`: tool `agent` conforme elegíveis + `AgentToolDeps.DelegationChain`.
- `internal/permission/policy.go`: `ParsePolicy` generalizado + `Policy.Task(nome)`.
- `internal/agentdef`: `Definition.Team` + `team:` no frontmatter (default: subpasta
  em `agent(s)/`).
- UI de chat: `internal/tui/components/chat/message.go` já tratava `AgentToolName`;
  `renderToolParams` agora prefixa o prompt com o nome do subagent quando presente.
- Correção de falha pré-existente: `internal/llm/tools/ls_test.go` carrega o config
  (paths relativos/ vazios caem no working directory, que panicava
  "config not loaded").

## Critérios de aceite

1. `agent` com `subagent:"security-auditor"` executa o subagente correto.
2. Sem `subagent` → executa `task` (inalterado).
3. Subagentes `hidden` não aparecem na descrição, mas ainda são invocáveis.
4. `permission.task: {"*":"deny","foo":"allow"}` remove os demais da descrição.
5. Nome inexistente → erro claro, sem panic.

## Testes

- Unitário: montagem da descrição (lista/omite por hidden e policy) —
  `internal/llm/agent/agent-tool_test.go`.
- Unitário: política `task` — `TestPolicyTask` em `internal/permission/policy_test.go`.
- Integração: `Run` com `subagent` cria sessão filha e soma custo — **não escrito**
  (harness de DB/provider; ver "Desvios" acima).

## Riscos

- Descrições longas inflam a tool description; truncar (ex.: 1 linha por subagente).
- Concorrência: múltiplos subagentes em paralelo já são suportados (sessões independentes).
