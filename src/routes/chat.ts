import { Hono } from 'hono';
import { config } from '../config.js';
import { chatCompletion, chatCompletionStream } from '../services/sdk-bridge.js';
import { formatError } from '../services/response-formatter.js';
import { LRUCache } from '../services/cache.js';
import type { ChatCompletionRequest, ChatCompletionResponse } from '../types/index.js';

export const chatRoutes = new Hono();

// Request-level cache for non-streaming completions
const completionCache = new LRUCache<ChatCompletionResponse>(
  config.cache.maxSize,
  config.cache.ttlMs
);

// POST /v1/chat/completions
chatRoutes.post('/v1/chat/completions', async (c) => {
  let body: ChatCompletionRequest;
  const requestApiKey = c.req.header('X-CodeBuddy-Api-Key');

  try {
    body = await c.req.json<ChatCompletionRequest>();
  } catch {
    return c.json(formatError('Invalid JSON body', 'invalid_request_error'), 400);
  }

  // Validate required fields
  if (!body.messages || !Array.isArray(body.messages) || body.messages.length === 0) {
    return c.json(
      formatError('messages is required and must be a non-empty array', 'invalid_request_error', null, 'messages'),
      400
    );
  }

  if (!body.model) {
    body.model = config.defaultModel;
  }

  const model = body.model;
  const messages = body.messages;
  const stream = body.stream ?? false;

  // ============ Streaming ============
  if (stream) {
    const readable = chatCompletionStream({ model, messages, stream: true }, requestApiKey);
    return new Response(readable, {
      headers: {
        'Content-Type': 'text/event-stream',
        'Cache-Control': 'no-cache',
        'Connection': 'keep-alive',
        'X-Accel-Buffering': 'no',
      },
    });
  }

  // ============ Non-streaming ============

  // Check cache
  if (config.cache.enabled) {
    const cacheKey = LRUCache.generateKey({
      model,
      messages,
      temperature: body.temperature,
      top_p: body.top_p,
      max_tokens: body.max_tokens,
    });

    const cached = completionCache.get(cacheKey);
    if (cached) {
      console.log(`[cache] HIT for model=${model}`);
      c.header('X-Cache', 'HIT');
      c.header('X-Cache-Stats', JSON.stringify(completionCache.stats));
      return c.json(cached);
    }

    // Cache miss — call SDK
    try {
      console.log(`[cache] MISS for model=${model}`);
      const result = await chatCompletion({ model, messages }, requestApiKey);
      completionCache.set(cacheKey, result);
      c.header('X-Cache', 'MISS');
      return c.json(result);
    } catch (error) {
      const msg = error instanceof Error ? error.message : 'Internal server error';
      return c.json(formatError(msg, 'server_error'), 500);
    }
  }

  // Cache disabled — direct call
  try {
    const result = await chatCompletion({ model, messages }, requestApiKey);
    return c.json(result);
  } catch (error) {
    const msg = error instanceof Error ? error.message : 'Internal server error';
    return c.json(formatError(msg, 'server_error'), 500);
  }
});

// Export cache for testing
export { completionCache };
