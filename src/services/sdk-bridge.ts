import { query as sdkQuery } from '@tencent-ai/agent-sdk';
import type { Message, Options, UserMessage } from '@tencent-ai/agent-sdk';
import { config } from '../config.js';
import { convertMessages, extractTextFromContentBlocks } from './message-converter.js';
import type { AnthropicContentBlock } from './message-converter.js';
import {
  formatChatCompletion,
  formatChatCompletionChunk,
  formatSSEMessage,
  formatSSEDone,
  formatModelList,
  mapUsage,
} from './response-formatter.js';
import type { ChatMessage, ChatCompletionResponse, ModelListResponse } from '../types/index.js';

function buildEnv(requestApiKey?: string): Record<string, string | undefined> {
  const env: Record<string, string | undefined> = {};
  env.CODEBUDDY_API_KEY = requestApiKey ?? config.codebuddy.apiKey;
  if (config.codebuddy.environment) {
    env.CODEBUDDY_INTERNET_ENVIRONMENT = config.codebuddy.environment;
  }
  return env;
}

// Default system prompt used when the caller doesn't supply one.
// This overrides the SDK's built-in CodeBuddy Code prompt so the model
// behaves as a plain assistant instead of a coding-specific agent.
const DEFAULT_SYSTEM_PROMPT =
  'You are a helpful assistant. Respond to the user\'s message directly.';

function buildOptions(model: string, systemPrompt?: string, requestApiKey?: string): Options {
  return {
    model,
    fallbackModel: config.fallbackModel,
    permissionMode: 'bypassPermissions',
    // Pure chat mode: disable ALL built-in tools so the Agent only produces
    // text responses. Without this, the SDK Agent has access to Bash, Read,
    // Write, Edit, Grep, Glob, WebSearch, etc. and will try to call them,
    // consuming extra turns and causing long delays or "Max turns exceeded".
    allowedTools: [],
    // With no tools available, 1 turn is sufficient: user message → text reply.
    maxTurns: 1,
    env: buildEnv(requestApiKey),
    // Always set systemPrompt to override the SDK's built-in CodeBuddy prompt.
    // When the caller provides one, use it; otherwise fall back to a neutral prompt.
    systemPrompt: systemPrompt ?? DEFAULT_SYSTEM_PROMPT,
  };
}

export interface ChatCompletionParams {
  model: string;
  messages: ChatMessage[];
  stream?: boolean;
}

/**
 * Build the prompt parameter for `sdkQuery`.
 * When the message contains images, we construct an `AsyncIterable<UserMessage>`
 * so the SDK forwards Anthropic-style content blocks (including image blocks)
 * directly to the CLI, enabling native multimodal support.
 */
function buildPrompt(
  prompt: string,
  contentBlocks?: AnthropicContentBlock[],
): string | AsyncIterable<UserMessage> {
  if (!contentBlocks || contentBlocks.length === 0) {
    return prompt;
  }

  // Build a single UserMessage with Anthropic content blocks.
  // The SDK's transport.sendUserMessage() will JSON.stringify it
  // and write to the CLI's stdin – the CLI then forwards these
  // blocks (including image blocks) to the model API.
  const userMessage: UserMessage = {
    type: 'user',
    session_id: '',
    message: {
      role: 'user',
      // Cast: SDK types don't include image blocks, but the CLI accepts them
      // eslint-disable-next-line @typescript-eslint/no-explicit-any
      content: contentBlocks as any,
    },
    parent_tool_use_id: null,
  };

  // Return as an async iterable yielding a single message
  async function* singleMessage(): AsyncIterable<UserMessage> {
    yield userMessage;
  }
  return singleMessage();
}

/**
 * Non-streaming chat completion.
 * Collects all assistant text and returns a complete response.
 */
export async function chatCompletion(
  params: ChatCompletionParams
): Promise<ChatCompletionResponse> {
  const { systemPrompt, prompt, contentBlocks } = await convertMessages(params.messages);
  const model = params.model || config.defaultModel;
  const options = buildOptions(model, systemPrompt);

  const q = sdkQuery({ prompt: buildPrompt(prompt, contentBlocks), options });
  let fullText = '';
  let lastUsage = { input_tokens: 0, output_tokens: 0 };
  let actualModel = model;

  for await (const message of q) {
    if (message.type === 'assistant') {
      const text = extractTextFromContentBlocks(
        message.message.content as ReadonlyArray<{ type: string; text?: string }>
      );
      fullText += text;
      lastUsage = message.message.usage;
      actualModel = message.message.model;
    } else if (message.type === 'result') {
      if (message.usage) {
        lastUsage = message.usage;
      }
    }
  }

  return formatChatCompletion(fullText, actualModel, lastUsage);
}

/**
 * Streaming chat completion.
 * Returns a ReadableStream of SSE-formatted data.
 */
export function chatCompletionStream(
  params: ChatCompletionParams
): ReadableStream<Uint8Array> {
  const encoder = new TextEncoder();

  return new ReadableStream<Uint8Array>({
    async start(controller) {
      try {
        const { systemPrompt, prompt, contentBlocks } = await convertMessages(params.messages);
        const model = params.model || config.defaultModel;
        const options: Options = {
          ...buildOptions(model, systemPrompt),
          includePartialMessages: true,
        };

        const q = sdkQuery({ prompt: buildPrompt(prompt, contentBlocks), options });
        let sentRole = false;
        let actualModel = model;

        // Buffer the final stop signal — we must send it AFTER all content
        // chunks. The SDK may emit `message_delta` (stop_reason) before
        // the `result` event that carries the full text in non-streaming
        // fallback scenarios.
        let pendingStopUsage: { input_tokens: number; output_tokens: number } | null = null;

        for await (const message of q) {
          if (message.type === 'stream_event') {
            const event = message.event;

            if (event.type === 'content_block_delta') {
              const delta = event.delta;
              if (delta.type === 'text_delta') {
                if (!sentRole) {
                  // First chunk: include role
                  const chunk = formatChatCompletionChunk(
                    delta.text, actualModel, null, null, true
                  );
                  controller.enqueue(encoder.encode(formatSSEMessage(chunk)));
                  sentRole = true;
                } else {
                  const chunk = formatChatCompletionChunk(
                    delta.text, actualModel
                  );
                  controller.enqueue(encoder.encode(formatSSEMessage(chunk)));
                }
              }
            } else if (event.type === 'message_start') {
              actualModel = event.message.model;
            } else if (event.type === 'message_delta') {
              // Don't send stop immediately — buffer it.
              // It will be sent after all content is done.
              pendingStopUsage = event.usage ? {
                input_tokens: event.usage.input_tokens,
                output_tokens: event.usage.output_tokens,
              } : null;
            }
          } else if (message.type === 'result') {
            // If no stream events produced text, send the full result as a single chunk
            if (!sentRole && 'result' in message) {
              const resultMsg = message as { result: string; usage: typeof lastUsage; [key: string]: unknown };
              const chunk = formatChatCompletionChunk(
                resultMsg.result, actualModel, null, null, true
              );
              controller.enqueue(encoder.encode(formatSSEMessage(chunk)));
              sentRole = true;

              // Use result usage if we don't have one from message_delta
              if (!pendingStopUsage && resultMsg.usage) {
                pendingStopUsage = {
                  input_tokens: resultMsg.usage.input_tokens,
                  output_tokens: resultMsg.usage.output_tokens,
                };
              }
            }
          }
        }

        // Now send the final stop chunk
        const stopChunk = formatChatCompletionChunk(
          null, actualModel, 'stop', pendingStopUsage
        );
        controller.enqueue(encoder.encode(formatSSEMessage(stopChunk)));

        controller.enqueue(encoder.encode(formatSSEDone()));
        controller.close();
      } catch (error) {
        const errMsg = error instanceof Error ? error.message : 'Unknown error';
        const errChunk = `data: ${JSON.stringify({ error: { message: errMsg, type: 'server_error' } })}\n\n`;
        controller.enqueue(encoder.encode(errChunk));
        controller.enqueue(encoder.encode(formatSSEDone()));
        controller.close();
      }
    },
  });
}

// Type alias for the result usage
type ResultUsage = { input_tokens: number; output_tokens: number; cache_read_input_tokens?: number | null; cache_creation_input_tokens?: number | null };
let lastUsage: ResultUsage;

// ---- Models cache ----
// Cache model list in memory to avoid spinning up a CLI subprocess on every request.
// TTL: 5 minutes (models rarely change during a session).

interface ModelsCache {
  data: ModelListResponse;
  expiresAt: number;
}

let modelsCache: ModelsCache | null = null;
let modelsFetchInFlight: Promise<ModelListResponse> | null = null;

/**
 * Get available models from the SDK.
 * Results are cached in memory for 5 minutes to avoid the expensive
 * subprocess startup on every request.
 */
export async function getModels(): Promise<ModelListResponse> {
  // Return cached result if still valid
  if (modelsCache && Date.now() < modelsCache.expiresAt) {
    return modelsCache.data;
  }

  // Deduplicate concurrent requests — if a fetch is already in progress, wait for it
  if (modelsFetchInFlight) {
    return modelsFetchInFlight;
  }

  modelsFetchInFlight = fetchModelsFromSDK();
  try {
    const result = await modelsFetchInFlight;
    return result;
  } finally {
    modelsFetchInFlight = null;
  }
}

async function fetchModelsFromSDK(): Promise<ModelListResponse> {
  const abortController = new AbortController();

  const q = sdkQuery({
    prompt: 'hi',
    options: {
      model: config.defaultModel,
      permissionMode: 'plan',
      maxTurns: 1,
      env: buildEnv(),
      abortController,
    },
  });

  try {
    const models = await q.supportedModels();

    // Immediately abort the query — we only needed the model list,
    // no need to wait for the actual LLM response.
    abortController.abort();

    const result = formatModelList(models);

    // Cache the result for 5 minutes
    modelsCache = {
      data: result,
      expiresAt: Date.now() + 5 * 60 * 1000,
    };

    return result;
  } catch {
    // Abort in case of error too
    abortController.abort();

    // Fallback: return default model
    return formatModelList([
      { id: config.defaultModel, name: config.defaultModel },
    ]);
  }
}

/**
 * Invalidate the models cache (useful for testing or manual refresh).
 */
export function invalidateModelsCache(): void {
  modelsCache = null;
}
