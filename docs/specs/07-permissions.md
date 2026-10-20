# Spec 07 — Permissions: regras globais, globs por recurso e gating ampliado

Onda 2. Depende de: 11 (chave `permission` na config). Toca 09 (prompt de permissão na TUI).

## Objetivo

Ir além do permission por-agente atual: regras **globais** na config, chaves por **tool** com
**globs de recurso** (comando/path/URL/query), `external_directory`, e gating dos tools que hoje
não passam por permissão (read/glob/grep/view/webfetch/websearch/diagnostics).

## Estado atual (pós-fase 5)

- `Service` (`internal/permission/permission.go:35-42`): `Grant/GrantPersistant/Deny/Request/
  AutoApproveSession`; `Ask` bloqueante por canal.
- `Policy` allow|ask|deny com wildcard para **bash**/**task**: `internal/permission/policy.go:11-22`,
  `PermissionKey` `:164`.
- Escopo por-agente (deny final): `internal/permission/scoped.go:20-40`, ligado em
  `internal/llm/agent/tools_filter.go:76-80`; config `agent.<name>.permissions`.
- Gateados: bash (`tools/bash.go:274`), edit/write/patch (`tools/edit.go:204,315,435`), fetch
  (`tools/fetch.go:124`), MCP (`llm/agent/mcp-tools.go:92`).
- Diálogo TUI: `internal/tui/components/dialog/permission.go` (a/s/d; detalhes por tool).
- AUSENTE: `permission` global na config; globs de recurso (path/comando/URL); `external_directory`;
  gating de read/glob/grep/view/webfetch/websearch.

## Design

### Config: chave `permission`

```jsonc
{
  "permission": {
    "*": "ask",
    "bash": { "*": "ask", "git status *": "allow", "git push *": "deny" },
    "edit": { "*": "deny", "packages/**/*.go": "allow" },
    "webfetch": { "*": "ask", "https://en.wikipedia.org/*": "allow" },
    "external_directory": { "~/projects/personal/**": "allow" }
  },
  "agent": { "reviewer": { "permission": { "edit": "deny" } } }
}
```

- Regras avaliadas por **pattern match**, **a última que casa vence** (como no TS).
- Valor pode ser string (`"ask"`) ou mapa recurso→efeito.
- Precedência: regra do agente (`agent.<n>.permission`) > regra global; dentro de um nível, a
  última regra de recurso que casa.

### Modelo de política

- Generalizar `permission.Policy` para receber uma lista ordenada de regras por `PermissionKey`
  (`action`, `resource`, `effect`), com matching `*`/`?`, aplicado ao **recurso** de cada tool:
  - bash → string do comando;
  - edit/write/patch → path normalizado;
  - webfetch → URL; websearch → query;
  - glob → pattern; grep → regex;
  - read → path (local ou absoluto externo);
  - `external_directory` → boundary canônico;
  - `<server>_<tool>` → `*` para MCP.
- `read` default = allow; `external_directory` habilita paths fora do workspace herdando os
  defaults.

### Gating ampliado

- Ligar permissão em read/glob/grep/view/webfetch/websearch/diagnostics, com defaults
  compatíveis (allow) para não quebrar fluxo.
- `ask` = allow na delegação permanece (spec 03); reavaliar se `permission.subagent` entrar.

## Mudanças (arquivos)

- `internal/config/config.go`: `Config.Permission` (global) + parse de regras por tool;
  `agent.<n>.permission` (compat com `permissions` atual).
- `internal/permission/policy.go` + `scoped.go`: regras ordenadas, matching de recurso, merge
  global×agente.
- Tools `read/glob/grep/view/webfetch/websearch/diagnostics`: consultar permissão.
- `internal/llm/agent/tools_filter.go`: injetar a política efetiva (global + agente).
- `opencode-schema.json`, `cmd/schema/main.go`: chave `permission`.

## Critérios de aceite

1. Config antiga e per-agent atual continuam funcionando (compat).
2. `permission.bash["git push *"] = deny` bloqueia push; `allow` libera; `ask` abre diálogo.
3. `permission.edit` com glob de path concede/nega por arquivo.
4. `external_directory` libera leitura fora do workspace; edit fora continua negado salvo regra.
5. read/glob/grep/webfetch passam a ser gateáveis, default allow.
6. Última regra que casa vence (testado).
7. Schema valida `permission`.

## Testes

- `internal/permission`: tabela de matching (última regra vence, globs de path/comando/URL).
- `internal/config`: parse de `permission` global e por-agente, precedência.
- `internal/llm/agent`: política efetiva por agente.

## Riscos

- Compat: manter `agent.<n>.permissions` (onda 1) funcionando — mapear para o novo modelo.
- Defaults permissivos são intencionais; mudar defaults pode surpreender — documentar.
- Ferramentas que hoje não têm "recurso" precisam de definição clara de recurso.
