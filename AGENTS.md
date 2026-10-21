# AGENTS.md — opencode (Go fork)

## Working specifications

This repository uses specs under `docs/specs/`. **Read the index before starting:**

- `docs/specs/00-index.md` — goal, architecture decisions, phases, file map and
  resume protocol.

The current phase and acceptance criteria live in the index and in the matching spec
(`docs/specs/01-…` through `docs/specs/13-…`; wave 2 has its own index at
`docs/specs/06-12-roadmap.md`).

## Rules

- Follow the phase order and the dependencies stated in each spec.
- When a phase is done: `go build ./... && go test ./...` and update the **Status** in
  `docs/specs/00-index.md` (only mark `Verificado` with passing tests).
- Do not change the dependency direction fixed in the index (new package `internal/agentdef`).
- **Single source of agents**: there is **one** `agentdef.Definition` and **one** `Registry`; every
  way of declaring an agent (a `.md` file or the JSON `agents` map) is a *source* (`agentdef.Source`)
  that produces a `Definition`. Do **not** create adapters/aliases or duplicate keys (see Spec 13).
- Do not add a structural ceiling to the number of agents (unlimited scale; lazy prompt).
- Preserve the native features: `team`, `hidden`, `disabled`, `color`, lazy prompt,
  delegation by name and `@`-mention.
- Keep backward compatibility with the existing config (`~/.opencode.json`, `<wd>/.opencode.json`);
  `agent`/`defaultAgent` are read as *deprecated*, feeding the same single source.
