# AGENTS.md — opencode (fork Go)

## Especificações de trabalho

Este repositório usa specs em `docs/specs/`. **Leia o índice antes de começar:**

- `docs/specs/00-index.md` — objetivo, decisões de arquitetura, fases, mapa de arquivos e
  protocolo de retomada.

Fase atual e critérios de aceite estão no índice e no spec correspondente
(`docs/specs/01-…` a `docs/specs/13-…`; a onda 2 tem índice próprio em
`docs/specs/06-12-roadmap.md`).

## Regras

- Siga a ordem das fases e as dependências indicadas em cada spec.
- Ao concluir uma fase: `go build ./... && go test ./...` e atualize o **Status** em
  `docs/specs/00-index.md` (só marque `Verificado` com testes passando).
- Não altere a direção de dependências fixada no índice (novo pacote `internal/agentdef`).
- **Fonte única de agentes**: existe **um** `agentdef.Definition` e **um** `Registry`; toda forma
  de declarar agente (`.md` ou mapa `agents` do JSON) é uma *source* (`agentdef.Source`) que
  produz `Definition`. **Não** criar adapters/aliases nem duplicar chaves (ver Spec 13).
- Não adicionar teto estrutural ao número de agentes (escala ilimitada; prompt lazy).
- Preserve as características próprias: `team`, `hidden`, `disabled`, `color`, prompt lazy,
  delegação por nome e `@`-mention.
- Mantenha back-compat com a config existente (`~/.opencode.json`, `<wd>/.opencode.json`);
  `agent`/`defaultAgent` são lidos como *deprecados*, alimentando a mesma fonte única.
