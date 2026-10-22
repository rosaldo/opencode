# Spec 09 — TUI: keybinds configuráveis, mouse, sessões e paste

Onda 2. Depende de: 11 (keybinds na config). Toca 07 (prompt de permissão).

## Objetivo

Fechar lacunas da TUI: **keybinds configuráveis**, suporte a **mouse/scroll**, gestão de sessões
(buscar/renomear/apagar/exportar), **paste de imagem/clipboard** e refinamentos (hyperlink/kitty).

## Estado atual (pós-fase 5)

- bubbletea + lipgloss + bubbles; root `internal/tui/tui.go` (~1025 linhas); páginas
  `page/{chat,logs}.go`.
- Keymap fixo `tui.go:45-87`: `ctrl+l` logs, `ctrl+c` quit, `ctrl+?` help, `ctrl+s` session,
  `tab`/`ctrl+a` agent, `ctrl+k` commands, `ctrl+f` filepicker, `ctrl+o` models, `ctrl+t` theme.
- 13 diálogos em `internal/tui/components/dialog/` (agents, arguments, commands, complete,
  custom_commands, filepicker, help, init, models, permission, quit, session, theme).
- Status bar `core/status.go:123-184` (tokens/custo, LSP, `agente · modelo`); sidebar com diffs
  `components/chat/sidebar.go:245-312`.
- Editor: até 5 anexos / imagens 5MB, `ctrl+e` `$EDITOR`, `@name` subagente, `@` file completion.
- Sessão: lista ≤10, `j/k`, sem busca/rename/delete (`dialog/session.go`).
- AUSENTE: mouse (`bubblezone` importado mas não usado; sem `tea.WithMouse*`), keybinds
  configuráveis, busca/rename/delete/export de sessão, paste de imagem, hyperlink/kitty/sixel.

## Design

### Keybinds configuráveis

```jsonc
// tui.json (ou seção tui da config)
{ "keybinds": { "leader": "ctrl+x", "agent_list": "<leader>a", "session_delete": "ctrl+d" } }
```

- Mapa nome→binding com defaults; `<leader>` e combos; `"none"`/`false` desabilita.
- Definir o conjunto de IDs (subset do TS): `app_exit`, `command_list`, `agent_list`,
  `session_list/new/rename/delete/interrupt/compact`, `model_list`, `theme_list`,
  `sidebar_toggle`, `prompt_paste`, etc.
- Merge sobre defaults; IDs desconhecidos rejeitados com aviso.

### Mouse

- Habilitar `tea.WithMouseCellMotion`; usar `bubblezone` (já inicializado em `cmd/root.go:11,119`)
  para zonas (sidebar, lista de mensagens, diálogos); scroll por roda.

### Sessões

- Busca incremental, renomear, apagar (com confirm), exportar/copiar, timeline.

### Editor

- Paste de imagem do clipboard (Linux/macOS), exibição de imagem em mensagens (o filepicker já
  renderiza ANSI; `view.go:166` tem TODO de imagens no chat).

## Mudanças (arquivos)

- `internal/tui/tui.go`: keymap resolvido de config, opções de mouse.
- Novo `internal/tui/keybinds.go`: defaults + merge + validação.
- `internal/tui/components/dialog/session.go`: busca/rename/delete/export.
- `internal/tui/components/chat/editor.go`, `message.go`, `view.go`, `list.go`: mouse/scroll, paste.
- `internal/config`: seção `tui.keybinds` (ou `tui.json`) — coordenar com spec 11.
- `internal/tui/util`/`clipboard`: helper de paste.

## Critérios de aceite

1. Keybind default inalterado sem config.
2. `keybinds.session_delete = "ctrl+d"` passa a apagar sessão; `"none"` desabilita.
3. Mouse: clique/seleção em zonas e scroll por roda na lista de mensagens/sidebar.
4. Sessão: buscar, renomear e apagar funcionam.
5. Paste de imagem anexa ao editor; imagem aparece na mensagem.
6. ID de keybind desconhecido é rejeitado com aviso (não trava).

## Testes

- `internal/tui`: resolução/merge/validação de keybinds (tabela).
- Dialog de sessão: busca/rename/delete (lógica pura).
- Mouse: teste de hit-test das zonas (bubblezone) sem depender de terminal.

## Status

- **Verificado**:
  - **Keybinds configuráveis** (09a): `tui.keybinds` (`internal/config/config.go` `TUIConfig.Keybinds`)
    com `internal/tui/keybinds.go` (defaults id→binding, `<leader>` resolvido para sequência de
    duas teclas, `"none"`/`false` desabilita, ids desconhecidos → aviso e ignorados). `applyKeybinds`
    reconstrói o `keyMap` do `tui.go` a partir da config; defaults idênticos ao keymap anterior.
    Testes: `internal/tui/keybinds_test.go`, `internal/config/tui_test.go`. Schema emite `tui.keybinds`.
  - **Sessões**: diálogo com busca incremental (`/`), renomear (`r`) e apagar com confirmação (`d`
    + `y`); emite `SessionRenameRequestMsg`/`SessionDeleteRequestMsg` tratados no `tui.go` via
    `Sessions.Save`/`Delete`. Testes: `dialog/session_test.go`.
  - **Mouse**: `tea.WithMouseCellMotion()` no `cmd/root.go`; roda (wheel) traduzida para `up`/`down`
    no `tui.go` e encaminhada para o diálogo ativo ou página atual.
  - **Paste de imagem**: `internal/tui/util/clipboard.go` (macOS `pngpaste`; Linux `wl-paste`/`xclip`),
    com fallback para texto; `ctrl+v` no editor anexa imagem (respeita `maxAttachments`) ou cola texto;
    `SniffImageMime`. Testes: `internal/tui/util/clipboard_test.go`.
  - **Imagens/kitty/sixel + hyperlink** (09b): `internal/tui/util/terminal_image.go` —
    `DetectImageProtocol` (kitty ≫ sixel ≫ half-block) e `RenderHalfBlocks` (truecolor `▀`),
    `KittyGraphics` (chunks base64 da graphics protocol), `Hyperlink` (OSC 8) e `SniffImageBytes`.
    `message.go` (`renderUserMessage`) desenha anexos de imagem inline (kitty quando disponível,
    senão half-blocks) e transforma o chip de arquivo num hyperlink `file://` clicável. Testes:
    `internal/tui/util/terminal_image_test.go`.
  - **Hit-test de zonas** (09b): `internal/tui/util/zones.go` (`ZoneSet` sobre o `bubblezone`
    global; `Mark` registra/decora, `At` resolve o id sob o cursor). A lista de mensagens
    (`chat/list.go`) marca cada mensagem (`msg:<id>`) e, em clique esquerdo, foca e rola até a
    mensagem clicada (`scrollToMessage`). Teste de hit-test em `terminal_image_test.go`.
  - **Paridade funcional com TS 2.0.0 (09c)** — comandos `/` e shell `!`:
    - Parser puro `internal/tui/components/chat/slash.go` (`ParseInput`): `/cmd args` → `InputSlash`, `!sh` → `InputShell`, só no início do input; testes `slash_test.go`.
    - Editor intercepta `/` e `!` com textarea vazio: `/` emite `RunSlashCommandMsg` e abre o command dialog (filtrado por nome); `!` entra em **shell mode** (`!` no prompt, esc sai). Enter em `!cmd` executa via `sh -c`, captura saída e injeta `!cmd\n\n```console\n$ ...\n...\n```` como prompt, acionando o agente.
    - `tui.go` filtra o dialog por nome (`filterCommands`, testado). Testes: `internal/tui/commands_test.go`.

  - **Command palette + leader keys (09d)** — feito:
    - **Command palette `ctrl+p`**: id `command_palette` em `keybinds.go`, resolve para `ctrl+p` e
      abre o mesmo dialog que `ctrl+k` (`openCommandDialog`) — coexistem e são configuráveis.
    - **Leader**: `resolveBinding` vira sequência de duas teclas (`<leader>x` → `[leader, x]`);
      `keys.Leader` sai de `tui.keybinds.leader` e `applyLeaderBindings` (`tui.go`) monta
      `leaderSuffixes` (sufixo → ação) a partir dos próprios keybinds — se um id for reconfigurado
      sem o prefixo (ex.: `session_delete: ctrl+d`) vira `leaderDirect` (tecla direta); `none`
      remove a ação. Ações: `session_new` → `page.NewSessionMsg`, `sidebar_toggle` →
      `page.ToggleSidebarMsg` (ambos tratados pelo `chatPage`, que guarda `sidebarVisible`; `n` limpa
      a sessão/sidebar e `chat.SessionClearedMsg` zera `appModel.selectedSession`, então
      `session_compact` não resume a sessão anterior), `session_rename`/`session_delete` → diálogo de
      sessão aberto **com a sessão atual pré-selecionada** (`SetSessions` + `SetSelectedSession`),
      `session_compact` → `startCompactSessionMsg`.
    - Só ativa no ChatPage com nenhum diálogo aberto (`inChatContext`); qualquer outra tecla cancela
      o prefixo (`ctrl+c` é repassado ao handler de saída).
    - **Busca fuzzy no command dialog**: modo `search` ativado por `/` (mesmo modelo do diálogo de
      sessão), `fuzzy.RankFindFold` sobre `title + description` ordenado por Levenshtein, `esc`/`enter`
      saem preservando a lista filtrada e `SetCommands` reseta modo/query ao reabrir. `dialog/commands_test.go`.
    - **`prompt_paste` ligado**: `chat.SetPromptPasteKeybind` recebe
      `resolvedKeybinds.Keys(KeybindPromptPaste)` em `applyKeybinds`; `ctrl+v` vira configurável.
    - **Which-key + timeout do prefixo**: `beginLeader`/`cancelLeader` (`appModel.leaderSeq` invalida
      ticks obsoletos), `tea.Tick` de `tui.leader_timeout` (ms, default **2000**, mesmo valor do
      `tui.json` do TS; `config.TUIConfig.LeaderTimeout` + propriedade no `opencode-schema.json`) e
      popup com os sufixos válidos (`leaderHintView`, linhas ordenadas por `leaderHintRows`) ancorado
      no rodapé. Testes: `internal/tui/leader_test.go`.
    - Testes: `internal/tui/keybinds_test.go` (defaults `<leader>`, override, `none`,
      `applyLeaderBindings` defaults e reconfigurado).
  - **Blocos thinking/reasoning (09e)** — feito:
    - `renderThinkingBlock` (`message.go`): `▸ Thinking · N lines` recolhido; `▾` + conteúdo
      (markdown esmaecido, borda `TextMuted`) quando expandido.
    - `renderAssistantMessage` ganhou `thinkingExpanded` e **sempre** emite o bloco de raciocínio
      antes da resposta (`thinkingMessageType`, zona `think:<id>`); o `else if` antigo descartava o
      conteúdo do thinking (bug) e o bloco nunca era anexado.
    - Estado por mensagem em `messagesCmp.thinkingExpanded`; clique esquerdo na zona `think:`
      alterna, invalida o cache (`cachedContent`) e re-renderiza.
    - Testes: `internal/tui/components/chat/message_test.go` (recolhido/expandido, ordem dos
      blocos, zona e posição).

- Pendências conscientes: codificação **sixel** de fato (hoje o protocolo sixel detectado usa o
  fallback half-block, que funciona em qualquer terminal truecolor) e integração de hit-test das
  demais zonas (sidebar/diálogos).

## Riscos

- Mouse muda eventos: revisar handlers para não quebrar teclado.
- Paste de imagem é dependente de plataforma (X11/Wayland/macOS) — degradar graciosamente.
- Coordenar formato de keybinds com a spec 11 para não duplicar.
