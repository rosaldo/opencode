# Spec 01 — Registry de agentes + loader + prompt lazy

Fase 1. Depende de: —. Base para todas as demais.

## Objetivo

Substituir o conjunto fixo de 4 agentes por um **registro dinâmico** que descobre agentes em
arquivos markdown (global + projeto), mantendo apenas metadados em memória e lendo o prompt do
agente somente quando ele é ativado. A propriedade "só o agente em uso vai para o prompt" passa
a valer para N agentes.

## Contexto atual

- Enum fixo: `internal/config/config.go:39-44` (`AgentCoder`, `AgentSummarizer`, `AgentTask`, `AgentTitle`).
- Struct de config do agente: `internal/config/config.go:46-51` (`Agent{Model,MaxTokens,ReasoningEffort}`); mapa em `config.go:90`.
- Resolução de prompt: `internal/llm/prompt/prompt.go:15-39` (switch por nome).
- Prompt é injetado no provider em `internal/llm/agent/agent.go:731` (`provider.WithSystemMessage(prompt.GetAgentPrompt(...))`).
- Criação de provider: `internal/llm/agent/agent.go:706-758` (`createAgentProvider`); usa `cfg.Agents[agentName]`.
- Instanciação: `internal/app/app.go:63-74` (só `CoderAgent`).

## Design

### Novo pacote `internal/agentdef` (package `agentdef`)

Sem dependência de `config`, `prompt` ou `llm` (evita ciclos). Usa `gopkg.in/yaml.v3`
(já em `go.mod`) e `github.com/bmatcuk/doublestar/v4` (já em `go.mod`) para glob.

`definition.go`:

```go
type Mode string
const (
    ModePrimary  Mode = "primary"
    ModeSubagent Mode = "subagent"
    ModeAll      Mode = "all" // default
)

type Definition struct {
    Name            string
    Description     string
    Mode            Mode
    Hidden          bool
    Disabled        bool
    Builtin         bool
    Model           string            // "provider/model" ou ModelID; vazio = herda do coder
    MaxTokens       int64             // 0 = default do modelo
    ReasoningEffort string            // low|medium|high
    Tools           map[string]bool   // allow/deny; "*" wildcard; nil = herda
    Permissions     map[string]any    // reservado (Spec 02)
    Color           string            // "#RRGGBB" (reservado)

    promptPath string
    promptOnce sync.Once
    promptText string
    promptErr  error
}

// Prompt lê o corpo do .md de forma lazy (1x). Para builtin, retorna "" (resolvido no switch).
func (d *Definition) Prompt() (string, error)
func (d *Definition) SourcePath() string
```

`registry.go` — singleton thread-safe (espelha o padrão de `config.Get()`):

```go
type Registry struct { mu sync.RWMutex; defs map[string]*Definition; order []string; def string }
func (r *Registry) Register(d *Definition) error // erro se Name vazio ou duplicado
func (r *Registry) Get(name string) (*Definition, bool)
func (r *Registry) List() []*Definition                    // ordem de registro
func (r *Registry) ListByMode(m Mode, includeHidden bool) []*Definition
func (r *Registry) Default() string
func (r *Registry) SetDefault(name string)

func Get() *Registry             // singleton
func Set(r *Registry)            // para testes
```

`loader.go`:

```go
type Options struct {
    GlobalDirs  []string // ex.: ~/.config/opencode, $XDG_CONFIG_HOME/opencode, ~
    ProjectDir  string   // ex.: <workingDir>/.opencode
    Builtins    []*Definition // injetados pelo chamador (ver Integração)
}

func Load(opts Options) (*Registry, error)
```

- Glob (doublestar): `<dir>/agent/**/*.md` e `<dir>/agents/**/*.md`.
  (Constante `sourceDirs = {"agent","agents"}`, igual ao TS.)
- Para cada arquivo: ler, separar frontmatter (`---` ... `---`) e corpo.
  - **No `Load` só o frontmatter é processado**; o corpo fica no arquivo e é lido via
    `Definition.Prompt()` (lazy). `promptPath` guarda o caminho absoluto.
- Nome do agente = `frontmatter.name` se presente, senão `filepath.Base` sem `.md`.
- Precedência (menor → maior): builtin < global < projeto. Compartilham a chave `Name`.
- Frontmatter YAML aceito:

```yaml
description: string            # recomendado; obrigatório p/ subagent (validar com warn)
mode: primary | subagent | all # default: all
hidden: bool
disabled: bool
team: string                   # departamento p/ anúncio de delegação (Spec 03);
                               # default: a subpasta abaixo de agent(s)/, senão ""
model: provider/model          # opcional
maxTokens: int
reasoningEffort: low|medium|high
tools:                         # allow/deny com wildcard
  write: false
  bash: true
permissions:                   # ver Spec 02
  edit: deny
```

- Frontmatter inválido → `logging.Warn` e **ignora o arquivo** (não aborta o boot).
- Arquivo sem frontmatter → agente com defaults (mode=all) e corpo como prompt.

### Resolução de modelo por nome (helper em `internal/llm/models`)

Adicionar `func ResolveModelID(s string) (ModelID, bool)`:
- Se `s` casa com uma `ModelID` existente em `SupportedModels` → retorna.
- Se formato `provider/model` → procura ModelID cujo `models.Model.Provider` bate e cujo
  ID termina com `/model` ou é igual a `model`. (Necessário pois `Definition.Model` é string.)

### Integração

- `internal/config/config.go` (`Load`, ~128): após validar, montar o registry:
  - builtins = `builtinDefinitions()` (novo, retorna as 4 defs com `Builtin=true` e defaults:
    `coder`=primary, `task`/`title`/`summarizer`=subagent+hidden).
  - `globalDirs` derivados dos mesmos paths de `configureViper` (`config.go:222-224`).
  - `agentdef.Load(...)`; em erro → `logging.Warn` e segue com só builtins.
  - `agentdef.Set(reg)`.
- `internal/llm/prompt/prompt.go:15` — generalizar a assinatura para
  `GetAgentPrompt(name string, provider models.ModelProvider) string`:
  1. Se `name` é um builtin (`coder|title|task|summarizer`) → switch atual.
  2. Senão, `d, ok := agentdef.Get().Get(name)`; se ok → `d.Prompt()` (lazy);
     se vazio/erro → `"You are a helpful assistant"`.
  - Manter a injeção de contexto de projeto (`prompt.go:30-37`) apenas para coder/task.
- `internal/llm/agent/agent.go`:
  - `createAgentProvider(agentName config.AgentName)` → `createAgentProvider(name string)`,
    resolvendo modelo via `agentdef` (def.Model) e caindo no coder configurado quando vazio.
  - `NewAgent(agentName config.AgentName, ...)` → aceitar `name string`; `title/summarize`
    providers só quando `name` é o agente primário padrão (hoje `== config.AgentCoder`).
  - `Update` (`agent.go:516`) — aceitar nome string; manter comportamento do coder.
- `internal/app/app.go:63` — passa o nome do agente primário (**default**: `config.AgentCoder`).

## Mudanças (arquivos)

- **Novos**: `internal/agentdef/definition.go`, `registry.go`, `loader.go`, `loader_test.go`.
- `internal/llm/models/models.go` (ou arquivo existente do pacote): `ResolveModelID`.
- `internal/config/config.go`: `builtinDefinitions`, chamada de `agentdef.Load` no `Load`.
- `internal/llm/prompt/prompt.go`: assinatura + fallback ao registry.
- `internal/llm/agent/agent.go`: `createAgentProvider`, `NewAgent`, `Update` (string).
- `internal/app/app.go`: nome do agente.

## Critérios de aceite

1. Com 1000 `.md` válidos em `~/.config/opencode/agent/`, o boot carrega só metadados
   (tempo e memória desprezíveis; nenhum corpo lido ainda).
2. `agentdef.Get().List()` retorna todos; `Get("foo").Prompt()` lê o corpo 1x (cacheado).
3. O provider é criado apenas para o agente ativo; `WithSystemMessage` recebe **somente** o
   prompt desse agente.
4. Os 4 builtins e a config atual continuam funcionando sem alterações.
5. Frontmatter inválido não derruba o boot (warn + ignora).
6. Sem ciclos de import (`go build ./...` limpo).

## Testes

- `internal/agentdef/loader_test.go`:
  - descobre `agent/*.md` e `agents/*.md`; nome por frontmatter e por arquivo.
  - precedência builtin < global < projeto.
  - `Prompt()` lazy: corpo só lido após chamada (usar contador/`os.Stat`).
  - frontmatter inválido → arquivo ignorado, sem erro fatal.
- `internal/llm/prompt`: builtin ainda resolve; custom resolve do registry.
- Manual: `go build ./... && go test ./...`.

## Riscos / observações

- Não mover os prompts embutidos para `agentdef` (quebraria decoupling e `CoderPrompt(provider)`).
- Vazamento de memória: manter só frontmatter + caminho; corpo sob demanda.
- `Model` como string exige `ResolveModelID`; cobrir no teste.

## Implementação (fase 1)

Feito:
- Novo pacote `internal/agentdef`: `definition.go`, `registry.go`, `loader.go`, `loader_test.go`.
  - `Definition.Prompt()` lazy (lê o `.md` só na 1ª chamada; teste `TestPromptIsLazy` prova
    removendo o arquivo após o `Load`).
  - `SplitFrontmatter` exportado; `**` via `doublestar.FilepathGlob` em `<dir>/{agent,agents}`.
  - precedência builtin < global < projeto; frontmatter inválido só gera warn.
- `internal/llm/models/resolve.go`: `ResolveModelID` (aceita `id` puro ou `provider/model`).
- `internal/config/config.go`: `loadAgentDefinitions`, `agentSearchDirs`, `builtinDefinitions`;
  chamada ao fim de `Load`. Dirs globais: `~/.opencode` e `~/.config/opencode` (ou
  `$XDG_CONFIG_HOME/opencode`); projeto: `<workingDir>/.opencode`.
- `internal/llm/prompt/prompt.go`: `GetAgentPrompt(name string, provider)` + `customAgentPrompt`
  (registry/lazy; builtins seguem no switch).
- `internal/llm/agent/agent.go`: `createAgentProvider(name string)` resolve modelo pela
  `Definition` (ou herda do coder), `NewAgent(name string, ...)`, `Update(name string, ...)`;
  interface `Service.Update` agora usa `string`.
- `internal/app/app.go` e `internal/llm/agent/agent-tool.go` e `internal/tui/tui.go`:
  conversão para `string(config.AgentX)`.

Desvios do design:
- `Service.Update` mudou de `AgentName` para `string` (necessário p/ agentes custom);
  `config.UpdateAgentModel` continua recebendo `AgentName` (conversão no call site).
- Title/summarize providers ainda são criados só quando `name == "coder"` (Fase 4 generaliza).
- `Temperature` e `Permissions` são parseados mas ainda não aplicados (Fases 2/4).
- `Definition.Team` + frontmatter `team:` foram adicionados depois (fase 3, p/ o anúncio
  de delegação por departamento na Spec 03); default: a subpasta abaixo de `agent(s)/`
  (`agent/security/audit.md` → team `security`). Ver `TestTeamComesFromDirectoryOrFrontmatter`.

Pendência: nenhuma. Verificado com `go build ./...` e `go test` (ver notas no índice).

