# Spec 01 — Núcleo de agentes: registro, sources e prompt lazy

Fase 1. Depende de: —. **Base de todas as demais.**

> **Nota de revisão (onda 2).** Esta spec foi reescrita para fixar o desenho como **fonte única**:
> um único tipo `Definition` e um único `Registry` são o núcleo; arquivos `.md` e o mapa `agents`
> (JSON v2) são apenas *sources* que produzem `Definition` pela mesma interface. Isso elimina a
> necessidade de aliases/adaptação (nunca "remendar": ver [13](13-agents-v2.md)). A seção
> "Implementação (fase 1)" no fim permanece como histórico do que foi entregue.

## Objetivo

Substituir o conjunto fixo de 4 agentes por um **registro dinâmico** que descobre agentes de
múltiplas *sources* (arquivos markdown globais + projeto; e, na onda 2, o mapa `agents` do JSON),
mantendo apenas metadados em memória e lendo o prompt do agente somente quando ele é ativado. A
propriedade "só o agente em uso vai para o prompt" vale para um número **não limitado** de agentes.

## Contexto atual

- Enum fixo: `internal/config/config.go` (`AgentCoder`, `AgentSummarizer`, `AgentTask`, `AgentTitle`).
- Struct de config do agente: `Agent{Model,MaxTokens,ReasoningEffort}`; mapa `Agents`.
- Resolução de prompt: `internal/llm/prompt/prompt.go` (switch por nome).
- Prompt injetado em `internal/llm/agent/agent.go` (`provider.WithSystemMessage`).
- Criação de provider: `createAgentProvider`; instanciação em `internal/app` (só `CoderAgent`).

## Design

### Princípios (não negociáveis)

1. **Fonte única.** Existe **um** tipo `agentdef.Definition` e **um** `agentdef.Registry`. Toda
   forma de declarar um agente (`.md`, `agents` do JSON) produz `Definition` — não há tipo
   intermediário nem camada de tradução.
2. **`name` é a identidade.** O caminho do arquivo/subpasta **nunca** define a identidade (igual a
   Claude Code, Codex e opencode v2). O `name` vem do campo `name`; sem ele, do nome-base do arquivo.
3. **Prompt lazy.** No boot só metadados; o corpo/prompt só é lido na primeira ativação (ou é
   fornecido inline pelo JSON).
4. **Escala ilimitada.** Nada de teto estrutural: `map` + registry dinâmico.
5. **`team` é extensão nossa.** O departamento (subpasta abaixo de `agent(s)/` ou campo `team:`)
   é uma característica exclusiva deste fork (Claude Code/Codex ignoram o path para identidade).
   Preservado, opcional, com default derivado da subpasta.
6. **Sem ciclos de import.** `agentdef` não depende de `config`/`prompt`/`llm`.

### Pacote `internal/agentdef` (package `agentdef`)

Sem dependência de `config`, `prompt` ou `llm`. Usa `gopkg.in/yaml.v3` e
`github.com/bmatcuk/doublestar/v4` (ambos já em `go.mod`).

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
    Team            string            // extensão nossa: departamento (ver Spec 03)
    Model           string            // "provider/model" ou ModelID; vazio = herda do default
    MaxTokens       int64             // 0 = default do modelo
    ReasoningEffort string            // low|medium|high
    Temperature     float64
    Tools           map[string]bool   // allow/deny; "*" wildcard; nil = herda (Spec 02)
    Permissions     []PermissionRule  // regras v2 (Spec 13; array ordenado)
    Color           string            // ex.: "#RRGGBB"

    promptPath string
    promptOnce sync.Once
    promptText string
    promptErr  error
    body       string
    hasBody    bool
}

func (d *Definition) Prompt() (string, error) // lazy; inline (JSON) ou arquivo .md
func (d *Definition) SetPrompt(text string)   // para definições inline (JSON)
func (d *Definition) SourcePath() string
```

`registry.go` — thread-safe (espelha `config.Get()`):

```go
type Registry struct { /* map + ordem + default */ }
func (r *Registry) Register(d *Definition) error
func (r *Registry) Get(name string) (*Definition, bool)
func (r *Registry) List() []*Definition
func (r *Registry) ListByMode(m Mode, includeHidden bool) []*Definition
func (r *Registry) Default() string
func (r *Registry) SetDefault(name string)

func Get() *Registry
func Set(r *Registry)
```

### Sources (arquivos `.md`) — interface única

O ponto central da fonte única: uma interface pequena, um método, dois implementadores.

```go
// source.go
type Source interface {
    Definitions() ([]*Definition, error)
}
```

- `fileSource` — varre `<dir>/{agent,agents}/**/*.md` (doublestar); lê **só o frontmatter**;
  guarda `promptPath` para leitura lazy do corpo.
- `jsonSource` (onda 2, Spec 13) — converte `agents.<n>` do config em `Definition` com
  `SetPrompt(inline)`.
- O **loader** compõe sources em ordem de precedência e registra no `Registry`:

```go
func Load(sources ...Source) (*Registry, error) // registra na ordem; última vence
```

Precedência (menor → maior): builtins < global (JSON e `.md` no mesmo nível, JSON vence) < projeto
(idem). Detalhes de precedência entre JSON e `.md` na [Spec 13](13-agents-v2.md).

### Formato dos `.md` (YAML frontmatter + corpo = prompt)

Alinhado a Claude Code e opencode v2: **corpo do markdown é o prompt** (não há campo `system` no
`.md`; `system` é só a forma inline no JSON). Campos de frontmatter:

```yaml
name: string                   # identidade; default: nome-base do arquivo
description: string            # recomendado (obrigatório p/ subagent — warn se faltar)
mode: primary | subagent | all # default: all
hidden: bool
disabled: bool
team: string                   # extensão nossa; default: subpasta abaixo de agent(s)/, senão ""
model: provider/model          # opcional
maxTokens: int
reasoningEffort: low|medium|high
temperature: float
tools: { write: false, bash: true }   # allow/deny com wildcard (Spec 02)
permissions:                          # array v2 (Spec 13); ordem importa
  - { action: shell, resource: "git push *", effect: deny }
color: "#RRGGBB"
```

- Frontmatter inválido → `logging.Warn` e **ignora o arquivo** (não aborta o boot).
- Arquivo sem frontmatter → agente com defaults (mode=all) e corpo como prompt.

### Resolução de modelo por nome (helper em `internal/llm/models`)

`func ResolveModelID(s string) (ModelID, bool)`: aceita `ModelID` puro ou `provider/model`
(casa `Model.Provider` e ID igual/sufixo `/model`).

### Integração

- `internal/config/config.go` (`Load`): montar o registry com as sources (builtins, dirs globais,
  dir do projeto, e o mapa `agents` do JSON na onda 2); `agentdef.Set(reg)`.
- `internal/llm/prompt/prompt.go`: `GetAgentPrompt(name string, provider)` — builtins no switch;
  custom via `agentdef.Get().Get(name)`; fallback `"You are a helpful assistant"`.
- `internal/llm/agent/agent.go`: `createAgentProvider(name string)` resolve modelo pela
  `Definition` (ou herda do default); `NewAgent`/`Update` aceitam `string`.
- `internal/app/app.go`: nome do agente primário (`default_agent`/`agentdef.Default()`).

## Mudanças (arquivos)

- **Núcleo**: `internal/agentdef/{definition,registry,source,loader}.go` (+ testes).
- `internal/llm/models`: `ResolveModelID`.
- `internal/config/config.go`: montagem do registry a partir das sources.
- `internal/llm/prompt/prompt.go`: assinatura + fallback ao registry.
- `internal/llm/agent/agent.go`: `createAgentProvider`/`NewAgent`/`Update` por nome.
- `internal/app/app.go`: agente primário.

## Critérios de aceite

1. Muitos `.md` válidos (sem teto) carregam no boot só metadados (nenhum corpo lido).
2. `Registry.List()` retorna todos; `Get("foo").Prompt()` lê o corpo 1x (cacheado).
3. Provider criado só para o agente ativo; `WithSystemMessage` recebe **só** o prompt dele.
4. Builtins e a config atual continuam funcionando sem alterações.
5. `name` define identidade (path não); `team` derivado da subpasta quando ausente.
6. Frontmatter inválido não derruba o boot (warn + ignora).
7. `Load(sources ...Source)` compõe sources sem duplicar tipos; sem ciclos de import.

## Testes

- `internal/agentdef`: descoberta `agent/*.md`/`agents/*.md`; nome por frontmatter e por arquivo;
  precedência entre sources; `Prompt()` lazy (remover arquivo após `Load`); frontmatter inválido.
- `internal/agentdef`: `Source` (file e, na 13, json) produzindo o mesmo `Definition`.
- `internal/llm/prompt`: builtin ainda resolve; custom resolve do registry.

## Riscos / observações

- Não mover prompts embutidos para `agentdef` (quebraria decoupling e `CoderPrompt(provider)`).
- `Model` como string exige `ResolveModelID`.
- Preservar `team`/`hidden`/`disabled`/`color` como extensões (a v2 ignora o que não conhece;
  documentar).

## Implementação (fase 1) — histórico

Feito: pacote `internal/agentdef` (`definition.go`, `registry.go`, `loader.go`, `loader_test.go`);
`Definition.Prompt()` lazy (`TestPromptIsLazy`); `SplitFrontmatter`; `**` via
`doublestar.FilepathGlob` em `<dir>/{agent,agents}`; precedência builtin < global < projeto;
frontmatter inválido só warn. `internal/llm/models/resolve.go`: `ResolveModelID`.
`internal/config/config.go`: `loadAgentDefinitions`, `agentSearchDirs`, `builtinDefinitions`.
`internal/llm/prompt/prompt.go`: `GetAgentPrompt(name, provider)` + `customAgentPrompt`.
`internal/llm/agent/agent.go`: `createAgentProvider(name)`/`NewAgent(name)`/`Update(name)`.

Desvios do design original:
- `Service.Update` passou de `AgentName` para `string` (agentes custom).
- Title/summarize criados só quando `name == "coder"` (generalizado na fase 4a).
- `Temperature`/`Permissions` parseados, não aplicados (fases 2/4).
- `Definition.Team` + frontmatter `team:` adicionados na fase 3 (delegação por departamento).
- `options` de precedência JSON (GlobalDefinitions/ProjectDefinitions) foram um passo intermediário
  (fase 5); a **onda 2 (Spec 13)** consolida isso na interface `Source` descrita acima.

> **Nota (onda 2):** a consolidação da interface `Source` e a migração de `agent`→`agents` +
> permissões array v2 são a [Spec 13](13-agents-v2.md). A fase 1 entregue permanece válida; a 13 a
> refatora para a forma final sem perda de características.
