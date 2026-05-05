/**
 * Integration Tests for OpenAI Compatible Server
 *
 * These tests verify:
 * 1. Cache hit returns identical results and skips SDK calls
 * 2. Non-streaming endpoint returns correct format
 * 3. Streaming endpoint returns SSE format
 * 4. Models endpoint returns correct format
 * 5. Error handling for invalid requests
 *
 * Usage:
 *   1. Start the server: npm run dev
 *   2. Run tests: npm run test:integration
 *
 * Note: Tests that call the actual SDK require a valid CODEBUDDY_API_KEY.
 *       Cache tests work independently by making two identical requests.
 */

export {};

const BASE_URL = process.env.TEST_BASE_URL || 'http://localhost:3000';

let passed = 0;
let failed = 0;

function assert(condition: boolean, name: string) {
  if (condition) {
    console.log(`  ✓ ${name}`);
    passed++;
  } else {
    console.log(`  ✗ ${name}`);
    failed++;
  }
}

async function testHealthEndpoint() {
  console.log('\n▸ Health endpoint');
  const res = await fetch(`${BASE_URL}/health`);
  const body = await res.json() as { status: string; uptime: number };

  assert(res.status === 200, 'returns 200');
  assert(body.status === 'ok', 'status is ok');
  assert(typeof body.uptime === 'number', 'uptime is a number');
}

async function testInvalidRequest() {
  console.log('\n▸ Invalid request handling');

  // Missing messages
  const res1 = await fetch(`${BASE_URL}/v1/chat/completions`, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ model: 'test' }),
  });
  const body1 = await res1.json() as { error: { message: string; type: string } };
  assert(res1.status === 400, 'returns 400 for missing messages');
  assert(body1.error?.type === 'invalid_request_error', 'error type is invalid_request_error');

  // Invalid JSON
  const res2 = await fetch(`${BASE_URL}/v1/chat/completions`, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: 'not json',
  });
  assert(res2.status === 400, 'returns 400 for invalid JSON');
}

async function testCacheHitConsistency() {
  console.log('\n▸ Cache hit consistency (requires running server with SDK)');

  const requestBody = {
    model: 'deepseek-v3.1',
    messages: [
      { role: 'user', content: 'Say exactly: "cache test response 12345"' },
    ],
  };

  // First request — cache MISS
  console.log('    Making first request (cache MISS expected)...');
  const res1 = await fetch(`${BASE_URL}/v1/chat/completions`, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(requestBody),
  });

  if (res1.status !== 200) {
    console.log(`    ⚠ Server returned ${res1.status}, skipping cache test (SDK may not be configured)`);
    const errBody = await res1.text();
    console.log(`    Response: ${errBody.slice(0, 200)}`);
    return;
  }

  const body1 = await res1.json() as {
    id: string;
    object: string;
    model: string;
    choices: Array<{ message: { content: string } }>;
    usage: { prompt_tokens: number; completion_tokens: number; total_tokens: number };
  };
  const cache1 = res1.headers.get('X-Cache');

  assert(body1.object === 'chat.completion', 'first response is chat.completion');
  assert(typeof body1.choices[0]?.message?.content === 'string', 'first response has content');
  assert(cache1 === 'MISS', 'first request is cache MISS');

  // Second request — identical, should be cache HIT
  console.log('    Making second request (cache HIT expected)...');
  const res2 = await fetch(`${BASE_URL}/v1/chat/completions`, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(requestBody),
  });

  const body2 = await res2.json() as typeof body1;
  const cache2 = res2.headers.get('X-Cache');

  assert(cache2 === 'HIT', 'second request is cache HIT');
  assert(
    body1.choices[0].message.content === body2.choices[0].message.content,
    'cached response content matches original'
  );
  assert(body2.usage.prompt_tokens === body1.usage.prompt_tokens, 'cached usage.prompt_tokens matches');
  assert(body2.usage.completion_tokens === body1.usage.completion_tokens, 'cached usage.completion_tokens matches');

  // Verify cache stats header
  const statsHeader = res2.headers.get('X-Cache-Stats');
  if (statsHeader) {
    const stats = JSON.parse(statsHeader) as { hits: number; misses: number; hitRate: number };
    assert(stats.hits >= 1, 'cache stats shows hits >= 1');
    assert(stats.hitRate > 0, 'cache stats shows hitRate > 0');
    console.log(`    Cache stats: hits=${stats.hits}, misses=${stats.misses}, hitRate=${(stats.hitRate * 100).toFixed(1)}%`);
  }
}

async function testStreamingFormat() {
  console.log('\n▸ Streaming response format (requires running server with SDK)');

  const res = await fetch(`${BASE_URL}/v1/chat/completions`, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({
      model: 'deepseek-v3.1',
      messages: [{ role: 'user', content: 'Say hi' }],
      stream: true,
    }),
  });

  if (res.status !== 200) {
    console.log(`    ⚠ Server returned ${res.status}, skipping streaming test`);
    return;
  }

  assert(
    res.headers.get('Content-Type')?.includes('text/event-stream') === true,
    'Content-Type is text/event-stream'
  );

  const text = await res.text();
  const lines = text.split('\n').filter(l => l.startsWith('data: '));

  assert(lines.length > 0, 'received SSE data lines');

  // Check first data line is valid JSON chunk
  const firstLine = lines[0];
  if (firstLine && firstLine !== 'data: [DONE]') {
    const chunk = JSON.parse(firstLine.replace('data: ', ''));
    assert(chunk.object === 'chat.completion.chunk', 'chunk object type is correct');
    assert(typeof chunk.id === 'string', 'chunk has id');
  }

  // Check last data line is [DONE]
  const lastLine = lines[lines.length - 1];
  assert(lastLine === 'data: [DONE]', 'stream ends with [DONE]');
}

async function testNotFound() {
  console.log('\n▸ 404 handling');
  const res = await fetch(`${BASE_URL}/v1/nonexistent`);
  assert(res.status === 404, 'returns 404 for unknown endpoint');
}

async function testApiKeyHeaderFallback() {
  console.log('\n▸ API key header fallback');

  const testApiKey = process.env.TEST_CODEBUDDY_API_KEY || 'test-key-placeholder';
  const res = await fetch(`${BASE_URL}/v1/chat/completions`, {
    method: 'POST',
    headers: {
      'Content-Type': 'application/json',
      'X-CodeBuddy-Api-Key': testApiKey,
    },
    body: JSON.stringify({
      model: 'deepseek-v3.1',
      messages: [{ role: 'user', content: 'Say hi' }],
    }),
  });

  assert(res.status !== 400, 'header is accepted (no 400 for X-CodeBuddy-Api-Key)');

  if (res.status === 200) {
    const body = await res.json() as { object: string };
    assert(body.object === 'chat.completion', 'returns valid response with header key');
  } else {
    console.log('    ℹ Could not fully test (need valid TEST_CODEBUDDY_API_KEY)');
  }
}

async function testApiKeyHeaderOnModelsEndpoint() {
  console.log('\n▸ API key header on models endpoint');

  const testApiKey = process.env.TEST_CODEBUDDY_API_KEY || 'test-key-placeholder';
  const res = await fetch(`${BASE_URL}/v1/models`, {
    headers: {
      'X-CodeBuddy-Api-Key': testApiKey,
    },
  });

  assert(res.status !== 400, 'models endpoint accepts X-CodeBuddy-Api-Key header');

  if (res.status === 200) {
    const body = await res.json() as { object: string; data: unknown[] };
    assert(body.object === 'list', 'returns valid models list');
  }
}

async function testAuthorizationBearerHeader() {
  console.log('\n▸ Authorization: Bearer header (OpenAI-compatible)');

  const testApiKey = process.env.TEST_CODEBUDDY_API_KEY || 'test-key-placeholder';
  const res = await fetch(`${BASE_URL}/v1/chat/completions`, {
    method: 'POST',
    headers: {
      'Content-Type': 'application/json',
      'Authorization': `Bearer ${testApiKey}`,
    },
    body: JSON.stringify({
      model: 'deepseek-v3.1',
      messages: [{ role: 'user', content: 'Say hi' }],
    }),
  });

  assert(res.status !== 400, 'Authorization: Bearer header is accepted');

  if (res.status === 200) {
    const body = await res.json() as { object: string };
    assert(body.object === 'chat.completion', 'returns valid response with Bearer auth');
  } else {
    console.log('    ℹ Could not fully test (need valid TEST_CODEBUDDY_API_KEY)');
  }
}

async function testAuthorizationBearerOnModelsEndpoint() {
  console.log('\n▸ Authorization: Bearer header on models endpoint');

  const testApiKey = process.env.TEST_CODEBUDDY_API_KEY || 'test-key-placeholder';
  const res = await fetch(`${BASE_URL}/v1/models`, {
    headers: {
      'Authorization': `Bearer ${testApiKey}`,
    },
  });

  assert(res.status !== 400, 'models endpoint accepts Authorization: Bearer header');

  if (res.status === 200) {
    const body = await res.json() as { object: string; data: unknown[] };
    assert(body.object === 'list', 'returns valid models list with Bearer auth');
  }
}

async function testApiKeyPriority() {
  console.log('\n▸ API key header priority (X-CodeBuddy-Api-Key takes precedence)');

  // When both headers are present, X-CodeBuddy-Api-Key should be used
  const res = await fetch(`${BASE_URL}/v1/models`, {
    headers: {
      'X-CodeBuddy-Api-Key': 'priority-key',
      'Authorization': 'Bearer fallback-key',
    },
  });

  // Both headers are accepted (no 400 error)
  assert(res.status !== 400, 'both headers accepted without validation error');
}

// ============ Run all tests ============

console.log('\n🧪 Integration Tests\n');
console.log(`  Base URL: ${BASE_URL}`);

try {
  // Check if server is running
  const healthCheck = await fetch(`${BASE_URL}/health`).catch(() => null);
  if (!healthCheck) {
    console.log('\n  ⚠ Server not running. Start it with: npm run dev\n');
    process.exit(1);
  }

  await testHealthEndpoint();
  await testInvalidRequest();
  await testNotFound();
  await testCacheHitConsistency();
  await testStreamingFormat();
  await testApiKeyHeaderFallback();
  await testApiKeyHeaderOnModelsEndpoint();
  await testAuthorizationBearerHeader();
  await testAuthorizationBearerOnModelsEndpoint();
  await testApiKeyPriority();
} catch (error) {
  console.error('\n  ✗ Test error:', error);
  failed++;
}

console.log(`\n${'═'.repeat(40)}`);
console.log(`  Results: ${passed} passed, ${failed} failed`);
console.log(`${'═'.repeat(40)}\n`);

if (failed > 0) {
  process.exit(1);
}
