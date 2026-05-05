# Go + Fiber Rewrite Design

Rewrite the OpenAI-compatible CodeBuddy API server from TypeScript/Hono to Go/Fiber.

## Overview

**Goal**: Create a Go version of the existing TypeScript API server with better performance and simpler deployment (single binary).

**Approach**: Spawn CodeBuddy CLI subprocess and communicate via stdin/stdout JSON messages (same mechanism as TypeScript SDK).

**Output location**: `go/` directory, mirroring TypeScript structure.

---

## Project Structure & Dependencies

### Directory Layout

```
go/
├── cmd/
│   └── server/
│       └── main.go          # Entry point, Fiber app setup
├── internal/
│   ├── config/
│   │   └── config.go        # Environment configuration
│   ├── routes/
│   │   ├── chat.go          # /v1/chat/completions handler
│   │   ├── models.go        # /v1/models handler
│   │   └── health.go        # /health handler
│   ├── services/
│   │   ├── sdk-bridge.go    # CLI subprocess management
│   │   ├── message-converter.go  # OpenAI → CLI message format
│   │   ├── response-formatter.go # CLI → OpenAI response format
│   │   └── cache.go         # LRU cache implementation
│   ├── middleware/
│   │   ├── error-handler.go # Error handling middleware
│   │   └── logger.go        # Request logging middleware
│   ├── types/
│   │   ├── openai.go        # OpenAI API types
│   │   └── cli.go           # CLI JSON message types
│   └── utils/
│       └── auth.go          # Header extraction utilities
├── go.mod
├── go.sum
├── Dockerfile
└── .env.example
```

### Dependencies

- `github.com/gofiber/fiber/v2` - Web framework
- `github.com/gofiber/fiber/v2/middleware/cors` - CORS middleware
- Standard library: `os/exec`, `encoding/json`, `bufio`, `context`, `sync`, `crypto/sha256`

### Go Version

Target: **Go 1.22+** (runtime optimizations, good stability)

---

## CLI Subprocess Communication

### Process Spawning

Execute CodeBuddy CLI binary with stdin/stdout pipes.

**Binary resolution order**:
1. `CODEBUDDY_CODE_PATH` environment variable (user override)
2. Default: look for `codebuddy` in PATH or relative paths

**Spawn arguments**:
```go
args := []string{
    "--output-format", "stream-json",
    "--verbose",
    "--input-format", "stream-json",
    "--setting-sources", "none",
    "--model", model,
    "--fallback-model", fallbackModel,
    "--max-turns", "1",
    "--permission-mode", "bypassPermissions",
    "--allowedTools", "",  // Empty = no tools
    "--system-prompt", systemPrompt,
}
// For streaming: add "--include-partial-messages"
```

**Environment variables passed to CLI**:
- `CODEBUDDY_API_KEY` - From request header or server config
- `CODEBUDDY_INTERNET_ENVIRONMENT` - From server config (optional)
- `CODEBUDDY_CODE_ENTRYPOINT=sdk-go` - SDK entrypoint marker

### Message Protocol

JSON lines over stdin/stdout.

**stdin → CLI (user message)**:
```json
{"type":"user","session_id":"","message":{"role":"user","content":"Hello"},"parent_tool_use_id":null}
```

For multimodal (images):
```json
{"type":"user","session_id":"","message":{"role":"user","content":[{"type":"text","text":"What's this?"},{"type":"image","source":{"type":"base64","media_type":"image/png","data":"..."}}]},"parent_tool_use_id":null}
```

**stdout ← CLI (streaming events)**:
```json
{"type":"stream_event","event":{"type":"message_start","message":{"id":"...","model":"deepseek-v3.1"}}}
{"type":"stream_event","event":{"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":"Hi"}}}
{"type":"stream_event","event":{"type":"message_delta","delta":{"stop_reason":"end_turn"},"usage":{"output_tokens":5}}}
{"type":"result","usage":{"input_tokens":10,"output_tokens":5}}
```

**stdout ← CLI (non-streaming result)**:
```json
{"type":"assistant","message":{"role":"assistant","content":[{"type":"text","text":"Hello!"}],"usage":{"input_tokens":10,"output_tokens":5}}}
{"type":"result","usage":{"input_tokens":10,"output_tokens":5}}
```

**stdout ← CLI (control request/response for models)**:
```json
// Request (from CLI)
{"type":"control_request","request_id":"...","request":{"subtype":"get_supported_models"}}
// Response (from Go server)
{"type":"control_response","response":{"subtype":"success","request_id":"...","response":{"models":[{"id":"deepseek-v3.1","name":"DeepSeek V3.1"}]}}}
```

### Go Implementation

```go
type CLIProcess struct {
    cmd     *exec.Cmd
    stdin   io.WriteCloser
    stdout  io.Reader
    scanner *bufio.Scanner
    env     map[string]string
}

func (p *CLIProcess) Start() error
func (p *CLIProcess) SendUserMessage(msg UserMessage) error
func (p *CLIProcess) Messages() <-chan Message  // Channel-based streaming
func (p *CLIProcess) Close() error
```

---

## API Handlers & Request/Response Flow

### Endpoints

| Endpoint | Method | Handler | Description |
|----------|--------|---------|-------------|
| `/v1/chat/completions` | POST | `routes.ChatHandler` | Chat completions (streaming + non-streaming) |
| `/v1/models` | GET | `routes.ModelsHandler` | List all models |
| `/v1/models/:model` | GET | `routes.ModelHandler` | Get single model info |
| `/health` | GET | `routes.HealthHandler` | Health check |

### Chat Handler Flow

1. Parse request body → `types.ChatCompletionRequest`
2. Extract `X-CodeBuddy-Api-Key` header (optional override)
3. Check cache (non-streaming only)
   - Cache key: SHA256 hash of `{model, messages}` JSON
   - If hit: return cached response + `X-Cache: HIT` header
4. Convert messages → CLI prompt (`services.ConvertMessages`)
5. Spawn CLI subprocess with options
6. Send user message to CLI stdin
7. Read stdout messages:
   - Non-streaming: collect all text, build response
   - Streaming: convert each `content_block_delta` to SSE chunk
8. Format response → OpenAI format (`services.FormatResponse`)
9. Cache result (non-streaming only)
10. Return response + `X-Cache: MISS` header

### SSE Streaming (Fiber)

```go
func streamChatCompletion(c *fiber.Ctx, cli *CLIProcess) error {
    c.Set("Content-Type", "text/event-stream")
    c.Set("Cache-Control", "no-cache")
    c.Set("Connection", "keep-alive")

    c.Context().SetBodyStreamWriter(func(w *bufio.Writer) {
        for msg := range cli.Messages() {
            if msg.Type == "stream_event" {
                chunk := formatSSEChunk(msg)
                fmt.Fprintf(w, "data: %s\n\n", chunk)
                w.Flush()
            }
        }
        fmt.Fprintf(w, "data: [DONE]\n\n")
        w.Flush()
    })
    return nil
}
```

### Models Handler Flow

1. Check models cache (5 minute TTL)
2. If expired: spawn CLI, send control request `get_supported_models`
3. Wait for control response
4. Parse model list, format to OpenAI `ModelListResponse`
5. Cache result
6. Abort CLI process (only needed model list)
7. Return response

### Request/Response Types

```go
// types/openai.go
type ChatCompletionRequest struct {
    Model    string        `json:"model"`
    Messages []ChatMessage `json:"messages"`
    Stream   bool          `json:"stream,omitempty"`
}

type ChatMessage struct {
    Role    string      `json:"role"`
    Content interface{} `json:"content"` // string or []ContentPart
}

type ContentPart struct {
    Type     string    `json:"type"` // "text" or "image_url"
    Text     string    `json:"text,omitempty"`
    ImageURL *ImageURL `json:"image_url,omitempty"`
}

type ChatCompletionResponse struct {
    ID      string   `json:"id"`
    Object  string   `json:"object"`
    Created int64    `json:"created"`
    Model   string   `json:"model"`
    Choices []Choice `json:"choices"`
    Usage   Usage    `json:"usage"`
}
```

---

## Caching, Error Handling & Middleware

### LRU Cache Implementation

```go
// services/cache.go
type CacheEntry[V any] struct {
    Value     V
    ExpiresAt time.Time
}

type LRUCache[K comparable, V any] struct {
    maxSize int
    ttl     time.Duration
    entries map[K]*list.Element
    order   *list.List
    mu      sync.RWMutex
    stats   CacheStats
}

func (c *LRUCache[K, V]) Get(key K) (V, bool)
func (c *LRUCache[K, V]) Set(key K, value V)
func (c *LRUCache[K, V]) Stats() CacheStats
```

**Cache behavior**:
- Max size: 100 entries (configurable via `CACHE_MAX_SIZE`)
- TTL: 5 minutes (configurable via `CACHE_TTL_MS`)
- Eviction: LRU when size exceeded
- Expiration: checked on Get(), expired entries not returned
- Thread-safe: RWMutex for concurrent access

**Cache headers**:
- `X-Cache: HIT` / `X-Cache: MISS`
- `X-Cache-Stats: {"hits":10,"misses":5,"size":20,"hit_rate":0.67}`

### Error Handling Middleware

```go
// middleware/error-handler.go
func ErrorHandler(c *fiber.Ctx, err error) error {
    code := fiber.StatusInternalServerError
    var errMsg string

    if e, ok := err.(*fiber.Error); ok {
        code = e.Code
        errMsg = e.Message
    } else {
        errMsg = err.Error()
    }

    return c.Status(code).JSON(ErrorResponse{
        Error: ErrorDetail{
            Message: errMsg,
            Type:    "invalid_request_error",
            Param:   nil,
            Code:    nil,
        },
    })
}
```

**Error response format**:
```json
{
  "error": {
    "message": "Model not found",
    "type": "invalid_request_error",
    "param": "model",
    "code": "model_not_found"
  }
}
```

### Logger Middleware

```go
// middleware/logger.go
func Logger(cfg *config.Config) fiber.Handler {
    return func(c *fiber.Ctx) error {
        start := time.Now()
        err := c.Next()
        latency := time.Since(start)

        if cfg.LogLevel == "debug" {
            // Verbose: log request body and response
            log.Printf("[%s] %s %s %d %v\nbody=%s\nresponse=%s",
                start.Format(time.RFC3339),
                c.Method(), c.Path(), c.Response().StatusCode(),
                latency, c.Body(), c.Response().Body())
        } else {
            log.Printf("[%s] %s %s %d %v",
                start.Format(time.RFC3339),
                c.Method(), c.Path(), c.Response().StatusCode(),
                latency)
        }
        return err
    }
}
```

**Log levels**: `debug`, `info`, `warn`, `error`

### Auth Utilities

```go
// utils/auth.go
func ExtractAPIKey(c *fiber.Ctx) string {
    return c.Get("X-CodeBuddy-Api-Key")
}
```

---

## Testing, Docker & Deployment

### Testing Strategy

**Unit tests** (no server required):
- `cache_test.go` - LRU operations (Get/Set/Delete/Eviction/TTL/Concurrency)
- `message_converter_test.go` - Message format conversion
- `response_formatter_test.go` - Response formatting

**Integration tests** (requires running server + CodeBuddy CLI):
- `integration_test.go` - Full API flow
  - Non-streaming chat completion
  - Streaming chat completion (SSE parsing)
  - Models endpoint
  - Health endpoint
  - Cache headers verification
  - X-CodeBuddy-Api-Key header override
  - Error handling (invalid request, model not found)

### Dockerfile

```dockerfile
# Build stage
FROM golang:1.22-alpine AS builder
WORKDIR /app
COPY go/go.mod go/go.sum ./
RUN go mod download
COPY go/ ./
RUN go build -o server ./cmd/server

# Runtime stage
FROM alpine:3.19
RUN apk --no-cache add ca-certificates
COPY --from=builder /app/server /server

# Note: CodeBuddy CLI must be available in runtime environment
# Either: install via package manager, or mount from host, or bundle

EXPOSE 3000
ENV PORT=3000
ENV HOST=0.0.0.0

CMD ["/server"]
```

### Build & Run

```bash
# Local build
cd go
go mod tidy
go build -o codebuddy-server ./cmd/server
./codebuddy-server

# With environment
export CODEBUDDY_API_KEY=your-key
./codebuddy-server

# Docker build
docker build -f go/Dockerfile -t codebuddy-go .
docker run -p 3000:3000 -e CODEBUDDY_API_KEY=xxx codebuddy-go

# Tests
go test ./...              # All tests
go test ./internal/services -run TestCache  # Specific test
```

### Environment Variables

Same as TypeScript version:

| Variable | Required | Default | Description |
|----------|----------|---------|-------------|
| `CODEBUDDY_API_KEY` | Yes | - | CodeBuddy API Key |
| `CODEBUDDY_INTERNET_ENVIRONMENT` | No | - | `internal` or `ioa` |
| `PORT` | No | `3000` | Server port |
| `HOST` | No | `0.0.0.0` | Listen address |
| `DEFAULT_MODEL` | No | `deepseek-v3.1` | Default model |
| `FALLBACK_MODEL` | No | `deepseek-v3.1` | Fallback model |
| `CACHE_ENABLED` | No | `true` | Enable cache |
| `CACHE_TTL_MS` | No | `300000` | Cache TTL (ms) |
| `CACHE_MAX_SIZE` | No | `100` | Max cache entries |
| `LOG_LEVEL` | No | `info` | Log level |
| `CODEBUDDY_CODE_PATH` | No | - | CLI binary path override |

---

## Implementation Order

Recommended implementation sequence:

1. **Foundation** - `go.mod`, `config.go`, `types/openai.go`, `types/cli.go`
2. **CLI Bridge** - `services/sdk-bridge.go` (subprocess spawning, message protocol)
3. **Core handlers** - `routes/health.go`, `routes/models.go`
4. **Chat handler** - `routes/chat.go`, `services/message-converter.go`, `services/response-formatter.go`
5. **Middleware** - `middleware/error-handler.go`, `middleware/logger.go`
6. **Caching** - `services/cache.go`, integrate into chat handler
7. **Auth** - `utils/auth.go`, integrate into handlers
8. **Entry point** - `cmd/server/main.go`
9. **Testing** - Unit tests, integration tests
10. **Docker** - `Dockerfile`, build verification

---

## Key Differences from TypeScript

| Aspect | TypeScript | Go |
|--------|------------|-----|
| Framework | Hono | Fiber |
| Runtime | Node.js | Native binary |
| SDK | `@tencent-ai/agent-sdk` npm package | Direct CLI subprocess |
| Async model | async/await, AsyncIterable | Channels, goroutines |
| Types | TypeScript interfaces | Go structs |
| Error handling | try/catch | panic/recover, error returns |
| Deployment | Node + npm install | Single binary |