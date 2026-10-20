# Spec 11 — Config: chaves faltantes, schema gerador e TUI config

Onda 2. Depende de: —. É pré-requisito de 07 (permission), 09 (keybinds), 10 (plugin).

## Objetivo

Alinhar o conjunto de chaves de config com o TS e consertar o gerador de schema: hoje faltam
`instructions`, `keybinds`, `share`, `autoupdate`, `permissions`, `formatters`/`formatter`, e o
gerador (`cmd/schema/main.go`) não emite `agent`/`defaultAgent`/`shell`/`autoCompact`.

## Estado atual (pós-fase 5)

- Fontes: `.opencode.json` global (`$HOME`, `$XDG_CONFIG_HOME/opencode`, `$HOME/.config/opencode`)
  + `<wd>/.opencode.json`; env `OPENCODE_*`; defaults em `setDefaults`.
- Chaves parseadas (`Config` struct): `data, wd, mcpServers, providers, lsp, agents, agent,
  defaultAgent, debug, debugLSP, contextPaths, tui, shell, autoCompact`.
- `agent.<name>` = `AgentDefinition` (onda 1, `{file:}` incluso).
- Persistência: `UpdateAgentModel`, `UpdateTheme` (escrevem no `.opencode.json`).
- Schema gerado por `cmd/schema/main.go:38-307` lista só `data, wd, debug, debugLSP, contextPaths,
  tui, mcpServers, providers, agents, lsp` — **não** emite `agent`, `defaultAgent`, `shell`,
  `autoCompact`. O `opencode-schema.json` commitado foi editado à mão na spec 05.
- AUSENTE: `instructions`, `keybinds`, `share`, `autoupdate`, `permission` (global), `formatters`.

## Design

### Chaves novas (nesta fase)

```jsonc
{
  "instructions": ["docs/AGENTS.md", "~/.config/opencode/rules/*.md"],
  "formatter": { "golang": { "command": ["gofmt", "-w", "$FILE"], "extensions": [".go"] } },
  "autoupdate": true,
  "keybinds": { },        // usado pela spec 09
  "permission": { },      // usado pela spec 07 (parse, sem lógica aqui)
  "share": "manual"       // usado pela spec 12
}
```

- `instructions`: lista de caminhos (globs, `~`) cujo conteúdo entra no contexto como AGENTS.md.
  Coordenar com `contextPaths` existente (talvez unificar/aliás).
- `formatter`: formatters por linguagem (command/extensions/disabled), aplicados após edit.
- `autoupdate`: liga/desliga checagem de atualização (sem rede nesta fase; só a chave + gate).
- `keybinds`, `permission`, `share`: **parse e validação** nesta fase; comportamento nas specs
  09/07/12.

### Gerador de schema

- Reescrever `cmd/schema/main.go` para refletir **todo** o `Config` (incl. `agent`, `defaultAgent`,
  `shell`, `autoCompact`, e as novas chaves), eliminando a edição manual do `opencode-schema.json`.
- Idealmente o schema é derivado do struct (reflection/tags) para não voltar a divergir.
- Adicionar um **teste** que falha se `opencode-schema.json` divergir do gerador.

## Mudanças (arquivos)

- `internal/config/config.go`: `Instructions`, `Formatter`, `Autoupdate`, `Keybinds`,
  `Permission`, `Share`.
- `cmd/schema/main.go` + `opencode-schema.json`: gerar todas as chaves.
- `internal/llm/prompt` (context): incluir `instructions` no contexto.
- Novo `internal/format`/`internal/llm/tools`: aplicar `formatter` após edit (se entrar no escopo).

## Critérios de aceite

1. Config antiga carrega igual.
2. `instructions` adiciona arquivos ao contexto do modelo.
3. `formatter.golang` roda `gofmt` após editar `.go`.
4. `opencode-schema.json` contém `agent`, `defaultAgent`, `shell`, `autoCompact` e as chaves novas.
5. Teste de drift do schema passa (schema == gerador).
6. Chaves usadas por 07/09/12 são parseadas e validadas (mesmo sem comportamento).

## Testes

- `internal/config`: parse de cada chave nova + validação.
- `cmd/schema`: teste de drift (gerar e comparar com o arquivo commitado).

## Riscos

- `instructions` vs `contextPaths`: definir uma semântica única para evitar confusão.
- Formatter executa comandos externos — interage com permissões (spec 07); decidir gate.
- Derivar schema por reflection pode ser trabalhoso; aceitável manter geração manual **desde que
  com teste de drift**.
