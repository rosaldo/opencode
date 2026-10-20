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
  `DefaultAgent` (campo Go; tags `json/yaml: "default_agent"` — a chave v2; `defaultAgent` da
  onda 1 é lido como deprecado, mesmo campo).
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

## Status

### 11d — gerador de schema + teste de drift — Verificado

- `cmd/schema/main.go` emite **todo** o `Config` v2 (inclui `agent`, `default_agent`, `shell`,
  `autoCompact`, `model`, `small_model` e as chaves auxiliares da 11c); enum de modelos **ordenado**
  → saída determinística.
- `cmd/schema/main_test.go`: teste de drift — falha quando `opencode-schema.json` diverge do
  gerador; regenerar com `go run ./cmd/schema > opencode-schema.json`.
- `opencode-schema.json` regenerado a partir do gerador (fim da edição manual).
- `internal/llm/prompt`: `instructions` (11c) passa a alimentar o contexto ambiente, junto de
  `contextPaths`.

### 11c — chaves auxiliares — Verificado

- `internal/config/aux.go`: tipos e defaults das chaves auxiliares v2 — `instructions`, `commands`,
  `references`, `skills`, `formatter`, `watcher`, `snapshots`, `media`, `tool_output`, `websearch`,
  `compaction`, `warming`, `worktree`, `update`, `username`, `permissions`.
- Formas polimórficas (`bool`|objeto em `formatter`/`websearch`/`warming`; `string`|objeto em
  `references`; array|objeto em `skills`) resolvidas via `UnmarshalJSON` customizado; campos com
  `mapstructure:"-"` para o viper ignorá-los e `applyAuxConfig` decodificar do mapa mesclado (json).
- Defaults: `tool_output.max_lines=2000`, `max_bytes=51200`, `compaction.auto=true`. Validação
  leniente (`update` inválido descartado; `command` sem template e `reference` vazio removidos).
- Interpolação `{env:}`/`{file:}` aplica-se a estas strings (o mapa mesclado é interpolado antes).
- `cmd/schema/main.go` emite todas as chaves; `opencode-schema.json` regenerado (determinístico).

### 11b — chaves núcleo (`model`, `small_model`, `default_agent`) — Verificado

- `Config.Model` (`model`) e `Config.SmallModel` (`small_model`): `model` define o modelo do
  agente primário (o `default_agent`, caindo em `coder`) e dos subagentes sem modelo próprio;
  `small_model` define o modelo de `title` e `summarizer`. Modelo explícito em
  `agents.<nome>.model` sempre vence.
- `applyModelSelection` roda após o unmarshal; referências `provider/model` são resolvidas por
  `models.ResolveModelID`; modelo de topo não resolvível é avisado e ignorado.
- `validateAgent` passou a resolver `provider/model` antes do lookup (builtins incluídos).
- `cmd/schema/main.go` emite `model`/`small_model` e o enum de modelos agora é **ordenado**
  (saída determinística, base para o teste de drift da 11d); `opencode-schema.json` regenerado.

### 11a — formato, localização e interpolação — Verificado

- `internal/config/configfiles.go`: `configFiles(workingDir)` descobre os arquivos em ordem de
  precedência (global `~/.config/opencode/opencode.json(c)` < `opencode.json(c)` de cada ancestral
  da raiz ao wd < `.opencode/opencode.json(c)` no mesmo nível < legado `.opencode.json` no nível do
  wd < `/etc/opencode/opencode.json(c)`); o legado é lido nos mesmos slots (global e projeto).
- `internal/config/jsonc.go`: `stripJsonc` remove comentários e vírgulas finais em duas passadas,
  respeitando strings/escapes, sem dependência nova.
- `internal/config/interpolate.go`: `interpolate(s, baseDir)` expande `{env:VAR}` e `{file:path}`
  (absoluto, `~/` ou relativo a `baseDir`); placeholder desconhecido fica intacto; arquivo/env
  ausente → vazio.
- `Load` passa a ler **todos** os arquivos via `readConfigMaps` (JSONC + interpolação recursiva em
  **qualquer string**, baseDir por arquivo) e a alimentar o viper com o mapa mesclado; viper guarda
  apenas defaults/env. `configureViper` não faz mais descoberta de arquivos.
- `jsonSources`/`readAgentConfigsFrom`/`interpolateAgentConfigs`: as declarações `agents`/`agent`
  (v2 vence sobre o deprecado) são lidas por arquivo com JSONC e interpolação de todas as strings
  (`description`, `mode`, `team`, `model`, `reasoningEffort`, `color`, `system`).

## Testes

- `internal/config`: JSONC, precedência de diretórios, `{env:}`/`{file:}`, cada chave nova.
- `cmd/schema`: drift (gerar e comparar com o arquivo commitado).

## Riscos

- JSONC + precedência multi-diretório maior que o modelo viper atual — pode exigir trocar a
  estratégia de merge (hoje `viper.MergeConfigMap`).
- Duplicação `agent`/`agents` na transição — mapear com cuidado.
- Escopo amplo: dividir em 11a (formato/localização/interpolação), 11b (chaves núcleo),
  11c (chaves auxiliares) e 11d (schema gerador).
