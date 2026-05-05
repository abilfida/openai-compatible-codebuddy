# openai-compatible-codebuddy

Server API yang kompatibel dengan OpenAI, dibangun menggunakan [CodeBuddy Agent SDK](https://www.codebuddy.ai) (`@tencent-ai/agent-sdk`).

Setiap client yang menggunakan OpenAI SDK/API hanya perlu mengubah `base_url` untuk mengakses layanan model besar dari CodeBuddy.

## Memulai Cepat

### 1. Install Dependencies

```bash
npm install
```

### 2. Konfigurasi Environment Variables

```bash
cp .env.example .env
# Edit .env, masukkan CODEBUDDY_API_KEY Anda
```

### 3. Jalankan Service

```bash
npm run dev
```

Service default listen on `http://0.0.0.0:3000`.

## API Endpoint

### `POST /v1/chat/completions`

Endpoint chat completion yang kompatibel dengan OpenAI, mendukung streaming dan non-streaming.

```bash
# Non-streaming
curl http://localhost:3000/v1/chat/completions \
  -H "Content-Type: application/json" \
  -d '{
    "model": "deepseek-v3.1",
    "messages": [
      {"role": "system", "content": "You are a helpful assistant"},
      {"role": "user", "content": "Hello"}
    ]
  }'

# Streaming
curl http://localhost:3000/v1/chat/completions \
  -H "Content-Type: application/json" \
  -d '{
    "model": "deepseek-v3.1",
    "messages": [{"role": "user", "content": "Hello"}],
    "stream": true
  }'
```

### `GET /v1/models`

Mengembalikan daftar model yang tersedia.

```bash
curl http://localhost:3000/v1/models
```

### `GET /v1/models/:model`

Mengambil informasi model tunggal.

```bash
curl http://localhost:3000/v1/models/deepseek-v3.1
```

### `GET /health`

Health check.

```bash
curl http://localhost:3000/health
```

## Menggunakan OpenAI SDK

### Python

```python
from openai import OpenAI

client = OpenAI(
    base_url="http://localhost:3000/v1",
    api_key="not-needed"  # Authentication ditangani oleh CODEBUDDY_API_KEY di server
)

response = client.chat.completions.create(
    model="deepseek-v3.1",
    messages=[{"role": "user", "content": "Hello!"}]
)
print(response.choices[0].message.content)
```

### TypeScript/JavaScript

```typescript
import OpenAI from 'openai';

const client = new OpenAI({
  baseURL: 'http://localhost:3000/v1',
  apiKey: 'not-needed',
});

const response = await client.chat.completions.create({
  model: 'deepseek-v3.1',
  messages: [{ role: 'user', content: 'Hello!' }],
});
console.log(response.choices[0].message.content);
```

## Mekanisme Cache

### Double-Layer Cache

1. **SDK Cache Statistics Passthrough**: `cache_read_input_tokens` dari SDK di-map ke `usage.prompt_tokens_details.cached_tokens`
2. **Server-Side Request-Level Cache**: Hasil non-streaming untuk request yang sama (kombinasi model + messages) akan di-cache

### Konfigurasi Cache

| Environment Variable | Default | Deskripsi |
|---------|--------|------|
| `CACHE_ENABLED` | `true` | Enable/disable cache |
| `CACHE_TTL_MS` | `300000` | Cache TTL (milliseconds) |
| `CACHE_MAX_SIZE` | `100` | Maximum cache entries |

### Cache Response Headers

- `X-Cache: HIT` / `X-Cache: MISS` — Cache hit/miss
- `X-Cache-Stats` — Cache statistics (hit rate, etc.)

## Environment Variables

| Variable | Required | Default | Deskripsi |
|------|------|--------|------|
| `CODEBUDDY_API_KEY` | Yes | - | CodeBuddy API Key |
| `CODEBUDDY_INTERNET_ENVIRONMENT` | No | - | `internal` (China version) / `ioa` (iOA version) |
| `PORT` | No | `3000` | Service port |
| `HOST` | No | `0.0.0.0` | Listen address |
| `DEFAULT_MODEL` | No | `deepseek-v3.1` | Default model |
| `FALLBACK_MODEL` | No | `deepseek-v3.1` | Fallback model |

## Testing

```bash
# LRU cache unit tests
npm test

# Integration tests (requires running service)
npm run test:integration
```

## Tech Stack

- **Hono** — Lightweight high-performance web framework
- **@tencent-ai/agent-sdk** — CodeBuddy Agent SDK
- **TypeScript** — Type safety
- **Node.js >= 18.20**