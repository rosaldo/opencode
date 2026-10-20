# Spec 05 — Config JSON, schema e validação/back-compat

Fase 5. Depende de: [01](01-registry-loader.md).

## Objetivo

Permitir declarar agentes também no `opencode.json` (além dos `.md`), escolher o agente
**default** e validar tudo sem quebrar configs existentes.

## Contexto atual

- Config: `internal/config/config.go` — struct `Config` (`config.go:84-97`), `Agents map[AgentName]Agent`
  (`config.go:90`), override do title (`config.go:210-214`), `Validate` (`config.go:609-641`),
  `validateAgent` (`config.go:475-606`), `UpdateAgentModel` (`config.go:879-915`).
- Viper lê `~/.opencode.json` e `<wd>/.opencode.json` (`config.go:219-227,451-461`).
- Schema atual: `opencode-schema.json` (chaves `agent`, `agents`, `definitions`); exemplo
  `./.opencode.json`.

## Design

### Chaves novas

```jsonc
{
  "agent": {                       // NOVO: definições (paridade TS)
    "security-auditor": {
      "description": "Auditoria de segurança",
      "mode": "subagent",
      "model": "anthropic/claude-sonnet-4",
      "prompt": "{file:./prompts/security.txt}",  // ou "inline..."
      "tools": { "edit": false },
      "permissions": { "edit": "deny" }
    }
  },
  "defaultAgent": "coder",         // NOVO: agente primário padrão
  "agents": {                      // EXISTENTE: overrides de modelo por builtin (mantido)
    "coder": { "model": "...", "maxTokens": 8000 }
  }
}
```

- `Config` ganha: `Agent map[string]AgentDefinition` (json `agent`) e
  `DefaultAgent string` (json `defaultAgent`).
- `AgentDefinition` (config-facing) espelha os campos do frontmatter do Spec 01
  (`description`, `mode`, `hidden`, `disabled`, `model`, `maxTokens`, `reasoningEffort`,
  `tools`, `permissions`, `color`, `prompt`).
- `prompt` aceita `{file:...}` (path relativo ao arquivo de config) ou texto inline.

### Merge (ordem de precedência)

1. builtins (Spec 01).
2. `~/.opencode.json` → chaves `agent` e `agents`.
3. `<wd>/.opencode.json` → idem.
4. arquivos `.md` (Spec 01: global < projeto).

Mantida por `agentdef.Registry.Register`, com o loader de config convertendo
`Config.Agent[*]` em `*agentdef.Definition`.

### Validação

- `Validate` (`config.go:609`): continua validando `cfg.Agents` (builtins) como hoje.
- NOVO `validateDefinitions()`: para cada `agentdef` custom com `Model`:
  - resolver via `models.ResolveModelID`; se não existir → `logging.Warn` + herda o modelo do
    agente default (não aborta).
  - `MaxTokens` inválido → aplica default do modelo (reusar lógica de `config.go:536-563`).
- `defaultAgent` inexistente/subagent/hidden → warn + usa `coder`.
- Duplicatas entre `agent` (JSON) e `.md` → JSON vence no mesmo nível (documentar).

### Schema

- Atualizar `opencode-schema.json`: adicionar `agent` (map de definições) e `defaultAgent`;
  manter `agents` e `definitions`.
- Atualizar `.opencode.json` de exemplo, se necessário.

### Back-compat

- Nenhuma chave existente muda de semântica; `agents` (builtins) intacto.
- O override do title (`config.go:211-214`) permanece.
- Config sem `agent`/`defaultAgent` → comportamento atual (4 builtins, default `coder`).

## Mudanças (arquivos)

- `internal/config/config.go`: structs `AgentDefinition`, campos novos, `validateDefinitions`,
  wiring com `agentdef`.
- `internal/config/config_test.go`: casos de merge/validação.
- `opencode-schema.json`, `.opencode.json`: schema/exemplo.

## Critérios de aceite

1. Config antiga carrega igual (sem `agent`/`defaultAgent`).
2. `agent.<nome>` cria agente; `mode`, `description`, `prompt` (inline e `{file:}`) funcionam.
3. `defaultAgent` muda o primário no boot; valor inválido → warn + `coder`.
4. Modelo inexistente em agente custom → warn + herda do default (não aborta).
5. `opencode-schema.json` valida os novos campos.
6. Precedência JSON vs `.md` estável e testada.

## Testes

- `internal/config`: parse de `agent`, `defaultAgent`, `{file:}`, precedência, fallbacks.
- Validação: modelo inválido, `defaultAgent` inválido.
- Regressão: config v0.0.55 carrega idêntica.

## Riscos

- `{file:}` exige resolver path relativo ao config; usar `viper.ConfigFileUsed()`.
- Evitar duplicar a lógica de defaults de modelo já existente (`setDefaultModelForAgent`,
  `config.go:671-817`) — extrair helper e reusar.

## Implementação (fase 5)

Tudo em `internal/config/config.go`, `internal/agentdef/loader.go`, `opencode-schema.json`,
`.opencode.json`; testes em `internal/config/config_test.go` e `internal/agentdef/loader_test.go`.

### Structs e parsing

- `type AgentDefinition struct` (config-facing) espelha o frontmatter do Spec 01 e ainda
  `team` e `prompt`. `Config` ganhou `Agent map[string]AgentDefinition` (json `agent`) e
  `DefaultAgent string` (json `defaultAgent`).
- `loadConfigDefinitions(dirs)` lê o `agent` de cada `.opencode.json` **diretamente** (não via
  viper) para preservar a origem de cada definição. Dedup por nome: dirs posteriores vencem
  (global < local).
- `definitionFromConfig(name, def, baseDir)` converte para `*agentdef.Definition`; `mode`
  inválido → `all` com warn. `resolvePrompt` expande `"{file:./x}"` (relativo a `baseDir`,
  dir do arquivo de config) para o conteúdo; qualquer outro valor é inline. Leitura do arquivo
  é **eager** aqui (o `SetPrompt` marca `hasBody`), pois a definição veio do JSON, não de um
  `.md` lazy.

### Precedência (desvio documentado)

`Spec 05` lista a ordem `builtins → JSON → .md`, mas também diz "JSON vence no mesmo nível"
(l.67). Como `.md` é escaneado por camada (global/projeto), a precedência implementada é:

1. builtins
2. camada global: `.md` globais → `GlobalDefinitions` (JSON global, vence `.md` global)
3. camada de projeto: `.md` do projeto → `ProjectDefinitions` (JSON do projeto, vence `.md` do projeto)

Ou seja: **JSON vence `.md` dentro do mesmo nível**, e **projeto vence global** entre níveis.
`agentdef.Options` ganhou `GlobalDefinitions` e `ProjectDefinitions`; `loadFile` virou o helper
`scanDir` reusado pelas duas camadas.

### Default e validação

- `applyDefaultAgent(reg, name)`: nome válido e não-`disabled` → `reg.SetDefault`; vazio → no-op;
  inválido/disabled → warn + mantém `coder` (default do registry). Fonte do agente ativo no boot
  continua sendo `agentdef.Get().Default()` (usado em `internal/app/agent_manager.go:44`).
- `validateDefinitions()` roda após `loadAgentDefinitions` (as defs só existem no registry
  depois do load). Para cada def não-builtin: `Model` vazio → herda o modelo do `coder`;
  `Model` não resolvível por `models.ResolveModelID` → warn + herda `coder`; `MaxTokens <= 0`
  → `model.DefaultMaxTokens` ou `MaxTokensFallbackDefault`. Nunca aborta.

### Schema / exemplo

- `opencode-schema.json`: novo `agent` (map de definições com `description`, `mode`, `hidden`,
  `disabled`, `team`, `model`, `maxTokens`, `reasoningEffort`, `temperature`, `tools`,
  `permissions`, `color`, `prompt`) e nova chave `defaultAgent`. `agents` e `definitions`
  mantidos intactos.
- `.opencode.json` de exemplo ganhou um agente `security-auditor` (mode `subagent`, prompt inline).

### Back-compat

- Config antiga (sem `agent`/`defaultAgent`) carrega idêntica: as camadas de defs ficam vazias e
  `DefaultAgent` vazio mantém `coder`.
- `agents` (builtins) intacto, inclusive o override de title (`config.go:211-214`).
- Testes: `internal/config/config_test.go` (parse, `{file:}`, inline, dedup local>global, mode
  inválido, `applyDefaultAgent`, `validateDefinitions`) e
  `internal/agentdef/loader_test.go::TestConfigDefinitionsWinAtSameLevel` cobrem os critérios.

Nota: a flake intermitente `TestGetContextFromPaths` (`internal/llm/prompt`, ordem não
determinística por goroutines) é pré-existente e fora do escopo desta fase.

