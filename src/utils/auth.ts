import type { Context } from 'hono';

/**
 * Extract API key from request headers.
 *
 * Priority order:
 * 1. X-CodeBuddy-Api-Key header (custom, takes precedence)
 * 2. Authorization: Bearer <api-key> header (OpenAI-compatible standard)
 *
 * @param c - Hono context
 * @returns Extracted API key string, or undefined if not found
 */
export function extractApiKey(c: Context): string | undefined {
  // Priority 1: Custom header
  const customHeader = c.req.header('X-CodeBuddy-Api-Key');
  if (customHeader) {
    return customHeader;
  }

  // Priority 2: Authorization: Bearer header (OpenAI-compatible)
  const authHeader = c.req.header('Authorization');
  if (authHeader) {
    const parts = authHeader.split(' ');
    if (parts.length === 2 && parts[0] === 'Bearer') {
      return parts[1];
    }
  }

  return undefined;
}