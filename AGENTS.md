# AGENTS.md — opencode (fork Go)

## Especificações de trabalho

Este repositório usa specs em `docs/specs/`. **Leia o índice antes de começar:**

- `docs/specs/00-index.md` — objetivo, decisões de arquitetura, fases, mapa de arquivos e
  protocolo de retomada.

Fase atual e critérios de aceite estão no índice e no spec correspondente
(`docs/specs/01-…` a `docs/specs/05-…`).

## Regras

- Siga a ordem das fases e as dependências indicadas em cada spec.
- Ao concluir uma fase: `go build ./... && go test ./...` e atualize o **Status** em
  `docs/specs/00-index.md` (só marque `Verificado` com testes passando).
- Não altere a direção de dependências fixada no índice (novo pacote `internal/agentdef`).
- Mantenha back-compat com a config existente (`~/.opencode.json`, `<wd>/.opencode.json`).
