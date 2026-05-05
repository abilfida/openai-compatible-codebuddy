import { Hono } from 'hono';
import { getModels } from '../services/sdk-bridge.js';
import { formatModelObject, formatError } from '../services/response-formatter.js';
import { extractApiKey } from '../utils/auth.js';

export const modelsRoutes = new Hono();

// GET /v1/models — List available models
modelsRoutes.get('/v1/models', async (c) => {
  try {
    const requestApiKey = extractApiKey(c);
    const modelList = await getModels(requestApiKey);
    return c.json(modelList);
  } catch (error) {
    const msg = error instanceof Error ? error.message : 'Failed to fetch models';
    return c.json(formatError(msg, 'server_error'), 500);
  }
});

// GET /v1/models/:model — Get single model info
modelsRoutes.get('/v1/models/:model', async (c) => {
  const modelId = c.req.param('model');
  try {
    const requestApiKey = extractApiKey(c);
    const modelList = await getModels(requestApiKey);
    const found = modelList.data.find(m => m.id === modelId);
    if (!found) {
      return c.json(
        formatError(`Model '${modelId}' not found`, 'invalid_request_error', 'model_not_found'),
        404
      );
    }
    return c.json(found);
  } catch (error) {
    const msg = error instanceof Error ? error.message : 'Failed to fetch model';
    return c.json(formatError(msg, 'server_error'), 500);
  }
});
