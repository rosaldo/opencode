# Spec 13 — Agents v2: fonte única, mapa `agents` e permissões array

Onda 2 (paridade com o opencode TS **v2**). Depende de: [01](01-registry-loader.md),
[11](11-config.md). Reaproveita e **consolida** o que a fase 1 e a fase 5 entregaram, sem perda
de características nossas.

> **Regra de ouro.** Nada de alias, nada de "remendo", nada de duplicar chave. Existe **um**
> `agentdef.Definition` e **um** `Registry`; toda forma de declarar agente é uma *source* que
> produz `Definition`. Refazer o que estiver remendado é permitido; reescrever o que já está bom,
> não.

## Motivação

A onda 1 entregou:
- fase 1: registry dinâmico + loader de `.md` (fonte única de metadados, prompt lazy);
- fase 5: ponte config→agentdef via `Options.GlobalDefinitions/ProjectDefinitions` e a chave
  `agent` (singular) no JSON.

Isso funciona, mas o **v2 oficial** usa: arquivo `opencode.json(c)`, chave `agents` (plural, mapa
único builtin+custom), `default_agent`, `system` (prompt inline) e `permissions` como **array**.
A fase 5 deixou a ponte JSON como um caminho paralelo (dois arrays de precedência), o que é o
"remendo" a eliminar: JSON e `.md` devem ser apenas duas implementações da **mesma** interface
`agentdef.Source`.

## Escopo

1. **Fonte única de verdade.**
   - `agentdef.Source` ([Spec 01](01-registry-loader.md)):
     ```go
     type Source interface { Definitions() ([]*Definition, error) }
     ```
   - Dois implementadores: `fileSource` (`.md`, já existe — renomear `scanDir`) e `jsonSource`
     (mapa `agents` do config). Mesmo tipo de saída: `[]*Definition`. Sem tipo intermediário.
   - `Load(sources ...Source)` registra na ordem; **última vence**.
   - Remover a ponte `Options.GlobalDefinitions/ProjectDefinitions` (não há mais dois caminhos).

2. **Config v2 (mapa `agents`).**
   - Aceitar **`agents`** (plural, mapa). Manter **`agent`** (singular, onda 1) como leitura
     *deprecated* por uma **única** versão — mas, definido o redesenho, o objetivo é consolidar em
     `agents`. (`agent` singular deixa de ser um segundo caminho de código: vira apenas um
     carregador do *mesmo* `jsonSource`.)
   - Cada `agents.<n>` = `agentdef.AgentConfig` (espelha o frontmatter):
     ```jsonc
     {
       "agents": {
         "security-auditor": {
           "description": "...",
           "mode": "subagent",
           "model": "anthropic/claude-sonnet-4-5",
           "system": "Você é um auditor...",       // prompt inline (equivale ao corpo do .md)
           "tools": { "write": false },
           "permissions": [ { "action": "shell", "resource": "*", "effect": "deny" } ],
           "hidden": false, "disabled": false,
           "team": "security", "color": "#RRGGBB",
           "maxTokens": 8192, "reasoningEffort": "high", "temperature": 0.2
         }
       },
       "default_agent": "security-auditor"
     }
     ```
   - `description` obrigatório p/ `mode: subagent` (warn se faltar). `mode` inválido → `all` (warn).
   - Builtins (`coder`/`task`/`title`/`summarizer`) entram no mesmo mapa `agents` resolvido; a
     merge final é: builtins < global < projeto (JSON vence `.md` **no mesmo nível**; projeto vence
     global).

3. **`default_agent` (chave do schema) ↔ `Config.DefaultAgent` (campo Go).**
   - A **chave no arquivo** é `default_agent` porque é o schema oficial v2
     (`https://opencode.ai/config.json`) — o mesmo `opencode.json` deve funcionar no TS e no Go.
     Isto **não** é uma escolha de linguagem: é o contrato do arquivo.
   - O **campo Go** é `DefaultAgent` (idiomático) e a chave fica só na tag:
     ```go
     DefaultAgent string `json:"default_agent" yaml:"default_agent"`
     ```
     `snake_case` aparece **apenas** na tag/JSON, nunca no identificador Go.
   - Válido + não `disabled` → `Registry.SetDefault`; inválido/ausente → mantém `coder` (warn se
     inválido). O nome antigo `defaultAgent` (onda 1) é lido como *deprecado*, apontando para o
     **mesmo** campo `DefaultAgent`.

4. **Permissões por agente — array v2 (refazer, não adaptar).**
   - Trocar `Definition.Permissions map[string]any` por `[]PermissionRule`:
     ```go
     type PermissionRule struct {
         Action   string // ex.: "shell", "edit", "read", "webfetch", "task"
         Resource string // glob (ex.: "git push *"), opcional
         Effect   string // "allow" | "deny" | "ask"
     }
     ```
   - **Ordem importa** (a **última** regra que casa vence, conforme a v2). No `.md` e no JSON é lista, não mapa.
   - `action: shell` (não `bash`). Aplicação real em [Spec 07](07-permissions.md); aqui só o tipo
     e o parsing. Migrar o modelo antigo de `internal/permission` (mapa → regras) sem manter os
     dois: **refazer já no formato v2**.

5. **`system` (prompt inline).**
   - No JSON, `system` é o prompt. No `.md`, o corpo é o prompt. Ambos terminam em
     `Definition.Prompt()`; `SetPrompt` para inline. Sem campo `system` no `.md`.

6. **Preservar (características nossas — não são do v2, mas ficam).**
   - `team` (subpasta ou campo) — departamentos/anúncio de delegação (Spec 03).
   - `hidden`, `disabled`, `color`.
   - Prompt **lazy** (escala ilimitada; nenhum dos três ecossistemas documenta lazy).
   - Delegação por nome independe de `mode`; `@`-mention (Spec 04).
   - `Provider`/`Model` v2 com `providers.<p>.models/settings` (Spec 06) — `Definition.Model`
     continua string `provider/model`, resolvida por `models.ResolveModelID`.

7. **Interpolação v2** (`{env:VAR}`, `{file:path}`) — a cargo da [Spec 11](11-config.md); aqui
   apenas garantir que strings de `agents.*` passam pelo interpolador antes do parse.

## Design (Go)

```go
// internal/agentdef/source.go
type Source interface { Definitions() ([]*Definition, error) }

// fileSource: .md (lazy). jsonSource: mapa agents (inline).
func Load(sources ...Source) (*Registry, error)
```

- `jsonSource` recebe `map[string]AgentConfig` + `baseDir` (p/ `{file:}` relativo) e devolve
  `[]*Definition` (com `SetPrompt(system)`, `Permissions []PermissionRule`, `Team`, etc.).
- `Definition` ganha `Permissions []PermissionRule` (substitui o `map[string]any`).
- `config.AgentDefinition` passa a espelhar `agentdef.AgentConfig` 1:1 (sem campo extra): o
  parsing do frontmatter e do JSON compartilham as tags (`yaml`/`json`).

Evidência de mercado que sustenta o desenho (pesquisa na onda 2):
- **Claude Code**: `.claude/agents/*.md`, frontmatter `name`/`description`/`tools`/`model`;
  `name` é identidade, subpasta **não** afeta identidade; `.md`+YAML é o formato dominante.
- **Codex**: `.codex/agents/*.toml`, `name` é fonte de verdade (arquivo é só convenção).
- **opencode v2**: `.md` em `agents/` **e** `agents` no JSON; `system` inline; `permissions` array.
- **Conclusão**: manter `.md`+YAML (maioria) com corpo = prompt, `name` = identidade; nossa
  extensão `team` (path→departamento) é exclusiva e preservada.

## Mudanças (arquivos)

- `internal/agentdef/source.go` (**novo**): interface `Source`, `fileSource` (move `scanDir`),
  `jsonSource`.
- `internal/agentdef/loader.go`: `Load(sources ...Source)`; remove `Options.GlobalDefinitions/
  ProjectDefinitions`.
- `internal/agentdef/definition.go`: `Permissions []PermissionRule`; `SetPrompt`.
- `internal/agentdef/registry.go`: sem mudança de API (segue o núcleo).
- `internal/config/config.go`: `Config.Agents` (`agents`), `DefaultAgent` (`default_agent`),
  `AgentDefinition`→`agentdef.AgentConfig`; monta as sources (builtin/global/projeto + JSON) e
  chama `agentdef.Load`.
- `internal/permission/{policy,scoped,permission}.go`: migrar mapa → `[]PermissionRule` (v2).
- `opencode-schema.json` + `cmd/schema`: refletir `agents`/`permissions` array/`default_agent`.
- `internal/agentdef/*_test.go`, `internal/config/config_test.go`: casos v2.

## Critérios de aceite

1. `agents` (JSON) e `.md` produzem `Definition` pela **mesma** interface `Source` (um só tipo de
   saída; teste prova equivalência).
2. Não existe mais a duplicação `Options.GlobalDefinitions/ProjectDefinitions`; precedência é
   expressa pela ordem das sources.
3. `permissions` é array ordenado (`{action,resource,effect}`) em `Definition` e no `internal/
   permission`; `action: shell` (não `bash`).
4. `default_agent` define o primário; inválido → warn + `coder`.
5. `system` (JSON) e corpo (`.md`) resolvem pelo mesmo `Definition.Prompt()`.
6. Builtins entram no mesmo mapa `agents`; 1000+ agentes carregam só metadados (lazy).
7. Preservados: `team`, `hidden`, `disabled`, `color`, prompt lazy, delegação por nome,
   `@`-mention, back-compat mínima de `agent`/`defaultAgent` (deprecados, um caminho só).
8. Sem ciclos de import (`go build ./...` limpo).

## Testes

- `agentdef`: `jsonSource` e `fileSource` → mesmo `Definition`; ordem/precedência por sources;
  `permissions` array ordenado; `system` inline vs corpo; `team` por subpasta; lazy mantido.
- `config`: parse de `agents` (plural) + `agent` (singular deprecado) na mesma estrutura;
  `default_agent`/`defaultAgent`; `{env:}`/`{file:}` aplicados a `agents.*`.
- `permission`: **última** regra que casa vence; `shell`/`edit`/`read`/`webfetch`/`task`.
- Manual: `go build ./... && go test ./...`.

## Riscos / observações

- **Não** criar `adapter`/`alias` para reconciliar `agent`↔`agents` nem mapa↔array: ambos entram no
  *mesmo* `Definition` via `jsonSource`.
- `agent` singular e `defaultAgent` existem só como leitura com deprecação; um único código os
  consome (não dois caminhos).
- Não reescrever o que a fase 1 acertou (lazy, registry, descoberta): só trocar a costura
  `Options`→`Source` e o tipo de `Permissions`.
- `team` diverge de Claude Code/Codex de propósito (é nossa assinatura); documentar como extensão,
  nunca remover.
