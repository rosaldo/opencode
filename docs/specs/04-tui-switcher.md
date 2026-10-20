# Spec 04 — Switcher de agentes na TUI + `@`-mention + `AgentManager`

Fase 4. Depende de: [01](01-registry-loader.md), [02](02-tools-permissions.md), [03](03-subagents.md).

## Objetivo

Permitir **trocar de agente primário** (Tab) e **mencionar subagentes** (`@nome`) na TUI, com
busca fuzzy sobre o registry — sem carregar prompts de agentes inativos.

## Contexto atual

- App tem um único agente: `App.CoderAgent` (`internal/app/app.go:25-40,63-74`).
- Troca de modelo: `dialog.ModelSelectedMsg` em `internal/tui/tui.go:359-367` →
  `CoderAgent.Update(config.AgentCoder, ...)`.
- Diálogo de modelos: `internal/tui/components/dialog/models.go` (padrão de lista/fuzzy).
- Status bar mostra o modelo do coder: `internal/tui/components/core/status.go:121,269-...`.
- Turnos feitos por `a.app.CoderAgent.Run(...)` na página de chat.

## Design

### `AgentManager` (novo: `internal/app/agent_manager.go`)

```go
type AgentManager struct {
    mu      sync.RWMutex
    active  string                      // nome do agente primário ativo
    cache   map[string]agent.Service    // provider instanciado sob demanda
    deps    AgentDeps                   // sessions, messages, history, permissions, lsp
}
func (m *AgentManager) Active() string
func (m *AgentManager) ActiveAgent() agent.Service   // cria/cacheia sob demanda
func (m *AgentManager) Switch(name string) error     // valida mode!=subagent && !hidden
func (m *AgentManager) Run(ctx, sessionID, content, ...) (<-chan AgentEvent, error)
func (m *AgentManager) Update(name string, model models.ModelID) (models.Model, error)
```

- `App.CoderAgent` (`app.go:31`) vira `App.Agents *AgentManager`; manter um getter
  `App.CoderAgent` por compatibilidade se necessário, ou atualizar os call sites.
- Instanciação lazy: só o agente ativo (e subagentes efetivamente usados) geram `provider`.
- Default: `agentdef.Get().Default()` (ou `coder`).

### Diálogo de agentes (novo: `internal/tui/components/dialog/agents.go`)

- Segue o padrão de `dialog/models.go`; lista `agentdef.Get().ListByMode(ModePrimary, false)`
  (exclui `hidden`), com **busca fuzzy** via `github.com/lithammer/fuzzysearch`
  (já em `go.mod`) sobre `name + description`.
- Mensagens: `ShowAgentsDialogMsg`, `CloseAgentsDialogMsg`, `AgentSelectedMsg{ Name }`.
- Se houver > 1 primário, abrir no **Tab**; senão no keybind `switch_agent` (ex.: `Ctrl+A`).

### Wiring na TUI (`internal/tui/tui.go`)

- Tratar `AgentSelectedMsg`: `a.app.Agents.Switch(msg.Name)` + `util.ReportInfo`.
- Tecla **Tab** (fora de input de texto) abre o diálogo (ou cicla se poucos agentes).
- Status bar: incluir o nome do agente ativo em `status.go` (perto de `model()`,
  `status.go:269`), lendo `a.app.Agents.Active()`.

### `@`-mention (subagentes)

- Na página de chat (`internal/tui/page/chat.go`), detectar `@<nome>` no texto enviado.
- Se `<nome>` for um subagente do registry (`ModeSubagent`, `!Disabled`), roteá-lo: invocar
  como sessão filha (reusar caminho da tool `agent`, Spec 03) e marcar a sessão filha.
- Autocomplete opcional do `@` (lista filtrada por fuzzy). Se ficar grande, dividir em
  sub-fase (4b).

## Mudanças (arquivos)

- **Novo**: `internal/app/agent_manager.go`, `internal/tui/components/dialog/agents.go`.
- `internal/app/app.go`: substituir `CoderAgent` por `Agents *AgentManager`; ajustar `New`.
- `internal/tui/tui.go`: mensagens de agente, tecla Tab, uso de `Agents.Run`.
- `internal/tui/components/core/status.go`: mostrar agente ativo.
- `internal/tui/page/chat.go`: `@`-mention + envio via `AgentManager`.
- `internal/tui/components/dialog/models.go`: ao trocar modelo, aplicar ao agente **ativo**
  (hoje força `AgentCoder`: linhas 285, 327).

## Critérios de aceite

1. Tab abre lista de primários; escolher troca o agente ativo e a próxima mensagem usa o novo.
2. Busca fuzzy filtra por nome/descrição.
3. Status bar mostra o agente ativo.
4. Trocar modelo afeta o agente ativo (não sempre o coder).
5. `@subagent ...` executa o subagente e cria sessão filha navegável.
6. Nenhum prompt de agente inativo é carregado durante a navegação (verificar por log).

## Testes

- Unitário: `AgentManager.Switch` (rejeita subagent/hidden; cacheia).
- Unitário do filtro fuzzy do diálogo.
- Manual/TUI: Tab, troca de modelo, `@mention`.

## Riscos

- Diff amplo na TUI; fazer em sub-fases (4a: manager+switcher+status; 4b: `@`-mention).
- Sessões por agente: decidir se trocar de agente reinicia a sessão ou continua (paridade TS =
  continua a sessão; só o system prompt muda no próximo turno). Documentar a escolha.

## Implementação (fase 4a)

Entregue e verificado (`go build ./...` + `go test ./...`). Sub-fase 4b (`@`-mention)
pendente.

### `AgentManager` (`internal/app/agent_manager.go`)

- `App.CoderAgent agent.Service` virou `App.Agents *AgentManager`; os ~12 call sites
  (`tui.go`, `page/chat.go`, `chat/editor.go`, `chat/list.go`, `cmd/root.go`,
  `RunNonInteractive`) agora usam `app.Agents.<método>`.
- O manager **implementa `agent.Service`** delegando ao agente ativo (`Run`, `Model`,
  `Summarize`, `Update`, `Subscribe`), o que tornou a migração dos call sites mecânica.
- **Fan-in de eventos**: o manager tem um `pubsub.Broker` próprio; cada agente criado
  ganha uma goroutine que republica seus eventos no broker do manager, então
  `cmd/root.go` continua com uma única assinatura (`app.Agents.Subscribe`) e recebe
  eventos de qualquer agente.
- `IsBusy`/`IsSessionBusy`/`Cancel` consideram **todos** os agentes instanciados, não só
  o ativo: trocar de agente no meio de um turno nunca permite uma segunda geração
  concorrente na mesma sessão.
- Instantiation: o default (`agentdef.Get().Default()`) é criado eager no boot (erros de
  provider aparecem cedo); todos os demais são lazy sob `get()` com single-flight.
- **Continuidade de sessão** (decisão, paridade TS): trocar de agente **não** reinicia a
  sessão — a próxima mensagem usa o histórico existente com o system prompt do novo
  agente.
- `Shutdown()` (chamado por `App.Shutdown`) espera as goroutines de fan-in e derruba o
  broker.

### Title/summarize generalizados (`internal/llm/agent/agent.go`)

- `NewAgent` cria os providers de title/summarize para **qualquer agente primário**
  (`def.Mode != ModeSubagent`; nome ausente do registry conta como primário), não só
  para `coder`. Consequência aceita: uma chamada de title por sessão filha nova de
  subagentes `ModeAll` invocados via tool `agent` (também dá título útil à sessão filha).

### TUI

- Keybinding: **Tab** e **Ctrl+A** (`keys.SwitchAgent`) abrem o diálogo na página de
  chat quando nenhum outro overlay está ativo. Tab está livre (nada no editor usa Tab).
- `dialog/agents.go`: lista `ListByMode(ModePrimary, false)`, busca fuzzy via
  `fuzzy.RankFindFold` sobre `"name description"` (subsequência, case-insensitive),
  ordenada por distância; input de texto no topo; `●` marca o agente ativo; recarrega
  o registry a cada abertura (`Init`).
- Mensagens: `AgentSelectedMsg` → `Agents.Switch` + `util.ReportInfo`;
  `CloseAgentsDialogMsg` fecha. `Ctrl+Q` também fecha (padrão dos demais overlays).
- Status bar (`core/status.go`): widget `agente · modelo`. O nome vem de um getter
  injetado (`app.Agents.Active` — evita ciclo de import `core → app`); o model usa o
  novo **`config.AgentModelID(name)`** (cfg → `def.Model` → fallback coder).
- Model dialog (`dialog/models.go`): recebe `activeAgent func() string`; `Init()` é
  re-executado ao abrir para refletir o agente ativo; `ModelSelectedMsg` chama
  `Agents.Update("", modelID)` ("" = ativo) e informa agente + modelo no status.
- `config.AgentModelID` (novo, em `internal/config/config.go`): resolve o model
  efetivo de um agente por nome (entry do cfg > `def.Model` > coder), usado pela
  status bar e pelo model dialog.

### Testes

- `internal/app/agent_manager_test.go`: `Switch` aceita `primary`/`all`; rejeita
  subagent, hidden, disabled e desconhecido; `Switch` não instancia (cache vazio);
  `get` devolve a instância cacheada (stub). Instantiação real não é testada (exige
  provider configurado; sem harness, mesma limitação da fase 3).
- `internal/tui/components/dialog/agents_test.go`: filtro vazio mantém todos; filtro
  por nome; por descrição; case-insensitive; ranking (match de nome antes); sem
  match zera a lista; seleção recalculada ao estreitar o filtro.

## Implementação (fase 4b)

Entregue e verificado (`go build ./...` + `go test ./...`).

### `@`-mention de subagentes

- **Detecção** (`internal/tui/components/chat/chat.go`): `splitMention(text)` extrai o
  primeiro token `@nome` (`mentionPattern`, charset `[A-Za-z0-9._-]`, precedido por
  início ou espaço — e-mails como `a@b.com` não casam) e devolve o texto limpo
  (menção removida). Se a mensagem for só a menção, o texto limpo mantém `@nome`.
- **Validação** (`editor.go`): `editorCmp.mention` só roteia quando o nome existe no
  registry e não está disabled (`App.Agents.SubagentLookup`); menção desconhecida
  permanece no texto e é enviada como prosa (não é engolida silenciosamente).
  Aplica-se ao envio normal e ao `Ctrl+E` (editor externo).
- **Roteamento** (`internal/app/agent_manager.go`): `RunSubagent(ctx, parentID, name,
  prompt)` cria uma **sessão filha** via `CreateTaskSession` (com `parent_session_id`
  apontando para a sessão atual) e roda o agente nomeado ali — a sessão fica navegável
  pelo vínculo de parentesco. `SubagentLookup` centraliza a validação.
- **UI** (`internal/tui/page/chat.go`): `chat.SendMsg` ganhou `MentionedAgent`;
  `sendMessage` roteia para `RunSubagent` quando presente, reporta
  `Running @nome in a sub-session` e **abre a sessão filha** (`chat.SessionSelectedMsg`)
  para que a saída apareça. Os eventos da sessão filha chegam pelo fan-in do manager e
  são renderizados por `messagesCmp` (recarrega via `SetSession`).
- **Autocomplete textual do `@`**: **não** implementado nesta sub-fase. O `@` já é o
  gatilho do diálogo de arquivos/pastas existente (`keyMap.ShowCompletionDialog` na
  página de chat); sobrepor uma segunda lista no mesmo caractere exigiria uma fusão de
  completions fora do escopo. A menção funciona por digitação direta e a validação
  contra o registry. Fica como melhoria futura.

### Testes

- `internal/tui/components/chat/mention_test.go`: `splitMention` (leading, meio de
  frase, só menção, nome com ponto/underscore, sem menção, e-mail não é menção,
  primeira de duas vence). `RunSubagent` não tem teste de integração (cria sessão no
  DB e exige provider configurado — mesma limitação das fases 3/4a).
