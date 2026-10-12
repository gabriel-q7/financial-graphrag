// Types mirror backend/internal/rag (Retrieval, Answer) and the /api/ask SSE events.
import type { GraphEdge, GraphNode } from './graph';

export interface DocumentHit {
  node: GraphNode;
  /** Cosine score in [0,1]; -1 when the document was reached through the graph. */
  score: number;
}

export interface Retrieval {
  question: string;
  entities: GraphNode[];
  documents: DocumentHit[];
  nodes: GraphNode[];
  edges: GraphEdge[];
  noEvidence: boolean;
  contextChars: number;
  durationMs: number;
}

export interface GenerationStats {
  model: string;
  promptTokens: number;
  generatedTokens: number;
  /** Time until the first token: model load + reading the evidence. */
  firstTokenMs: number;
  generationMs: number;
  tokensPerSecond: number;
}

export interface AskAnswer {
  answer: string;
  citations: { valid: string[]; invalid: string[] };
  model: string;
  noEvidence: boolean;
  stats: GenerationStats;
  durationMs: number;
}

export type AskEvent =
  | { type: 'retrieval'; data: Retrieval }
  | { type: 'status'; data: { phase: 'generating' } }
  | { type: 'token'; data: { text: string } }
  | { type: 'done'; data: AskAnswer }
  | { type: 'error'; data: { code: string; message: string } };
