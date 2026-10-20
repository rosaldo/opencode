# Specs — Onda 2: paridade com o opencode TS (1.19.x)

Índice da segunda onda. **Leia o `00-index.md` primeiro** (onda 1: agentes dinâmicos, fases 1–5,
`Verificado`).

## Objetivo

Fechar as lacunas entre o fork Go (`v0.0.55`) e o opencode TS `1.19.x` nas áreas que **não** são
agentes: **providers, permissions, MCP, TUI, plugins, config e integração opencode.ai**.

Diagnóstico (inventário no HEAD pós-fase-5, ver seção "Estado atual" de cada spec):

| Área | Estado no Go | Spec |
|------|--------------|------|
| Providers | PARCIAL — 11 providers hard-coded, sem catálogo dinâmico nem `baseURL/headers/SDK custom` na config | [06](06-providers.md) |
| Permissions | PARCIAL — só por-agente; falta `permission` global, globs por recurso, `external_directory`, gating de read/glob/grep/webfetch | [07](07-permissions.md) |
| MCP | PARCIAL — stdio/sse; falta HTTP/streamable, resources/prompts, OAuth, ciclo de vida, gestão na TUI | [08](08-mcp.md) |
| TUI | PARCIAL — sem mouse, sem keybinds configuráveis, sem busca/rename/delete de sessão, sem paste de imagem | [09](09-tui.md) |
| Plugins | AUSENTE — nenhum mecanismo de plugin/hook | [10](10-plugins.md) |
| Config | PARCIAL — faltam `instructions`, `keybinds`, `share`, `autoupdate`, `permissions`, `formatters`; schema gerador desatualizado | [11](11-config.md) |
| opencode.ai | AUSENTE — sem share, sem serve/web, sem ACP, sem zen | [12](12-opencode-ai.md) |

## Ordem sugerida

Dependências leves; a ordem abaixo minimiza retrabalho:

1. **06 Providers** — base para tudo (modelos por provider, SDKs, `small_model`).
2. **11 Config** — estende o schema/config que as demais fases consomem (permission global,
   keybinds, instructions).
3. **07 Permissions** — depende de 11 (chave `permission` na config).
4. **08 MCP** — independente; depende de 11 só para `mcp` na config.
5. **09 TUI** — depende de 11 (keybinds) e toca 07 (prompt de permissão).
6. **10 Plugins** — depende de 11 (chave `plugin`) e de 08 (hooks de tool).
7. **12 opencode.ai** — por último; share/serve/web/ACP dependem de quase tudo.

## Convenções

- Seguir o protocolo do `00-index.md`: uma fase por spec, `go build ./... && go test ./...` ao
  concluir, e atualizar o **Status** no índice só com testes verdes.
- Manter a direção de dependências de `internal/agentdef` (nada de `agentdef → config`).
- Back-compat obrigatória: nenhuma chave existente muda de semântica.
- Documentar desvios na seção "Implementação (fase N)" de cada spec, como na onda 1.
