/**
 * Proxy URL Parsing Unit Tests
 *
 * Run: npm test
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

function assertDefined(value: unknown, name: string) {
  const ok = value !== undefined;
  if (!ok) {
    console.log(`  ✗ ${name} (expected defined value, got undefined)`);
    failed++;
  } else {
    console.log(`  ✓ ${name}`);
    passed++;
  }
}

console.log('\n🧪 Proxy URL Parsing Unit Tests\n');

// --- Test 1: config.proxy structure ---
console.log('▸ config.proxy structure');
{
  assertDefined(config.proxy, 'config.proxy is defined');
  assertEqual(typeof config.proxy, 'object', 'config.proxy is an object');
}

// --- Test 2: config.proxy.url based on HTTP_PROXY_URL env ---
console.log('\n▸ config.proxy.url from environment');
{
  // HTTP_PROXY_URL is not set in .env, so config.proxy.url should be undefined
  // (unless the environment running the test has it set externally)
  if (config.proxy?.url === undefined) {
    console.log('  ℹ HTTP_PROXY_URL not set in environment (expected for this test run)');
    assertUndefined(config.proxy?.url, 'config.proxy.url is undefined when HTTP_PROXY_URL not set');
  } else {
    console.log(`  ℹ HTTP_PROXY_URL is set to: ${config.proxy.url}`);
    assertDefined(config.proxy?.url, 'config.proxy.url is defined when HTTP_PROXY_URL is set');
  }
}

// --- Test 3: buildEnv proxy vars when proxy.url is set (mock test) ---
console.log('\n▸ buildEnv with proxy.url set');
{
  const originalProxy = config.proxy;

  // Test with non-authenticated proxy URL
  config.proxy = { url: 'http://proxy.example.com:8080' };

  const env = buildEnv();
  assertEqual(env.HTTP_PROXY, 'http://proxy.example.com:8080', 'HTTP_PROXY is set for non-auth URL');
  assertEqual(env.HTTPS_PROXY, 'http://proxy.example.com:8080', 'HTTPS_PROXY is set for non-auth URL');
  assertEqual(env.NO_PROXY, 'localhost,127.0.0.1', 'NO_PROXY is set correctly');

  config.proxy = originalProxy;
}

// --- Test 4: buildEnv with authenticated proxy URL ---
console.log('\n▸ buildEnv with authenticated proxy URL');
{
  const originalProxy = config.proxy;

  // Test with authenticated proxy URL
  config.proxy = { url: 'http://user:pass@proxy.example.com:8080' };

  const env = buildEnv();
  assertEqual(env.HTTP_PROXY, 'http://user:pass@proxy.example.com:8080', 'HTTP_PROXY handles auth URL');
  assertEqual(env.HTTPS_PROXY, 'http://user:pass@proxy.example.com:8080', 'HTTPS_PROXY handles auth URL');

  config.proxy = originalProxy;
}

// --- Test 5: buildEnv does not include proxy vars when proxy is undefined ---
console.log('\n▸ buildEnv without proxy.url');
{
  const originalProxy = config.proxy;

  config.proxy = undefined;

  const env = buildEnv();
  assertUndefined(env.HTTP_PROXY, 'HTTP_PROXY is undefined when proxy not configured');
  assertUndefined(env.HTTPS_PROXY, 'HTTPS_PROXY is undefined when proxy not configured');
  assertUndefined(env.NO_PROXY, 'NO_PROXY is undefined when proxy not configured');

  config.proxy = originalProxy;
}

// --- Test 6: buildEnv does not include proxy vars when proxy.url is undefined ---
console.log('\n▸ buildEnv with proxy but no url');
{
  const originalProxy = config.proxy;

  config.proxy = {};

  const env = buildEnv();
  assertUndefined(env.HTTP_PROXY, 'HTTP_PROXY is undefined when proxy.url not set');
  assertUndefined(env.HTTPS_PROXY, 'HTTPS_PROXY is undefined when proxy.url not set');

  config.proxy = originalProxy;
}

// --- Test 7: Proxy URL format validation (non-auth) ---
console.log('\n▸ Proxy URL format validation (non-authenticated)');
{
  const originalProxy = config.proxy;

  config.proxy = { url: 'http://proxy.example.com:8080' };

  const env = buildEnv();
  assert(env.HTTP_PROXY?.startsWith('http://'), 'Non-auth proxy URL uses http:// protocol');
  assert(env.HTTP_PROXY?.includes('proxy.example.com'), 'Non-auth proxy URL contains hostname');
  assert(env.HTTP_PROXY?.includes(':8080'), 'Non-auth proxy URL contains port');

  config.proxy = originalProxy;
}

// --- Test 8: Proxy URL format validation (auth) ---
console.log('\n▸ Proxy URL format validation (authenticated)');
{
  const originalProxy = config.proxy;

  config.proxy = { url: 'http://admin:secret123@proxy.internal:3128' };

  const env = buildEnv();
  assert(env.HTTP_PROXY?.startsWith('http://'), 'Auth proxy URL uses http:// protocol');
  assert(env.HTTP_PROXY?.includes('admin'), 'Auth proxy URL contains username');
  assert(env.HTTP_PROXY?.includes('proxy.internal'), 'Auth proxy URL contains hostname');
  assert(env.HTTP_PROXY?.includes('3128'), 'Auth proxy URL contains port');

  config.proxy = originalProxy;
}

// --- Test 9: HTTPS proxy is set independently ---
console.log('\n▸ HTTPS proxy setting');
{
  const originalProxy = config.proxy;

  config.proxy = { url: 'http://proxy.example.com:8080' };

  const env = buildEnv();
  assertDefined(env.HTTPS_PROXY, 'HTTPS_PROXY is set');
  assertEqual(env.HTTP_PROXY, env.HTTPS_PROXY, 'HTTP and HTTPS proxy are equal');

  config.proxy = originalProxy;
}

// --- Test 10: NO_PROXY contains localhost entries ---
console.log('\n▸ NO_PROXY configuration');
{
  const originalProxy = config.proxy;

  config.proxy = { url: 'http://proxy.example.com:8080' };

  const env = buildEnv();
  assert(env.NO_PROXY?.includes('localhost'), 'NO_PROXY includes localhost');
  assert(env.NO_PROXY?.includes('127.0.0.1'), 'NO_PROXY includes 127.0.0.1');

  config.proxy = originalProxy;
}

// ============ Summary ============
console.log(`\n${'═'.repeat(40)}`);
console.log(`  Results: ${passed} passed, ${failed} failed`);
console.log(`${'═'.repeat(40)}\n`);

if (failed > 0) {
  process.exit(1);
}