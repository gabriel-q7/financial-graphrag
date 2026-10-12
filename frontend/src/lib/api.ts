import type { AskEvent } from '../types/ask';
import type { ApiMeta, HealthReport, RelType, SearchResult, Subgraph } from '../types/graph';
import { createSSEParser } from './sse';

const BASE_URL = (import.meta.env.VITE_API_BASE_URL ?? 'http://localhost:8080').replace(/\/$/, '');
const TIMEOUT_MS = 15_000;
/** Answers are generated on CPU and can take minutes (backend limit: 5 min). */
const ASK_TIMEOUT_MS = 6 * 60_000;

/** Error raised for any failed API call, with a message safe to show to users. */
export class ApiError extends Error {
  readonly status: number;
  readonly code: string;

  constructor(status: number, code: string, message: string) {
    super(message);
    this.name = 'ApiError';
    this.status = status;
    this.code = code;
  }

  /** True when the backend itself could not be reached. */
  get isNetwork(): boolean {
    return this.status === 0;
  }
}

interface RequestOptions {
  signal?: AbortSignal | undefined;
  /** Status codes that still carry a valid JSON body (e.g. 503 on /api/health). */
  acceptStatus?: number[];
}

async function request<T>(path: string, opts: RequestOptions = {}): Promise<T> {
  const timeout = AbortSignal.timeout(TIMEOUT_MS);
  const signal = opts.signal ? AbortSignal.any([opts.signal, timeout]) : timeout;

  let res: Response;
  try {
    res = await fetch(`${BASE_URL}${path}`, { headers: { Accept: 'application/json' }, signal });
  } catch (err) {
    if (opts.signal?.aborted) throw err;
    if (timeout.aborted) throw new ApiError(0, 'timeout', 'A API não respondeu a tempo.');
    throw new ApiError(0, 'network', `Não foi possível conectar à API em ${BASE_URL}.`);
  }

  let body: unknown = null;
  try {
    body = await res.json();
  } catch {
    // Non-JSON body: handled below.
  }

  if (res.ok || opts.acceptStatus?.includes(res.status)) {
    if (body === null) throw new ApiError(res.status, 'invalid_response', 'Resposta inválida da API.');
    return body as T;
  }
  const error = (body as { error?: { code?: string; message?: string } } | null)?.error;
  throw new ApiError(res.status, error?.code ?? 'http_error', error?.message ?? `Erro HTTP ${res.status}.`);
}

async function errorFromResponse(res: Response): Promise<ApiError> {
  type ErrorEnvelope = { error?: { code?: string; message?: string } };
  let body: ErrorEnvelope | null = null;
  try {
    body = (await res.json()) as ErrorEnvelope;
  } catch {
    // Non-JSON body.
  }
  return new ApiError(res.status, body?.error?.code ?? 'http_error', body?.error?.message ?? `Erro HTTP ${res.status}.`);
}

/**
 * POST /api/ask and dispatch each Server-Sent Event as it arrives.
 * Resolves when the stream ends; rejects with ApiError for errors before the stream starts.
 */
async function ask(question: string, onEvent: (e: AskEvent) => void, signal: AbortSignal): Promise<void> {
  const timeout = AbortSignal.timeout(ASK_TIMEOUT_MS);
  let res: Response;
  try {
    res = await fetch(`${BASE_URL}/api/ask`, {
      method: 'POST',
      headers: { 'Content-Type': 'application/json', Accept: 'text/event-stream' },
      body: JSON.stringify({ question }),
      signal: AbortSignal.any([signal, timeout]),
    });
  } catch (err) {
    if (signal.aborted) throw err;
    throw new ApiError(0, 'network', `Não foi possível conectar à API em ${BASE_URL}.`);
  }
  if (!res.ok || !res.body) throw await errorFromResponse(res);

  const feed = createSSEParser(({ event, data }) => {
    onEvent({ type: event, data: JSON.parse(data) } as AskEvent);
  });
  const reader = res.body.pipeThrough(new TextDecoderStream()).getReader();
  try {
    for (;;) {
      const { value, done } = await reader.read();
      if (done) break;
      feed(value);
    }
  } catch (err) {
    if (signal.aborted) throw err;
    if (timeout.aborted) throw new ApiError(0, 'timeout', 'A resposta excedeu o tempo limite.');
    throw new ApiError(0, 'network', 'A conexão com a API foi interrompida.');
  }
}

export interface NeighborOptions {
  depth: number;
  limit: number;
  relationshipTypes: RelType[];
}

export const api = {
  ask,

  health: (signal?: AbortSignal) => request<HealthReport>('/api/health', { signal, acceptStatus: [503] }),

  meta: (signal?: AbortSignal) => request<ApiMeta>('/api/meta', { signal }),

  search: (q: string, signal?: AbortSignal) =>
    request<SearchResult>(`/api/entities?${new URLSearchParams({ q, limit: '10' })}`, { signal }),

  entity: (id: string, limit: number, signal?: AbortSignal) =>
    request<Subgraph>(`/api/graph/${encodeURIComponent(id)}?${new URLSearchParams({ limit: String(limit) })}`, {
      signal,
    }),

  neighbors: (id: string, opts: NeighborOptions, signal?: AbortSignal) => {
    const params = new URLSearchParams({ depth: String(opts.depth), limit: String(opts.limit) });
    if (opts.relationshipTypes.length > 0) params.set('relationshipTypes', opts.relationshipTypes.join(','));
    return request<Subgraph>(`/api/graph/${encodeURIComponent(id)}/neighbors?${params}`, { signal });
  },
};
