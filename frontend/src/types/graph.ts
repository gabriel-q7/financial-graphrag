// Types mirror the backend DTOs (backend/internal/graph/dto.go).

export const NODE_TYPES = ['Person', 'Company', 'BankAccount', 'Transaction', 'Device', 'Document'] as const;
export type NodeType = (typeof NODE_TYPES)[number];

export const REL_TYPES = ['OWNS', 'CONTROLS', 'SENT', 'TO_ACCOUNT', 'USES', 'MENTIONS'] as const;
export type RelType = (typeof REL_TYPES)[number];

export type PropertyValue = string | number | boolean | PropertyValue[];

export interface GraphNode {
  id: string;
  type: NodeType;
  properties: Record<string, PropertyValue>;
}

export interface GraphEdge {
  id: string;
  type: RelType;
  source: string;
  target: string;
}

export interface SubgraphMeta {
  depth: number;
  nodeLimit: number;
  edgeLimit: number;
  relationshipTypes: RelType[];
  nodeCount: number;
  edgeCount: number;
  truncated: boolean;
}

export interface Subgraph {
  center: GraphNode;
  nodes: GraphNode[];
  edges: GraphEdge[];
  meta: SubgraphMeta;
}

export interface ApiLimits {
  defaultDepth: number;
  maxDepth: number;
  defaultLimit: number;
  maxLimit: number;
  edgeLimitFactor: number;
}

export interface ApiMeta {
  nodeTypes: { type: NodeType; idPrefix: string; properties: string[] }[];
  relationshipTypes: { type: RelType; from: NodeType[]; to: NodeType[] }[];
  limits: ApiLimits;
}

export type HealthStatus = 'ok' | 'degraded' | 'unavailable';

export interface HealthCheck {
  status: HealthStatus;
  essential: boolean;
  latencyMs: number;
  error?: string;
  details?: Record<string, unknown>;
}

export interface HealthReport {
  status: HealthStatus;
  checks: Record<string, HealthCheck>;
  timestamp: string;
}

export interface SearchResult {
  items: GraphNode[];
  limit: number;
}
