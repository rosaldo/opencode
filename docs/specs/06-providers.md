# Spec 06 — Providers: catálogo dinâmico, SDKs customizados e opções

Onda 2. Depende de: —. É pré-requisito de 12 (share/zen consome providers).

## Objetivo

Aproximar o modelo de providers do opencode TS: candidatos/configuração de provider declaráveis,
modelos descobertos dinamicamente, endpoints OpenAI-compatíveis customizados (`baseURL`, `headers`,
`npm`/SDK) e um "small model" separado. Hoje tudo é hard-coded.

## Estado atual (pós-fase 5)

- 11 providers + mock, por `switch` em `internal/llm/provider/provider.go:86`
  (`anthropic, azure, copilot, gemini, groq, local, bedrock, openai, openrouter, vertexai, xai`).
- Modelos 100% estáticos em `internal/llm/models/*.go` (`SupportedModels`, `models.go:50`),
  popularidade em `models.go:38-48`.
- Referências `provider/model`: `internal/llm/models/resolve.go:8`.
- `Provider` na config tem só `apiKey`/`disabled` (`internal/config/config.go`).
- Endpoint local descoberto via `LOCAL_ENDPOINT` (`internal/llm/models/local.go:24-57`).
- AUSENTE: catálogo `models.dev`, `baseURL`, `headers` por provider, catálogo de modelos por
  provider na config, `small_model`, OAuth de device-code/ChatGPT (codex), `apiKeyHelper`.

## Design

### Config: bloco `provider`

```jsonc
{
  "provider": {
    "openai": {
      "options": { "baseURL": "https://proxy.local/v1", "headers": { "X-Tenant": "acme" } }
    },
    "myprovider": {
      "npm": "@ai-sdk/openai-compatible",   // ou "openai-compatible"
      "options": { "baseURL": "https://api.myprovider.com/v1" },
      "models": {
        "my-model": { "name": "My Model", "limit": { "context": 128000, "output": 8192 } }
      }
    }
  },
  "model": "anthropic/claude-sonnet-4",
  "small_model": "anthropic/claude-haiku-4"
}
```

- `Config` ganha `Provider map[string]ProviderConfig` (json `provider`) — distinto do atual
  `Providers` (credenciais) — ou estende `Provider` com `Options`/`Models`. Decidir na fase
  (preferir estender `Provider` para não duplicar).
- `options.baseURL`/`options.headers` passam a ser repassados aos clientes (hoje hard-coded em
  `provider.go:119,137,149,157` e `openai.go:396-404`).
- `model`/`small_model` no topo da config substituem os defaults por provedor: `model` = primário
  default; `small_model` = titler/summarizer.

### Catálogo de modelos

- Introduzir um catálogo embutido (subset de `models.dev`) em `internal/llm/models` e permitir
  merge de modelos declarados na config (`provider.<p>.models`).
- `ResolveModelID` passa a considerar modelos declarados em config.

### Discovery/credenciais

- Manter env/API-key atual; adicionar ganchos:
  - `apiKeyHelper` (comando que imprime a key) opcional por provider.
  - Checagem de disponibilidade por `has*Credentials` reutilizada.
- (Fora de escopo desta fase: OAuth device-code/ChatGPT codex — vira spec futura se necessário.)

## Mudanças (arquivos)

- `internal/config/config.go`: `Provider.Options`, `Models`, topo `model`/`small_model`.
- `internal/llm/provider/*.go`: aceitar `baseURL`/`headers`/opções; abrir `switch` para
  providers OpenAI-compatíveis genéricos.
- `internal/llm/models/resolve.go` + catálogo de modelos.
- `cmd/schema/main.go` + `opencode-schema.json`: refletir as chaves novas (e corrigir a enum de
  providers, que hoje omite `copilot`/`xai`/`local`).

## Critérios de aceite

1. Config antiga carrega igual.
2. `provider.<p>.options.baseURL/headers` afetam as requisições do provider `<p>` (verificado por
   teste com servidor HTTP falso).
3. Provider OpenAI-compatível customizado funciona declarando `npm: openai-compatible` + baseURL.
4. `model`/`small_model` no topo mudam primário/titler sem tocar `agents.*`.
5. Modelo declarado em `provider.<p>.models` é aceito por `ResolveModelID`.
6. Schema valida as chaves novas.

## Testes

- `internal/llm/provider`: teste com `httptest.Server` verificando `baseURL` + header custom.
- `internal/config`: parse de `provider`, `model`, `small_model`.
- `internal/llm/models`: `ResolveModelID` com modelo de config.

## Riscos

- Não duplicar `Providers` (credenciais) com `provider` (config): desenhar bem o merge.
- Alguns SDKs (bedrock/vertex) não aceitam baseURL arbitrário — documentar limitações por provider.
- Escopo grande; dividir em 06a (config/options) e 06b (catálogo dinâmico) se necessário.
