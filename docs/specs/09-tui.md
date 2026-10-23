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
  `session_list/new/rename/delete/interrupt/compact/export`, `model_list`, `theme_list`,
  `sidebar_toggle`, `prompt_paste`, `history_previous/next`, `display_thinking`, `messages_copy`,
  `messages_undo/redo`, etc.
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
    `message.go` (`renderUserMessage`) desenha anexos de imagem inline (kitty/sixel quando
    disponível, senão half-blocks) e transforma o chip de arquivo num hyperlink `file://` clicável. Testes:
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
  - **Histórico de prompt (09f)** — feito:
    - IDs `history_previous`/`history_next` (defaults `up`/`down`, como no TS) em `keybinds.go`;
      `applyKeybinds` chama `chat.SetHistoryKeybinds`, no mesmo padrão do `prompt_paste`
      (`none` desliga a navegação).
    - `editorCmp.history` guarda os prompts enviados na **sessão atual** (`recordHistory` colapsa
      repetição imediata); `historyIdx == len(history)` = rascunho vivo e `historyDraft` guarda o
      texto digitado quando o usuário volta no histórico. Trocar/limpar sessão zera o histórico
      (`SessionSelectedMsg`/`SessionClearedMsg`).
    - `up` assume o histórico só com prompt de **uma linha** no rascunho (o textarea do bubbles não
      expõe a linha do cursor, então em prompt multilinha `up` segue movendo o cursor); em modo
      delete/shell ou com o prompt desfocado nada navega, e uma vez voltando, `up` continua mesmo
      com o histórico multilinha restaurado. `down` só anda com histórico a percorrer e restaura o
      rascunho ao fim.
    - Testes: `internal/tui/components/chat/editor_history_test.go`.
  - **Export da conversa (09g)** — feito:
    - Comando **`/export`** registrado em `tui.New` (ID `export`, "Export Session") →
      `exportSessionMsg` tratado no `appModel`: valida a sessão ativa, `app.Messages.List`,
      renderiza Markdown (`chat.SessionToMarkdown`: título, `> N messages · exported`, seções por
      papel, texto raciocínio/tool call/tool result em fence — `Finish`/imagens descartados),
      grava em `os.CreateTemp("opencode-export-*.md")` e abre no `$EDITOR` via `tea.ExecProcess`
      (`chat.EditorCommand`, extraído do `ctrl+e`); avisa "No active session to export" /
      "Nothing to export yet".
    - **`session_export`** (`<leader>x`, igual ao TS): `leaderExportSession` em `runLeaderAction`
      e linha no which-key ("export session").
    - Testes: `internal/tui/components/chat/export_test.go` (render + título padrão + tool error),
      `leader_test.go`/`keybinds_test.go` (sufixo `x`).
  - **Comandos built-in do TUI (09h)** — feito:
    - `dialog.Command` ganhou `Aliases []string` e `filterCommands`/`matchesCommandName` aceitam
      id **ou** alias, com maiúsculas/minúsculas ignoradas.
    - Registrados em `tui.New`: `new` (alias `clear` → `page.NewSessionMsg`), `sessions`
      (`resume`, `continue`), `models`, `themes`, `help`, `editor` (→ `chat.OpenEditorMsg` →
      editor abre `$EDITOR` respeitando o busy guard) e `exit` (`quit`, `q` → diálogo de
      confirmação, igual à tecla `keys.Quit`).
    - Handlers não alcançam o `appModel`: cada um emite um msg (`showSessionListMsg`,
      `showModelListMsg`, `showThemeListMsg`, `showHelpMsg`, `showQuitMsg`) tratado no `Update`,
      que delega aos métodos extraídos do switch de teclas (`openSessionList`, `openModelDialog`,
      `openThemeDialog`, `openHelp`, `toggleQuit`) — `ctrl+s`/`ctrl+o`/`ctrl+t`/`ctrl+c` agora
      compartilham o mesmo caminho que os comandos (sem mudança de comportamento).
    - Testes: `internal/tui/commands_test.go` (alias/case-insensitive).
  - **Comando `/thinking` + `display_thinking` (09i)** — feito:
    - `renderAssistantMessage` ganhou `showThinking`: com `false` o bloco de raciocínio não é
      gerado (nem a zona `think:`), mantendo a mensagem/respuesta intactas.
    - `messagesCmp.showThinking` (default **true**), `chat.ToggleThinkingMsg` limpa todo o
      `cachedContent` e re-renderiza, reportando "Thinking blocks shown/hidden"; comando
      `/thinking` em `tui.New` e keybind id `display_thinking` (default `none`, igual ao TS) que
      vira a ação de leader `leaderToggleThinking` quando configurado.
    - Testes: `message_test.go` (hidden com/sem resposta), `list_test.go` (toggle).
  - **Cópia de mensagem (09j)** — feito:
    - `messages_copy` (`<leader>y`, igual ao TS) → ação de leader `leaderCopyMessage` →
      `chat.CopyMessageMsg` → `messagesCmp`: copia a mensagem **focada** (`currentMsgID`; o
      `SetSession` foca a última) ou, sem foco, a mais recente.
    - `MessagePlainText` achata as partes de texto (`TextContent` unidos por `\n\n`, raciocínio
      descartado; sem texto, os nomes dos tool calls) e `util.WriteClipboardText`
      (`pbcopy`/`wl-copy`/`xclip -selection clipboard`, timeout de 2s) degrada para
      `ErrClipboardUnavailable` com aviso.
    - Testes: `list_test.go` (plain text/alvo de cópia), `util/clipboard_test.go`
      (helper ausente/encadeado, sem tocar no clipboard real), `leader_test.go`/
      `keybinds_test.go` (sufixo `y`).
  - **Undo/Redo de mensagem (09k)** — feito (com rollback de arquivos via snapshots):
    - `/undo` → `undoMessageMsg` → `appModel.undoMessage()` → `stageRevert`: apaga a mensagem
      de usuário mais recente e **todas** as mensagens seguintes (`Messages.Delete`), empilha o
      lote (`undoneBatch`) e devolve o texto ao prompt (`chat.RestorePromptMsg`, só se o draft
      estiver vazio) para edição. Guards: sem sessão, sessão ocupada (`IsSessionBusy`), nada a
      desfazer.
    - O turno desfeito também é desfeito no **working tree**: cada turno de usuário guarda um
      snapshot (`SnapshotContent` na user message); o undo restaura o snapshot capturado antes
      do turno (`snapshot.Service.Restore`), apaga arquivos criados pelo turno e mostra o banner
      "N messages reverted · M files rolled back" (ver 09u).
    - `/redo` → `redoMessageMsg` → `clearRevert`: restaura o snapshot pós-turno, recria o lote na
      mesma ordem (`Messages.Create`) e limpa o banner. Sem revert staged, avisa "Nothing to
      redo" (não é uma pilha de redo). Partes `Finish` sintéticas são removidas dos papéis
      não-assistente antes de recriar. **Não** reexecuta o agent.
    - Keybinds: `messages_undo` (`<leader>u`, igual ao TS) e `messages_redo` (`<leader>.` — o
      default do TS é `<leader>r`, colidindo com o `session_rename` legado deste fork; `.` mantém
      o mnemônico do vim). Sufixos `u`/`.` no which-key ("undo/redo message").
    - `applyLeaderBindings` agora percorre as ações em **ordem determinística** e avisa no log
      quando dois keybinds disputam o mesmo sufixo (o primeiro da lista vence).
    - **Snapshot service (`internal/snapshot`)**: repo git sombra em
      `$XDG_CONFIG_HOME/opencode/snapshot/<hash do worktree>` (fora da árvore do usuário), com
      `Capture` (sincroniza arquivos reportados pelo `git status` + `write-tree`), `Restore`
      (`read-tree` + `checkout-index` para um diretório temporário e cópia para o worktree),
      `Diff`/`Files` (`git diff --name-status` entre trees) e `RemoveFiles` (apaga arquivos que
      não existiam no snapshot alvo). Na primeira execução o repo sombra é **semeado** com todos
      os arquivos rastreados/não-ignorados (`git ls-files`), para que um capture em árvore limpa
      já conheça o conteúdo pré-turno. Desabilitado quando o worktree não é um repo git ou
      `snapshots: false` na config.
    - Testes: `undo_test.go` (`lastUserMessageIndex`, `stripFinishParts`), `message_actions_test.go`
      (`boundarySnapshot`, banner), `message/content_test.go` (round-trip do `SnapshotContent`),
      `snapshot/snapshot_test.go` (capture/diff/restore, seed em árvore limpa, arquivo criado como
      `D` no diff reverso, `RemoveFiles`, repo não-git), `git/git_test.go`.
    - Adaptações conscientes: o snapshot é capturado **por turno de usuário** (não por assistant
      message como `snapshot.start`/`snapshot.files` no TS); o revert staged é um **slot único
      global** (o TS tem `session.revert` por sessão) — stagear em outra sessão avisa "Another
      session has a staged revert; clear it first"; o banner aparece **acima do prompt no
      editor**, não no transcript; symlinks são copiados como arquivos regulares e o modo de
      arquivo não é preservado; arquivos ignorados pelo git não entram no snapshot.
  - **Detalhes de tool (09l)** — feito:
    - `/details` → `chat.ToggleDetailsMsg` → `messagesCmp.showDetails` (default **visível**):
      quando oculto, tool calls finalizados renderizam só `nome: params` (uma linha) — sem o
      corpo da resposta, sem o placeholder "Waiting for response..." e sem os filhos aninhados
      de `task` (agent). Tool calls em execução já são one-liners e não mudam.
    - O toggle limpa o `cachedContent` (todo render cacheado pode embutir uma resposta) e
      re-renderiza; keybind id `tool_details` registrado com default `none` (igual ao TS — o
      comando `/details` é a forma descobrível).
    - Testes: `message_test.go` (resposta visível/oculta, params preservados), `list_test.go`
      (`TestToggleDetails`).
  - **Keybinds editor_open/tool_details (09m)** — feito:
    - `editor_open` (`<leader>e`, igual ao TS) → `leaderOpenEditor` → `chat.OpenEditorMsg`
      (o editorCmp já tem o guard de sessão ocupada).
    - `tool_details` (default `none`) → `leaderToggleDetails` → `chat.ToggleDetailsMsg`
      (permutável via `tui.json`, como no TS).
    - `applyLeaderBindings` cobre as duas ações; which-key ganha o sufixo `e` ("open editor").
    - Testes: `leader_test.go`/`keybinds_test.go` (sufixo `e`).
  - **Hit-test de mouse das zonas restantes (09n)** — feito:
    - O `case tea.MouseMsg` do `appModel` engolia cliques (`default: return a, nil`): só o
      wheel era traduzido para up/down, então o handler de clique do `messagesCmp` (zonas
      `think:`/`msg:`) era código morto. Agora o wheel segue traduzido (com a rota do tema
      incluída) e o clique esquerdo vai para `forwardMouseClick`: diálogo aberto recebe o
      MouseMsg cru (nunca a página por baixo), senão a página atual (`chatPage` broadcast →
      messagesCmp/sidebar).
    - Diálogos de lista (session/model/theme) ganharam `clickRow(idx)` puro e testável e
      zonas com **índice absoluto** no View (`sd:`/`md:`/`td:`; `slist:` no `SimpleList`,
      com helpers exportados `ListZoneID`/`ListClickedRow`) — o índice sobrevive à janela
      de scroll. Clique = seleção + a mesma ação do Enter (troca de sessão/modelo/tema;
      tema já ativo fecha o diálogo).
    - `ListActivatedMsg` do SimpleList é relayado pelo appModel ao command dialog
      (a seleção já foi movida pelo clique → `CommandSelectedMsg`, igual ao Enter).
    - Sidebar: título da sessão → `chat.ShowSessionListMsg` (abre o switcher) e arquivo
      modificado → `$EDITOR` via `tea.ExecProcess(EditorCommand(abs))` com path absoluto
      resolvido pela seam `workingDirectory`.
    - Testes: `mouse_test.go` (clickRow de session/model/theme), `simple-list_test.go`
      (`ListZoneID`/`ListClickedRow`/`clickRow`), `commands_test.go` (relay),
      `sidebar_test.go` (`clickZone`, seam de working directory).
  - **Session timeline + copy transcript (09o)** — feito:
    - `session_timeline` (`<leader>g`, igual ao TS) → `leaderTimeline` → `openTimelineDialog`:
      lista as **mensagens de usuário** da sessão ativa (mais recentes primeiro, com hora
      `15:04:05`), busca fuzzy (`/`), preview ao mover a seleção (`TimelinePreviewMsg` →
      `chat.JumpToMessageMsg`, o transcript rola atrás do diálogo) e Enter/clique →
      `OpenMessageActionsMsg` (troca o picker pelo Message Actions, Spec 09u). Esc fecha.
      Slash `/timeline` ("Jump to message").
    - `chat.JumpToMessageMsg` → `messagesCmp`: foca a mensagem e usa o `scrollToMessage`
      existente (offset = altura acumulada).
    - `session_copy` (default `none`, sem slash — igual ao TS): transcrição Markdown da
      sessão ativa → clipboard (`util.WriteClipboardText`) com toast; comando paleta
      "Copy Session Transcript" (`/copy` no TS é outra coisa — `messages_copy`).
    - Testes: `timeline_test.go` (itens user/newest-first, Enter/Esc, ListActivatedMsg,
      busca, preview), `list_test.go` (`TestJumpToMessage`), `leader_test.go`/
      `keybinds_test.go` (sufixo `g`, `session_copy` desabilitado).
  - **Session fork (09p)** — feito:
    - `session.Service.Fork(ctx, sessionID, before)`: cria uma sessão nova com cópia das
      mensagens **estrictamente antes** de `before` (`before == ""` → transcrição completa),
      remapeando ids (UUIDs novos) e preservando papel/parts/model/`created_at`/`updated_at`/
      `finished_at`. Mensagens in-flight (assistant sem `finished_at` — o `Finish` só grava a
      coluna no `Update`) não são copiadas. Fronteira desconhecida → erro; nada a copiar →
      `ErrSessionEmpty` (a sessão só é criada depois do corte e um erro no meio do copy a
      apaga via cascade — sem linha parcial). Título `"<título> (fork #N)"`, com N vindo de
      `CountSessionForks` (primeiro fork = 1).
    - Origem gravada em `fork_session_id` (**não** `parent_session_id`) via migration
      `20261022000000_add_fork_session_id.sql` + query `CreateForkSession`: assim o fork
      continua aparecendo no switcher (`ListSessions` filtra `parent_session_id IS NULL`) e
      as task sessions continuam ocultas. O `message_count` do fork é mantido pelo trigger
      de INSERT existente. sqlc regenerado (v1.31.1, só muda o comentário de versão).
    - TUI: `/fork` ("Fork Session") → `showForkMsg` → `openForkDialog` → diálogo
      `dialog/fork.go`: linha **"Full session"** primeiro (sem timestamp) + mensagens de
      usuário mais recentes primeiro (reusa o `TimelineItem` do 09o), busca fuzzy `/`,
      Enter/clique → `ForkMsg` (`MessageID` vazio = sessão completa). Enquanto o `Fork`
      assíncrono roda o diálogo mostra "Forking session…" e engole teclas/cliques
      (`SetPending`; `SetItems` limpa o estado ao reabrir). Sucesso → troca de sessão
      (`chat.SessionSelectedMsg`), toast "Forked session" e prefill do prompt com o texto da
      mensagem de fronteira (`chat.RestorePromptMsg`, só se o draft estiver vazio); falha →
      toast de erro e fecha. Guards: sem app/sessão ativa, sessão sem mensagens.
    - Keybind `session_fork` (default `none`, igual ao TS — `/fork` é a forma descobrível);
      quando configurado com `<leader>x` vira sufixo do which-key ("fork session") pela rota
      `leaderForkSession`.
    - `TimelineItem.Render` omite a hora quando `CreatedAt` é zero (linha "Full session").
    - Testes: `session_fork_test.go` (SQLite real com migrations: corte completo/por
      fronteira, in-flight excluído, ids remapeados, ordem e timestamps preservados,
      `fork_session_id`, contador via trigger, `ErrSessionEmpty` sem linha órfã, fronteira
      desconhecida, numeração `#1/#2`, evento `CreatedEvent`), `fork_test.go` (itens,
      Enter completo/por mensagem, Esc, clique, pending, busca, render sem timestamp),
      `keybinds_test.go`/`leader_test.go` (default desabilitado, sufixo `f` quando
      configurado).
  - **Prompt stash (09q)** — feito:
    - `internal/promptstash`: store limitado (MAX 50, LIFO) persistido em
      `<data.dir>/prompt-stash.jsonl` (um JSON por linha, reescrito a cada mutação — igual ao
      TS). Leitura tolerante: linhas malformadas ou sem timestamp são ignoradas e o formato
      da referência (`{"prompt":{"text":...},"timestamp":...}`) também é aceito (nosso
      writer usa `text`/`attachments`). Sem `data.dir` (testes) o store é só memória.
    - O stash vive no **editor** (`editorCmp.stash`, aberto no `NewEditorCmp`): msgs
      `chat.StashPushMsg` (exige texto não vazio e **limpa** composer, anexos e shellMode),
      `chat.StashPopMsg` (LIFO), `chat.RequestStashListMsg` → `chat.StashListMsg{Entries}`
      (ou warn quando vazio), `chat.StashTakeMsg{Index}` (sobrescreve e remove) e
      `chat.StashRemoveMsg{Index}`. Diferente de `RestorePromptMsg` (só preenche vazio),
      take/pop **sobrescrevem** o draft; take restaura anexos e shellMode (`!cmd`).
    - Diálogo `dialog/stash.go`: linhas newest-first com preview da 1ª linha (50 chars),
      idade relativa (`just now`/`Nm ago`/`Nh ago`/`Nd ago`/absoluta após 7 dias) e
      `~N lines` quando multiline; busca fuzzy `/`; Enter/clique → `StashSelectMsg` (fecha e
      restaura); `ctrl+d` em **dois passos** (arm → hint "press ctrl+d again to delete" com
      cor de erro → `StashDeleteMsg`; remove a linha e renumera os índices, diálogo fica
      aberto). Mover a seleção ou editar a busca desarma; Esc desarma antes de fechar.
      Adaptação consciente: o aviso de confirmação vai na linha de hint (o TS substitui o
      título da linha selecionada); a cor "destructive" do tema não existe aqui — `Error()`.
    - Comandos da paleta: `prompt_stash` ("Stash prompt"), `prompt_stash_pop` ("Stash pop"),
      `prompt_stash_list` ("Stash list") — os mesmos do TS. Keybinds
      `prompt_stash`/`prompt_stash_pop`/`prompt_stash_list` default `none` (igual ao TS);
      quando configurados com `<leader>x` viram sufixos do which-key
      ("stash prompt"/"pop stash"/"list stash") pela rota `leaderStash*`.
    - Testes: `promptstash/stash_test.go` (LIFO, trim em 50, roundtrip com anexos em disco,
      leitura tolerante e do formato TS, store em memória), `chat/stash_test.go`
      (push/pop/take/remove/guards via `Update`), `dialog/stash_test.go` (projeção e
      preview, tempo relativo, Enter/Esc/clique, dois passos do delete, desarm por
      movimento/busca, delete sob filtro, footer), `keybinds_test.go`/`leader_test.go`
      (defaults desabilitados, sufixos `i`/`o`/`p`).
  - **Clique/scroll em agents e plugins (09r)** — feito:
    - `dialog/agents.go`: zonas por linha com índice absoluto (`ad:`, helpers
      `ListZoneID`/`ListClickedRow`) + `selectAgent(idx)` compartilhado por Enter e clique —
      valida faixa, move a seleção e emite `AgentSelectedMsg`; clique fora de linha (ou sem
      zona renderizada) é engolido, sem cair na página. O wheel (traduzido em up/down) agora
      também move a seleção do switcher.
    - `dialog/plugins.go`: diálogo informativo (sem linhas acionáveis) consome `MouseMsg` para
      o clique não atravessar para a página atrás; Esc continua fechando.
    - `tui.go`: `forwardMouseClick` e a cadeia de wheel ganham os ramos
      `showAgentDialog`/`showPluginsDialog` antes do fallback da página.
    - Testes: `mouse_test.go` (`TestAgentDialogClickRow`, lista vazia, engolir
      clique/motion, `TestPluginsDialogConsumesMouse` com Esc preservado).
  - **Agrupamento do palette por contexto (09s)** — feito:
    - `dialog.Command` ganhou `Group` + `GroupTitle()`; os 22 built-ins recebem grupos no
      `New()` seguindo o vocabulário do TS (`Session`, `Prompt`, `Agent`, `System`,
      `Integration` — verificados em `app.tsx`/`session/index.tsx`/`prompt/index.tsx`);
      `RegisterCommand` promove grupo vazio para `"Custom"` (comandos do usuário).
    - `dialog.groupCommands`: buckets estáveis por grupo — a ordem é a primeira aparição de
      cada grupo e a ordem de registro é preservada dentro do bucket (o mesmo resultado do
      `groupBy` do TS). Chamado no `SetCommands`.
    - `SimpleList` ganhou `SetGroupHeaders(bool)` (opt-in, default off) + detecção de
      `interface{ GroupTitle() string }`: sem busca, o palette renderiza um cabeçalho por
      grupo (linha própria, `Primary`+bold, **sem zona** — clique no cabeçalho é engolido);
      com uma busca a lista achata sem cabeçalhos (igual ao `flatten` do TS). A janela deixa
      de contar itens e passa a contar **linhas** (linha + cabeçalho) quando os cabeçalhos
      estão ligados, para não estourar `maxVisibleItems` linhas.
    - Testes: `dialog/commands_test.go` (buckets estáveis, cabeçalhos ligados/desligados
      pela busca), `simple-list_test.go` (render com/sem header, orçamento de linhas com
      seleção no início e no meio, headers sem zonas), `internal/tui/commands_test.go`
      (`Custom` por omissão).
    - Adaptações conscientes: sem linha em branco entre grupos (o TS usa gap de 1 linha) e
      sem footer de categoria na linha filtrada; a ordem dos grupos segue a primeira
      aparição no registro nosso (o TS segue a ordem do Keymap).
  - **Codificação sixel real (09t)** — feito:
    - `internal/tui/util/sixel.go`: `RenderSixel(img, cols, rows)` emite um DCS
      `ESC P q "1;1;w;h ... ST` — paleta por cube 6×6×6 (até 216 entradas, cada nível
      convertido para percent `0/20/40/60/80/100` que o terminal mapeia de volta ao valor
      exato), bandas de 6 pixels, RLE (`!n<c>`) para corridas ≥ 4, `$`/`-` entre cores e
      bandas, pixels transparentes omitidos. Tamanho em pixels = células × 10×20 (mesma
      proporção 1:2 do half-block, células do terminal em geral ~10×20 — documentado como
      suposição; sem query de métricas da fonte).
    - `message.go`: `case util.ImageSixel` roteia anexos para `RenderSixel`, caindo para
      half-blocks se a imagem for vazia. Sequência pura, sem avanço de cursor — terminais
      com DECSDM ligado (default em xterm/foot/mlterm/WezTerm) já posicionam abaixo da
      imagem, igual ao tratamento do kitty.
    - Testes: `sixel_test.go` — golden byte-a-byte (sólido 10×20 = 4 bandas + RLE),
      entradas vazias/transparentes, stripes sem RLE, **round-trip** (decode do payload de
      volta a pixels e comparação com o quantizador, com verificação de largura exata de
      cada strip, sobreposição de cores e percents do palette) e contagem de bandas;
      `message_test.go` — roteamento por `TERM=xterm-sixel` vs fallback half-block.
    - Adaptação consciente: paleta de 216 cores (banding leve em fotos) em troca de
      resolução real do protocolo; escalonamento de texto/cursor é responsabilidade do
      terminal.
  - **Message Actions na timeline (09u)** — feito:
    - Enter/clique no timeline agora emitem `OpenMessageActionsMsg` → `openMessageActions`
      carrega a mensagem (`Messages.Get`) e troca o picker pelo novo diálogo
      `dialog/message.go` — o mesmo option set do `DialogMessage` ("Message Actions") do
      TS: **Jump to** ("view message in session"), **Revert** ("undo messages and file
      changes from this point"), **Copy** ("message text to clipboard") e **Fork**
      ("create a new session", só para mensagens de usuário, como no TS).
    - Jump → fecha e rola o transcript (`chat.JumpToMessageMsg`, o comportamento antigo do
      Enter). Revert → `revertToMessage` → `stageRevert` (mesma seam do `/undo`, com
      `restorePrompt` devolvendo o texto da mensagem de usuário ao composer).
    - Revert **stageia** o rollback como no TS: `stageRevert` remove a mensagem escolhida e
      todas as seguintes, captura o trabalho pós-turno (`snapshotAfter`), restaura o snapshot
      pré-turno, guarda o diff (`files`) e exibe o banner "◇ N messages reverted · M files
      rolled back · redo restores" acima do prompt. O estado (slot único global) permanece até
      `/redo` (clear: restaura mensagens + árvore) ou até um **novo prompt**, que emite
      `chat.CommitRevertMsg` e torna o rollback permanente antes de rodar o agent.
    - Copy → texto da mensagem no clipboard, com os mesmos toasts do `<leader>y`; Fork → abre o
      fork picker preselecionado na mensagem (`ForkDialog.SelectMessage` +
      `SimpleList.SelectIndex`).
    - Diálogo: `SimpleList[messageActionItem]` com título + descrição muted como rodapé;
      `SetMessage` zera a seleção (Jump to preselecionado) e desabilita Copy quando não
      há mensagem; Esc → fecha; clique/wheel idênticos aos demais diálogos (relay de
      `ListActivatedMsg`, ramos em `forwardMouseClick`, wheel, cadeia principal, View e
      `inChatContext`).
    - Testes: `message_test.go` (4 opções para usuário / 3 sem Fork para assistant,
      Enter=Jump, navegação até Fork, Esc, clique, reset da seleção, Enter ignorado sem
      mensagem), `timeline_test.go` (Enter/clique → `OpenMessageActionsMsg`),
      `fork_test.go` (`SelectMessage`), `simple-list_test.go` (`SelectIndex`).
    - Adaptações conscientes: o TS mantém `session.revert` **por sessão**; aqui o revert
      staged usa um **slot único global** (stagear em outra sessão avisa "Another session
      has a staged revert; clear it first"). O snapshot é **por turno de usuário** (o TS
      captura `snapshot.start`/`snapshot.files` por assistant message), o banner aparece
      **acima do prompt no editor** (não no transcript) e o Esc do actions fecha direto
      (não volta ao timeline).

  - **Working directory por sessão / session_move (09v)** — feito:
    - `sessions.working_directory` (migration `20261022000001_add_working_directory.sql`,
      `TEXT NOT NULL DEFAULT ''`; sqlc regenerado à mão no mesmo estilo v1.31.1). Vazio =
      diretório de lançamento (`config.WorkingDirectory()`). O fork herda o diretório do pai.
    - `session.SetWorkingDirectory` valida que o caminho existe e é um diretório, resolve
      para absoluto, persiste via `UpdateSession` e publica `UpdatedEvent`. Vazio reseta.
    - Contexto: `config.WithWorkingDirectory`/`config.WorkingDirectoryFromContext` carregam o
      diretório **por requisição**; `agent.processGeneration` aplica o diretório da sessão
      (ou o já presente no contexto, permitindo que subagentes herdem o cwd do pai). As tools
      (`bash`, `edit`, `write`, `patch`, `view`, `ls`, `glob`, `grep`, `fetch`) resolvem
      caminhos relativos por esse contexto. `bash` usa um shell persistente por diretório.
    - Snapshots: `App.SnapshotFor(dir)` cria/cacheia um `snapshot.Service` por diretório (o
      diretório de lançamento reusa o serviço do boot); `SnapshotContent.Directory` grava a
      qual repo o snapshot pertence e o revert usa o serviço certo. Reverter uma mensagem
      após mover a sessão desfaz só as mensagens (aviso no log), pois os trees vivem em repos
      diferentes.
    - TUI: keybind `session_move` (default `none`, igual ao TS v2 — `/move` é a forma
      descobrível; quando configurado vira sufixo do which-key "move session"); comando
      `/move` → `openSessionMoveDialog` (bloqueado com sessão ocupada) → diálogo
      `dialog/session_move.go`: textinput pré-preenchido com o diretório atual, `~`/`~/x`
      expandidos, caminhos relativos resolvidos contra o diretório atual, Enter aplica,
      Esc fecha. O header e o sidebar passam a mostrar/listar o diretório da sessão; o
      sidebar recarrega os arquivos modificados ao receber a sessão atualizada.
    - Testes: `session_fork_test.go` (`SetWorkingDirectory` válido/inválido/arquivo/reset,
      fork herda cwd), `dialog/session_move_test.go` (`ResolveDirectoryInput`, Enter/Esc),
      `ls_test.go` (tool respeita o cwd do contexto), `config_test.go`
      (`WithWorkingDirectory`/`WorkingDirectoryFromContext`), `keybinds_test.go` (default
      `none`).
    - Adaptações conscientes: LSP, watcher de arquivos, MCP/plugins e a linha "cwd:" do
      **system prompt** continuam ancorados no diretório de lançamento (o TS inicializa esses
      serviços por sessão). "Move session to worktree" do TS **não** foi implementado — só a
      troca de diretório (decisão do usuário).

  - **CLI de sessão (09w)** — feito:
    - `opencode [directory]` inicia no diretório informado (paridade com o TS; `--cwd`
      continua aceito, mas sem `-c`); `-c`/`--continue` retoma a sessão mais recente criada
      para esse diretório (sessões com `working_directory` vazio contam, pois seguem o
      diretório de lançamento). A sessão inicial é carregada no `Init` do TUI via
      `chat.SessionSelectedMsg`, com sidebar e transcript populados.
    - Testes: `cmd/root_test.go` (`lastSessionID`: projeto atual, outro projeto, legado,
      lista vazia/erro).
    - Pendente consciente: `-s/--session <id>` do TS (continuar/criar por ID) ainda não
      implementado.

- Pendências conscientes: `/undo`/`/redo` não reexecutam o agent (comportamento igual ao
  TS). Rollback usa snapshots de arquivos com as limitações listadas na Spec 09k (sem
  symlinks/modos, ignora arquivos ignorados pelo git). Worktrees por sessão ("Move session
  to worktree" do TS) não implementados — só troca de diretório.

## Riscos

- Mouse muda eventos: revisar handlers para não quebrar teclado.
- Paste de imagem é dependente de plataforma (X11/Wayland/macOS) — degradar graciosamente.
- Coordenar formato de keybinds com a spec 11 para não duplicar.
- O repo de snapshots ocupa disco proporcional ao tamanho do worktree semeado (uma cópia por
  projeto em `~/.config/opencode/snapshot/`); não há GC/limpeza automática ainda.
- Um capture copia todos os arquivos sujos do worktree a cada turno — repos muito grandes com
  muitos arquivos alterados podem adicionar latência ao envio do prompt.
- Após `/move`, LSP/diagnostics e o system prompt continuam apontando para o diretório de
  lançamento; tools e snapshots seguem o diretório da sessão.
