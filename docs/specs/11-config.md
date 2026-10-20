# Spec 11 — Config v2: formato, localização, interpolação e chaves

Onda 2. Depende de: —. É a **base** da onda 2 (as demais specs consomem esta).

## Objetivo

Aproximar a configuração do fork Go do formato **v2** do opencode: arquivo `opencode.json`/
`opencode.jsonc`, `$schema: https://opencode.ai/config.json`, localização e precedência de
diretórios, interpolação `{env:}`/`{file:}` e o conjunto de chaves v2.

## Estado atual (pós-fase 5)

- Arquivo: `.opencode.json` (viper, `internal/config/config.go`), buscado em
  `$HOME`, `$XDG_CONFIG_HOME/opencode`, `$HOME/.config/opencode`, merge com `<wd>/.opencode.json`.
- Chaves: `data, wd, mcpServers, providers, lsp, agents, agent, defaultAgent, debug, debugLSP,
  contextPaths, tui, shell, autoCompact`.
- `agent.<name>` = `AgentDefinition` (onda 1) com `{file:}` no **prompt**.
- Schema gerado por `cmd/schema/main.go` — **desatualizado** (não emite `agent`/`defaultAgent`/
  `shell`/`autoCompact`); `opencode-schema.json` foi editado à mão.
- AUSENTE (v2): `opencode.json(c)`, `$schema` oficial, `{env:}`, `commands`, `instructions`,
  `references`, `skills`, `formatter`, `watcher`, `snapshots`, `media`, `tool_output`,
  `websearch`, `compaction`, `warming`, `worktree`, `update`, `username`, `small_model`,
  `default_agent` (snake_case), `mcp`, `permissions`, `policies`, `plugins`.

## Design

### Formato e localização

- Aceitar `opencode.json` e `opencode.jsonc` (comentários e vírgulas finais). JSONC exige um
  parser tolerante (ex.: `github.com/tailscale/hujson` ou remoção de comentários).
- Precedência (do menor para o maior), conforme v2:
  1. global `~/.config/opencode/opencode.json(c)`
  2. `opencode.json(c)` diretos do mais distante ao mais próximo (subida até a raiz)
  3. `.opencode/opencode.json(c)` (vence os diretos no mesmo nível)
  4. sistema `/etc/opencode/opencode.json(c)` (maior precedência)
- **Back-compat**: continuar aceitando `.opencode.json` (formato atual) com a mesma precedência de
  hoje; a adoção do novo formato é aditiva. Documentar se `.opencode.json` legado continua lido.

### Interpolação de strings

- `{env:VAR}` → valor do env (vazio se ausente).
- `{file:path}` → conteúdo do arquivo; path relativo ao diretório do config, absoluto, ou `~/`.
  Vazio se não existir.
- Aplicar em **qualquer string** do config (não só prompt). Generalizar o `resolvePrompt` da onda 1
  para um `interpolate(s, baseDir)` reusável.

### Chaves v2

- **Núcleo de seleção**: `model` (`provider/model`), `small_model`, `default_agent`.
- **`agents`** como mapa único (builtin + custom) — reconcilia com o `agent`/`agents` da onda 1;
  manter ambos por compat, `agents` é o nome v2.
- **`providers`**: `models` (`modelID`, `name`, `limit.{context,output}`), `settings`
  (`apiKey`, `baseURL`, `headers`, `transport`), variantes — spec 06.
- **`permissions`** (array) e **`policies`** — spec 07.
- **`mcp.servers`** — spec 08.
- **`plugins`** — spec 10.
- **`commands`** (templates de slash command), **`instructions`** (arquivos/globs/URLs de
  instrução), **`references`** (dirs/repos), **`skills`** (dirs/URLs).
- **`formatter`** (bool ou mapa de formatters), **`watcher.ignore`**, **`snapshots`** (bool),
  **`media.image`** (resize/limites), **`tool_output.{max_lines,max_bytes}`**,
  **`websearch.provider`**, **`compaction.{auto,keep,buffer}`**, **`warming`**,
  **`worktree.directory`**, **`update`** (`disable|notify|auto`), **`username`**, **`shell`**.

Nesta fase: **parse + validação + defaults** de todas; **comportamento** das que têm spec própria
fica lá; as demais (commands/references/skills/formatter/watcher/snapshots/media/tool_output/
websearch/compaction/warming/worktree/update/username) podem ser implementadas aqui ou
subdivididas em 11b/11c.

### Gerador de schema

- Reescrever `cmd/schema/main.go` para emitir **todo** o `Config` v2 (incluir `agent`,
  `default_agent`, `shell`, `autoCompact` e as chaves novas), eliminando a edição manual.
- Preferir derivar de tags/reflection; se manual, **obrigatório** um teste de drift que falhe
  quando `opencode-schema.json` divergir do gerador.
- Publicar/versionar um `opencode-schema.json` compatível com a v2.

## Mudanças (arquivos)

- `internal/config/config.go`: localização/formato novos, `interpolate`, structs e chaves v2,
  `default_agent` (alias de `defaultAgent`).
- Novo `internal/config/jsonc.go` (ou dep) para JSONC.
- `cmd/schema/main.go` + `opencode-schema.json`: gerador completo + teste de drift.
- `internal/llm/prompt`: consumir `instructions` no contexto.

## Critérios de aceite

1. Config no formato atual (`.opencode.json`) carrega idêntica (back-compat).
2. `opencode.json` e `opencode.jsonc` (com comentários) são lidos; precedência de diretórios da v2
   respeitada (testado).
3. `{env:VAR}` e `{file:path}` funcionam em strings arbitrárias (não só prompt).
4. `model`, `small_model` e `default_agent` alteram primário/titler/agente default.
5. `opencode-schema.json` contém `agent`, `default_agent`, `shell`, `autoCompact` e as chaves v2.
6. Teste de drift do schema passa (schema == gerador).
7. Chaves de 06/07/08/10 são parseadas e validadas (mesmo sem comportamento aqui).

## Testes

- `internal/config`: JSONC, precedência de diretórios, `{env:}`/`{file:}`, cada chave nova.
- `cmd/schema`: drift (gerar e comparar com o arquivo commitado).

## Riscos

- JSONC + precedência multi-diretório maior que o modelo viper atual — pode exigir trocar a
  estratégia de merge (hoje `viper.MergeConfigMap`).
- Duplicação `agent`/`agents` na transição — mapear com cuidado.
- Escopo amplo: dividir em 11a (formato/localização/interpolação), 11b (chaves núcleo),
  11c (chaves auxiliares) e 11d (schema gerador).
