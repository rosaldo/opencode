# Spec 07 — Permissions e Policies v2: regras por recurso e precedência

Onda 2. Depende de: 11 (config v2). Toca 09 (prompt de permissão na TUI).

## Objetivo

Adotar o modelo v2: **`permissions`** como lista **ordenada** de regras `{action, resource,
effect}` (a última que casa vence) e **`policies`** como camada de governança (`provider.use`,
`tool.use`, bloqueio de MCP/skill/plugin), com precedência entre arquivos de config.

## Estado atual (pós-fase 5)

- `Service` (`internal/permission/permission.go:35-42`): `Grant/GrantPersistant/Deny/Request/
  AutoApproveSession`.
- `Policy` allow|ask|deny com wildcard para **bash**/**task** (`internal/permission/policy.go`);
  escopo por-agente (`scoped.go`), ligado em `tools_filter.go`.
- Gateados: bash, edit/write/patch, fetch, MCP.
- AUSENTE (v2): `permissions` array, `policies`, ações por recurso (path/comando/URL/query),
  `external_directory`, gating de read/glob/grep/websearch, precedência entre arquivos.

## Design

### `permissions` (array ordenado)

```jsonc
{
  "permissions": [
    { "action": "shell", "resource": "*", "effect": "ask" },
    { "action": "shell", "resource": "git push *", "effect": "deny" },
    { "action": "edit",  "resource": "packages/docs/*.mdx", "effect": "allow" },
    { "action": "read",  "resource": "*.env", "effect": "deny" }
  ],
  "agents": {
    "reviewer": {
      "permissions": [{ "action": "edit", "resource": "*", "effect": "deny" }]
    }
  }
}
```

- **Última regra que casa vence**; matching `*` (qualquer, incl. `/`) e `?` (um char).
- Ações (built-in) e recurso:
  `read` (path), `edit` (path de edit/write/patch), `glob` (pattern), `grep` (regex),
  `shell` (comando; compostos podem gerar várias), `subagent` (id do agente),
  `skill` (id), `question` (`*`), `webfetch` (URL), `websearch` (query),
  `external_directory` (boundary externo), `<server>_<tool>` (`*` p/ MCP), `execute` (`*`).
- `read` default allow; `external_directory` habilita paths fora do workspace herdando defaults.
- Precedência: topo < `agents.<id>.permissions` (mais específico vence).

### `policies` (governança)

```jsonc
{
  "policies": [
    { "action": "provider.use", "resource": "*", "effect": "deny" },
    { "action": "provider.use", "resource": "anthropic", "effect": "allow" },
    { "action": "tool.use", "resource": "shell:git push *", "effect": "deny" }
  ]
}
```

- Ações de policy: `provider.use`, `tool.use`, e bloqueio de MCP/skill/plugin.
- Precedência entre arquivos: global e projeto avaliam em ordem reversa; **sistema** vence todos
  (v2). Config gerenciada (MDM/`/etc/opencode`) tem a maior precedência e não é sobrescrita.

### Gating ampliado

- Ligar read/glob/grep/view/webfetch/websearch/diagnostics à permissão, defaults compatíveis
  (allow). Manter `ask`=allow na delegação até `subagent` entrar.

## Mudanças (arquivos)

- `internal/config/config.go`: `Permissions []PermissionRule`, `Policies []PolicyRule`.
- `internal/permission/policy.go` + `scoped.go`: regras ordenadas, matching por recurso, merge
  topo×agente, avaliação de policies.
- Tools read/glob/grep/view/webfetch/websearch/diagnostics: consultar permissão.
- `internal/llm/agent/tools_filter.go`: política efetiva (topo + agente + policies).
- `opencode-schema.json`, `cmd/schema/main.go`.

## Critérios de aceite

1. Modelo atual (`agent.<n>.permissions`) continua funcionando (compat).
2. `permissions: [{shell, "*", ask}, {shell, "git push *", deny}]` bloqueia push, pergunta no resto.
3. `edit` com glob de path concede/nega por arquivo; última regra vence.
4. `external_directory` libera leitura fora do workspace; edit segue regras.
5. `policies provider.use` nega/permite uso de provider; `tool.use` nega `shell:...`.
6. read/glob/grep/webfetch gateáveis, default allow.
7. Schema valida `permissions` e `policies`.

## Testes

- `internal/permission`: matching (última regra vence, globs path/comando/URL), policies.
- `internal/config`: parse de `permissions`/`policies`, precedência topo×agente.
- `internal/llm/agent`: política efetiva.

## Riscos

- Compat: mapear `agent.<n>.permissions` (mapa, onda 1) para o array v2.
- Nome `shell` vs `bash` atual — aceitar ambos na transição.
- Defaults permissivos são intencionais; documentar.
