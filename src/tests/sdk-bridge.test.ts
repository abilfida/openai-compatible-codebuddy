/**
 * SDK Bridge Unit Tests
 *
 * Run: npx tsx src/tests/sdk-bridge.test.ts
 */
import { config } from '../config.js';
import { buildEnv } from '../services/sdk-bridge.js';

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

function assertEqual<T>(actual: T, expected: T, name: string) {
  const ok = actual === expected;
  if (!ok) {
    console.log(`  ✗ ${name} (expected ${JSON.stringify(expected)}, got ${JSON.stringify(actual)})`);
    failed++;
  } else {
    console.log(`  ✓ ${name}`);
    passed++;
  }
}

function assertUndefined(value: unknown, name: string) {
  const ok = value === undefined;
  if (!ok) {
    console.log(`  ✗ ${name} (expected undefined, got ${JSON.stringify(value)})`);
    failed++;
  } else {
    console.log(`  ✓ ${name}`);
    passed++;
  }
}

console.log('\n🧪 SDK Bridge Unit Tests\n');

// --- Test 1: buildEnv includes proxy env vars when proxy.url is set ---
console.log('▸ buildEnv with proxy.url set');
{
  const originalProxy = config.proxy;
  config.proxy = { url: 'http://proxy.example.com:8080' };

  const env = buildEnv();
  assertEqual(env.HTTP_PROXY, 'http://proxy.example.com:8080', 'HTTP_PROXY is set');
  assertEqual(env.HTTPS_PROXY, 'http://proxy.example.com:8080', 'HTTPS_PROXY is set');
  assertEqual(env.NO_PROXY, 'localhost,127.0.0.1', 'NO_PROXY is set');

  config.proxy = originalProxy;
}

// --- Test 2: buildEnv does not include proxy env vars when not configured ---
console.log('\n▸ buildEnv without proxy.url');
{
  const originalProxy = config.proxy;
  config.proxy = undefined;

  const env = buildEnv();
  assertUndefined(env.HTTP_PROXY, 'HTTP_PROXY is undefined when proxy not configured');
  assertUndefined(env.HTTPS_PROXY, 'HTTPS_PROXY is undefined when proxy not configured');

  config.proxy = originalProxy;
}

// --- Test 3: buildEnv uses requestApiKey when provided ---
console.log('\n▸ buildEnv with requestApiKey');
{
  const env = buildEnv('test-api-key-123');
  assertEqual(env.CODEBUDDY_API_KEY, 'test-api-key-123', 'CODEBUDDY_API_KEY uses requestApiKey');
}

// --- Test 4: buildEnv falls back to config.codebuddy.apiKey ---
console.log('\n▸ buildEnv falls back to config.apiKey');
{
  const originalApiKey = config.codebuddy.apiKey;
  config.codebuddy.apiKey = 'config-api-key-456';

  const env = buildEnv();
  assertEqual(env.CODEBUDDY_API_KEY, 'config-api-key-456', 'CODEBUDDY_API_KEY falls back to config');

  config.codebuddy.apiKey = originalApiKey;
}

// ============ Summary ============
console.log('\n===============');
console.log(`Tests: ${passed} passed, ${failed} failed`);
console.log('===============\n');

if (failed > 0) {
  process.exit(1);
}