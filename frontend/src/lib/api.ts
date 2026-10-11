import type { ApiMeta, HealthReport, RelType, SearchResult, Subgraph } from '../types/graph';

const BASE_URL = (import.meta.env.VITE_API_BASE_URL ?? 'http://localhost:8080').replace(/\/$/, '');
const TIMEOUT_MS = 15_000;

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

export interface NeighborOptions {
  depth: number;
  limit: number;
  relationshipTypes: RelType[];
}

export const api = {
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
