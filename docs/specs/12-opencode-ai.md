# Spec 12 — Integração opencode.ai: share, serve/web, ACP e zen

Onda 2. Depende de: 06 (providers), 11 (config `share`), 09 (TUI). Última fase; opcional/tardia.

## Objetivo

Aproximar a experiência "opencode" que não existe no fork Go: **share** de conversas
(`opncd.ai/s/<id>`), **serve/web** (UI via navegador), **ACP** (Agent Client Protocol) e possíveis
pontos de integração com a nuvem (`zen`/modelos hospedados).

## Estado atual

- **AUSENTE.** Sem share, sem servidor HTTP/serve, sem web UI, sem ACP. O fork Go é 100% local
  (TUI + print mode). Não há integração com `opencode.ai`.

## Design

### Share

- `share` na config (`manual|auto|disabled`) — parse na spec 11.
- Comandos `/share` e `/unshare`; upload do histórico da sessão para um endpoint e geração de link.
- **Tensão**: o fork Go é local e o backend de share do TS é deles. Opções:
  - (a) integração com a API oficial do opencode.ai (exige endpoint/auth pública);
  - (b) **share self-hosted**: `opencode serve` sobe um servidor local que expõe a sessão por
    link; sem dependência de nuvem de terceiros.
- Recomendação: implementar **(b) share/serve local** primeiro (útil e sem dependência externa) e
  tratar (a) só se houver endpoint oficial estável. Documentar o que é possível.

### Serve / Web

- `opencode serve`: servidor HTTP (SSE/WebSocket) que expõe sessões e streaming de eventos;
  possível web UI mínima consumindo essa API.
- Reaproveitar o broker/pubsub (`internal/pubsub`, `AgentManager`) como fonte de eventos.

### ACP (Agent Client Protocol)

- Implementar o ACP para integração com editores (Zed etc.): o processo fala JSON-RPC por stdio
  expondo as capacidades de agente/sessão.

### Zen / modelos hospedados

- Só se houver API documentada; caso contrário, marcar como **fora de escopo** e manter providers
  via spec 06 (incl. endpoints OpenAI-compatíveis apontando para serviços hospedados).

## Mudanças (arquivos)

- Novo `cmd/serve.go` (comando `serve`) + `internal/server`: API HTTP/SSE + auth local.
- Novo `internal/acp`: servidor ACP (JSON-RPC stdio).
- `internal/tui`: comandos `/share`/`/unshare` (se share local), indicação de status.
- `internal/config`: `share` (parse na 11).
- `opencode-schema.json`: `share`.

## Critérios de aceite

1. `opencode serve` sobe um servidor local que expõe sessões e streaming de eventos (teste com
   cliente HTTP).
2. `/share` gera um link local acessível; `/unshare` invalida.
3. `share: disabled` bloqueia share; `auto` compartilha novas sessões.
4. Servidor ACP responde handshake e uma sessão mínima (teste com cliente fake).
5. Sem config de share, comportamento inalterado (só local).
6. Endpoints são **locais por padrão** (bind 127.0.0.1) e exigem token para acesso remoto.

## Testes

- `internal/server`: rotas principais, auth, streaming SSE.
- `internal/acp`: handshake + uma troca de mensagem.
- Share: geração/invalidação de link (backend local).

## Status

- **12a serve/share local** — Verificado. Pacote **novo** `internal/server`: API HTTP com
  `GET /api/health`, `GET /api/sessions`, `GET /api/sessions/{id}`,
  `GET /api/sessions/{id}/messages`, `GET /api/events` (SSE via broker) e
  `POST/GET/DELETE /api/share[/{id}]` (share **self-hosted** local, opção (b): links em memória,
  `URL` aponta para o próprio servidor). Bind **loopback por padrão** (`127.0.0.1:0`); bind
  não-loopback **exige token** (recusa sem ele); auth por `Authorization: Bearer` ou `?token=`.
  Comando `opencode serve` (`cmd/serve.go`) liga os brokers de sessão/mensagem ao stream SSE.
  `Config.Share` (`manual|auto|disabled`, default `disabled`) parseado; schema emite `share`.
  Testes: `internal/server/server_test.go` (health, auth, sessões/mensagens, SSE, share
  create/get/revoke, não-loopback sem token), `internal/config/share_test.go`.
- **12c ACP** — Verificado. Pacote **novo** `internal/acp`: JSON-RPC 2.0 newline-delimited sobre
  stdio; métodos `initialize` (handshake com `protocolVersion: 1`), `session/new`, `session/prompt`
  (streaming via notificações `session/update` + resposta `stopReason`), `session/cancel`; método
  desconhecido → erro JSON-RPC. Comando `opencode acp` (`cmd/acp.go`) adapta `agent.Service`
  (`AgentEvent` → chunks de texto). Testes: `internal/acp/acp_test.go` (handshake, prompt com
  streaming, sessão desconhecida, cancel, método desconhecido).
- **12b web UI** — Verificado. `internal/server/ui/index.html` (embutido via `go:embed`) servido em
  `/` e `/ui` (`handleUI`; demais paths → 404). A página é autossuficiente (sem build): lista
  sessões (`/api/sessions`), carrega mensagens (`/api/sessions/{id}/messages`) e assina o stream
  `/api/events` (SSE). Quando há token configurado, lê `?token=` da própria URL e o repassa nas
  chamadas; `opencode serve` imprime a URL do web UI. Testes: `internal/server/server_test.go`
  (`TestUIPageServed`).
- **12d zen/modelos hospedados** — Verificado. Provider nativo **`opencode`** (`internal/llm/models/opencode.go`)
  apontando para o gateway OpenAI-compatível `https://opencode.ai/inference/openai/v1`
  (`internal/llm/provider/provider.go`), API key via `OPENCODE_API_KEY` (ou
  `providers.opencode.settings.apiKey`). Catálogo curado dos modelos publicados em
  `opencode.ai/docs/zen` (GPT‑5.x, Claude via gateway, GLM, Kimi, Qwen, DeepSeek, MiniMax,
  Grok, Mistral e os *free*). Seleção por `opencode/<model-id>` (ex.: `opencode/gpt-5.5`),
  igual ao TS, resolvida por `ResolveModelID`. Usuários podem declarar/sobrescrever modelos
  em `providers.opencode.models` (Spec 06b). Comando **`/connect`** na TUI abre um diálogo para
  colar a API key (`config.SetProviderAPIKey`, persistida em `~/.opencode.json`). **Default
  amigável**: sem nenhuma credencial, o modelo cai no tier grátis do Zen (`opencode/big-pickle`
  para o coder; GLM Flash Free/MiMo Free para os agentes auxiliares), e o provider `opencode` é
  registrado e mantido habilitado mesmo sem key. Testes:
  `internal/llm/provider/opencode_test.go` (base URL do gateway e resolução `opencode/<id>`),
  `internal/config/connect_test.go` (persistência da key e default grátis),
  `internal/tui/components/dialog/connect_test.go` (diálogo).
- Sem config de share, comportamento inalterado (só local) — critério 5 atendido.

## Riscos

- **Escopo e expectativa**: paridade real com `opencode.ai` depende de APIs oficiais indisponíveis
  — manter a fase honesta sobre o que é local vs. nuvem.
- Segurança de um servidor HTTP que expõe sessões/segredos — bind local + auth obrigatória.
- Grande; dividir em 12a (serve/share local), 12b (web UI), 12c (ACP), 12d (zen, se viável).
