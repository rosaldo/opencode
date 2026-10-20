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

## Riscos

- Mouse muda eventos: revisar handlers para não quebrar teclado.
- Paste de imagem é dependente de plataforma (X11/Wayland/macOS) — degradar graciosamente.
- Coordenar formato de keybinds com a spec 11 para não duplicar.
