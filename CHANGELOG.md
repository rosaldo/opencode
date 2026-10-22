# Changelog

> Generated from `git` by `scripts/gen-changelog.sh` — do not edit by hand.

- **v1.14.0** (2026-10-22) · `feat` — tui: mouse click hit-testing for list dialogs and the sidebar (09n)
- **v1.13.0** (2026-10-22) · `feat` — tui: editor_open keybind (<leader>e) and tool_details id wired to the leader (09m)
- **v1.12.0** (2026-10-22) · `feat` — tui: /details collapses tool call outputs to name+params (09l)
- **v1.11.0** (2026-10-22) · `feat` — tui: /undo /redo without git rollback (09k)
- **v1.10.0** (2026-10-22) · `feat` — tui: messages_copy keybind copies the focused message to the clipboard (09j)
- **v1.9.0** (2026-10-22) · `feat` — tui: /thinking command and display_thinking keybind to hide reasoning blocks (09i)
- **v1.8.0** (2026-10-22) · `feat` — tui: builtin slash commands /new /sessions /models /themes /help /editor /exit with aliases (09h)
- **v1.7.0** (2026-10-22) · `feat` — tui: /export command and session_export keybind (09g)
- **v1.6.0** (2026-10-21) · `feat` — tui: prompt history with configurable history_previous/next keys (09f)
- **v1.5.0** (2026-10-21) · `feat` — tui: leader which-key popup and configurable leader timeout (tui.leader_timeout, 09d)
- **v1.4.0** (2026-10-21) · `feat` — tui: fuzzy / search in the command dialog and configurable prompt_paste keybind (09d)
- **v1.3.0** (2026-10-21) · `feat` — tui: render collapsible thinking blocks (09e) and make leader key sequences config-driven (09d)
- **v1.2.0** (2026-10-21) · `feat` — tui: wire leader key sequences (session_new/rename/delete/compact/sidebar)
- **v1.1.0** (2026-10-21) · `feat` — tui: add ctrl+p command palette keybind, coexists with ctrl+k
- **v1.0.6** (2026-10-21) · `fix` — tui parity: / slash commands and ! shell passthrough in editor (spec 09c)
- **v1.0.5** (2026-10-20) · `fix` — zen free tier: force stream:true for one-shot calls (gate rejects stream:false, broke title generation), add provider diagnostics logging, default title model to GLM Flash Free
- **v1.0.4** (2026-10-20) · `fix` — add tests for message/session/db/fileutil/version/provider and fix 3 bugs they revealed (image parts dropped on read, glob limit erroring, nil config on debug path)
- **v1.0.3** (2026-10-20) · `fix` — fix Zen provider free tier: correct base URL to zen/v1, add client fingerprint (session, UA, required tools), refresh free model catalog
- **v1.0.2** (2026-10-20) · `fix` — clean --version output (no stray warnings, plain X.Y.Z) and English-only user-facing text
- **v1.0.1** (2026-10-20) · `fix` — release workflow: accept a tag via workflow_dispatch input and resolve it for both triggers
- **v1.0.0** (2026-10-20) · `major` — first release of the opencode Go fork (TS v2 parity)
