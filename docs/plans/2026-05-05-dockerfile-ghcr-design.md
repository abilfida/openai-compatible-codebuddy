# Dockerfile dan GitHub Actions Design

**Date:** 2026-05-05

## Overview

Dockerfile production-optimized dan GitHub Actions workflow untuk auto-build ke GHCR saat release/tag.

## Dockerfile Design

### Approach: Multi-stage Build dengan Alpine

**Stage 1 (Builder):**
- Base: `node:18-alpine`
- Install dependencies dengan `npm ci`
- Build TypeScript dengan `npm run build`
- Output: `dist/` directory

**Stage 2 (Production):**
- Base: `node:18-alpine`
- Copy hanya `dist/` dan production dependencies
- Run dengan `npm start`
- Non-root user (`node`) untuk security
- Health check: curl ke `http://localhost:3000/v1/models`

**Image size target:** ~80-100MB

### Environment Variables

Runtime configuration via environment variables:
- `CODEBUDDY_API_KEY` - Required
- `PORT` - Default: 3000
- `HOST` - Default: 0.0.0.0
- `DEFAULT_MODEL` - Optional
- `CACHE_ENABLED` - Optional
- `LOG_LEVEL` - Optional

## GitHub Actions Workflow Design

### Trigger

Tags dengan format `v*` (e.g., `v1.0.0`, `v2.1.0-beta`)

```yaml
on:
  push:
    tags:
      - 'v*'
```

### Image Registry

GitHub Container Registry (GHCR):
- Image: `ghcr.io/{owner}/{repo}`
- Tagging:
  - `{version}` - exact tag (e.g., `ghcr.io/user/repo:1.0.0`)
  - `latest` - update setiap release baru

### Workflow Steps

1. Checkout code
2. Setup Docker buildx untuk multi-platform support
3. Login to GHCR dengan `GITHUB_TOKEN`
4. Extract version dari git tag
5. Build & push dengan GitHub Actions cache

### Permissions

- `packages: write` - untuk push ke GHCR
- `contents: read` - untuk checkout code

## Security Considerations

1. Non-root user dalam container
2. Tidak include source files dalam production image
3. Tidak include dev dependencies dalam production image
4. `.env` files tidak di-copy ke image (runtime config via env vars)