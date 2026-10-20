# Spec 06 — Providers v2: models/settings, endpoints e catálogo

Onda 2. Depende de: 11 (config v2). É referência para 12.

## Objetivo

Aproximar o modelo de providers do opencode v2: `providers.<id>.models` (declarar/sobrescrever
modelos), `providers.<id>.settings` (`apiKey`, `baseURL`, `headers`, `transport`), catálogo de
modelos conhecidos e seleção por topo `model`/`small_model`.

## Estado atual (pós-fase 5)

- 11 providers + mock por `switch` em `internal/llm/provider/provider.go:86` (`anthropic, azure,
  copilot, gemini, groq, local, bedrock, openai, openrouter, vertexai, xai`).
- `SupportedModels` estático (`internal/llm/models/models.go:50`); `ResolveModelID`
  (`internal/llm/models/resolve.go:8`).
- `Provider` na config só tem `apiKey`/`disabled`; baseURL/headers **hard-coded**
  (`provider.go:119,137,149,157`; `openai.go:396-404`).
- Endpoint local por `LOCAL_ENDPOINT` (`models/local.go:24-57`).
- AUSENTE (v2): `providers.<p>.models/settings/headers`, topo `model`/`small_model`, variantes,
  `transport: websocket`, catálogo de modelos.

## Design

### Config v2

```jsonc
{
  "model": "anthropic/claude-sonnet-4-5",
  "small_model": "anthropic/claude-haiku-4-5",
  "providers": {
    "openai": {
      "settings": { "apiKey": "{env:OPENAI_API_KEY}", "baseURL": "https://proxy/v1",
                    "headers": { "X-Tenant": "acme" } },
      "models": {
        "gpt-5.2-custom": {
          "modelID": "gpt-5.2",
          "name": "GPT-5.2 Custom",
          "limit": { "context": 200000, "output": 32000 }
        }
      }
    }
  }
}
```

- `Provider` v2 ganha `Settings` (apiKey/baseURL/headers/transport) e `Models`
  (`modelID`, `name`, `limit.{context,output}`, variantes). Manter `apiKey`/`disabled` atuais por
  compat (mapear para `settings.apiKey`).
- `baseURL`/`headers`/`settings` repassados aos SDKs (deixam de ser hard-coded).
- `model`/`small_model` no topo substituem os defaults por provedor (`model` = primário,
  `small_model` = titler/summarizer).
- `transport: "websocket"` (settings) seleciona transporte WebSocket quando suportado.

### Catálogo de modelos

- Catálogo embutido (subset de `models.dev`) + merge de `providers.<p>.models` na config.
- `ResolveModelID` considera modelos declarados; `limit.context/output` alimentam MaxTokens/context.

## Mudanças (arquivos)

- `internal/config/config.go`: `Provider.Settings/Models`; topo `model`/`small_model`.
- `internal/llm/provider/*.go`: aceitar settings; abrir genérico OpenAI-compatível.
- `internal/llm/models/resolve.go` + catálogo.
- `cmd/schema/main.go` + `opencode-schema.json`.

## Critérios de aceite

1. Config atual carrega igual.
2. `providers.<p>.settings.baseURL/headers` afetam as requisições (teste `httptest`).
3. Provider OpenAI-compatível custom funciona por `settings.baseURL` (+ `npm`/pacote se adotado).
4. `model`/`small_model` mudam primário/titler sem tocar `agents`.
5. Modelo de `providers.<p>.models` é aceito por `ResolveModelID`; `limit` afeta tokens.
6. Schema valida as chaves novas.

## Testes

- `internal/llm/provider`: `httptest.Server` verificando baseURL + header.
- `internal/config`: parse de `providers.<p>.models/settings`, `model`, `small_model`.
- `internal/llm/models`: `ResolveModelID` com modelo de config.

## Riscos

- Não confundir `providers` (credenciais atuais) com o v2 (models/settings): unificar com migração.
- SDKs bedrock/vertex não aceitam baseURL arbitrário — documentar por provider.
- `transport: websocket` depende de suporte por SDK — tratar como opcional.
- Escopo: dividir 06a (settings/endpoints) e 06b (catálogo/variantes).
