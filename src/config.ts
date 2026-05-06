import 'dotenv/config';

export interface AppConfig {
  port: number;
  host: string;
  defaultModel: string;
  fallbackModel: string;
  cache: {
    enabled: boolean;
    ttlMs: number;
    maxSize: number;
  };
  logLevel: 'debug' | 'info' | 'warn' | 'error';
  /** Whether debug mode is active (LOG_LEVEL=debug or NODE_ENV=development). */
  debug: boolean;
  codebuddy: {
    apiKey?: string;
    environment?: string;
  };
  proxy?: {
    url?: string;
  };
}

function getEnv(key: string, fallback?: string): string | undefined {
  return process.env[key] ?? fallback;
}

function getEnvRequired(key: string, fallback: string): string {
  return process.env[key] ?? fallback;
}

function getEnvInt(key: string, fallback: number): number {
  const val = process.env[key];
  if (val === undefined) return fallback;
  const parsed = parseInt(val, 10);
  return isNaN(parsed) ? fallback : parsed;
}

function getEnvBool(key: string, fallback: boolean): boolean {
  const val = process.env[key];
  if (val === undefined) return fallback;
  return val === 'true' || val === '1';
}

const _logLevel = (getEnv('LOG_LEVEL', 'info') as AppConfig['logLevel']);
const _nodeEnv = getEnv('NODE_ENV', 'production');

export const config: AppConfig = {
  port: getEnvInt('PORT', 3000),
  host: getEnvRequired('HOST', '0.0.0.0'),
  defaultModel: getEnvRequired('DEFAULT_MODEL', 'deepseek-v3.1'),
  fallbackModel: getEnvRequired('FALLBACK_MODEL', 'deepseek-v3.1'),
  cache: {
    enabled: getEnvBool('CACHE_ENABLED', true),
    ttlMs: getEnvInt('CACHE_TTL_MS', 300_000),
    maxSize: getEnvInt('CACHE_MAX_SIZE', 100),
  },
  logLevel: _logLevel,
  debug: _logLevel === 'debug' || _nodeEnv === 'development',
  codebuddy: {
    apiKey: getEnv('CODEBUDDY_API_KEY'),
    environment: getEnv('CODEBUDDY_INTERNET_ENVIRONMENT'),
  },
  proxy: {
    url: getEnv('HTTP_PROXY_URL'),
  },
};
