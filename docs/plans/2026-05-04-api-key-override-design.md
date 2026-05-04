# API Key Override Design

## Overview

Add support for per-request API key override via HTTP header, allowing clients to provide their own CodeBuddy API key that takes precedence over the server's environment variable.

## Motivation

Fallback mechanism for resilience - clients can provide alternative API keys when the server's configured `CODEBUDDY_API_KEY` fails (expired, rate limited, invalid).

## Design

### Header-Based Approach

Clients pass API key via `X-CodeBuddy-Api-Key` header:

```
POST /v1/chat/completions
X-CodeBuddy-Api-Key: sk-xxx
Content-Type: application/json

{ "model": "deepseek-v3.1", "messages": [...] }
```

### Precedence Logic

```
request header key > env var CODEBUDDY_API_KEY
```

If header is provided, use it. Otherwise, fall back to env var.

### Implementation

**`sdk-bridge.ts` changes:**

```typescript
function buildEnv(requestApiKey?: string): Record<string, string | undefined> {
  const env: Record<string, string | undefined> = {};
  env.CODEBUDDY_API_KEY = requestApiKey ?? config.codebuddy.apiKey;
  if (config.codebuddy.environment) {
    env.CODEBUDDY_INTERNET_ENVIRONMENT = config.codebuddy.environment;
  }
  return env;
}

export async function chatCompletion(
  params: ChatCompletionParams,
  requestApiKey?: string
): Promise<ChatCompletionResponse>

export function chatCompletionStream(
  params: ChatCompletionParams,
  requestApiKey?: string
): ReadableStream<Uint8Array>

export async function getModels(requestApiKey?: string): Promise<ModelListResponse>
```

**`chat.ts` changes:**

```typescript
chatRoutes.post('/v1/chat/completions', async (c) => {
  const requestApiKey = c.req.header('X-CodeBuddy-Api-Key');
  // pass to chatCompletion() or chatCompletionStream()
});
```

**`models.ts` changes:**

```typescript
modelsRoutes.get('/v1/models', async (c) => {
  const requestApiKey = c.req.header('X-CodeBuddy-Api-Key');
  const modelList = await getModels(requestApiKey);
});

modelsRoutes.get('/v1/models/:model', async (c) => {
  const requestApiKey = c.req.header('X-CodeBuddy-Api-Key');
  // pass to getModels()
});
```

### Security

- Header is never logged (logger only logs request body)
- Key exists only in request scope, not persisted
- No additional validation - let SDK handle invalid keys naturally

### Files Modified

| File | Changes |
|------|---------|
| `src/services/sdk-bridge.ts` | Add `requestApiKey` param to functions |
| `src/routes/chat.ts` | Extract header, pass to SDK calls |
| `src/routes/models.ts` | Extract header, pass to SDK calls |
| `src/tests/integration.test.ts` | Add tests for header override |

### Testing

Integration tests should verify:
1. Request with header key works when env var is absent
2. Request with header key overrides env var when both present
3. Request without header falls back to env var