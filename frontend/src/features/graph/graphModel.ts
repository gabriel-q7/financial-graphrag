import type { Edge, Node, XYPosition } from '@xyflow/react';
import type { GraphEdge, GraphNode, NodeType, RelType, Subgraph } from '../../types/graph';

export interface EntityNodeData extends Record<string, unknown> {
  entity: GraphNode;
  expanded: boolean;
  isRoot: boolean;
}
export type EntityFlowNode = Node<EntityNodeData, 'entity'>;

export interface RelEdgeData extends Record<string, unknown> {
  rel: GraphEdge;
}
export type RelFlowEdge = Edge<RelEdgeData>;

export interface MergeResult {
  nodes: EntityFlowNode[];
  edges: RelFlowEdge[];
  addedNodes: number;
  addedEdges: number;
}

const RING_SPACING = 230;
const MIN_ARC_GAP = 120;

/** Hop distance from `start` to every node reachable through `edges` (undirected). */
export function hopDistances(start: string, edges: GraphEdge[]): Map<string, number> {
  const adj = new Map<string, string[]>();
  for (const e of edges) {
    adj.set(e.source, [...(adj.get(e.source) ?? []), e.target]);
    adj.set(e.target, [...(adj.get(e.target) ?? []), e.source]);
  }
  const dist = new Map<string, number>([[start, 0]]);
  const queue = [start];
  for (let i = 0; i < queue.length; i++) {
    const cur = queue[i]!;
    for (const next of adj.get(cur) ?? []) {
      if (!dist.has(next)) {
        dist.set(next, dist.get(cur)! + 1);
        queue.push(next);
      }
    }
  }
  return dist;
}

/**
 * Places `ids` on rings around `anchor`, one ring per hop distance. When the graph
 * already has other nodes, new nodes fan out on the side facing away from them.
 */
export function layoutAround(
  anchor: XYPosition,
  ids: string[],
  distances: Map<string, number>,
  existing: XYPosition[],
): Map<string, XYPosition> {
  const byRing = new Map<number, string[]>();
  for (const id of ids) {
    const ring = Math.max(1, distances.get(id) ?? 1);
    byRing.set(ring, [...(byRing.get(ring) ?? []), id]);
  }

  // Direction away from the centroid of the existing graph (full circle if alone).
  let baseAngle = -Math.PI / 2;
  let arc = 2 * Math.PI;
  const others = existing.filter((p) => p.x !== anchor.x || p.y !== anchor.y);
  if (others.length > 0) {
    const cx = others.reduce((s, p) => s + p.x, 0) / others.length;
    const cy = others.reduce((s, p) => s + p.y, 0) / others.length;
    if (Math.hypot(anchor.x - cx, anchor.y - cy) > 1) {
      baseAngle = Math.atan2(anchor.y - cy, anchor.x - cx);
      arc = (2 * Math.PI * 200) / 360;
    }
  }

  const out = new Map<string, XYPosition>();
  for (const [ring, ringIds] of byRing) {
    // Grow the radius when many nodes share a ring so they do not overlap.
    const radius = Math.max(RING_SPACING * ring, (ringIds.length * MIN_ARC_GAP) / arc);
    const full = arc >= 2 * Math.PI - 1e-9;
    const step = full ? arc / ringIds.length : ringIds.length > 1 ? arc / (ringIds.length - 1) : 0;
    const start = full ? baseAngle : baseAngle - (ringIds.length > 1 ? arc / 2 : 0);
    ringIds.forEach((id, i) => {
      const angle = start + step * i;
      out.set(id, { x: anchor.x + radius * Math.cos(angle), y: anchor.y + radius * Math.sin(angle) });
    });
  }
  return out;
}

export function toFlowEdge(rel: GraphEdge): RelFlowEdge {
  return { id: rel.id, source: rel.source, target: rel.target, type: 'relationship', data: { rel } };
}

/**
 * Merges an API subgraph into the current React Flow state without duplicating
 * nodes or edges. Existing nodes keep their position; new nodes are laid out
 * around `anchorId`.
 */
export function mergeSubgraph(
  nodes: EntityFlowNode[],
  edges: RelFlowEdge[],
  sub: Subgraph,
  anchorId: string,
): MergeResult {
  const byId = new Map(nodes.map((n) => [n.id, n]));
  const anchorPos = byId.get(anchorId)?.position ?? { x: 0, y: 0 };

  const fresh = sub.nodes.filter((n) => !byId.has(n.id) && n.id !== anchorId);
  const positions = layoutAround(
    anchorPos,
    fresh.map((n) => n.id),
    hopDistances(anchorId, sub.edges),
    nodes.map((n) => n.position),
  );
  const incoming = new Map(sub.nodes.map((n) => [n.id, n]));

  const merged: EntityFlowNode[] = nodes.map((n) => {
    const update = incoming.get(n.id);
    if (!update && n.id !== anchorId) return n;
    return {
      ...n,
      data: { ...n.data, entity: update ?? n.data.entity, expanded: n.data.expanded || n.id === anchorId },
    };
  });
  if (!byId.has(anchorId)) {
    const anchor = incoming.get(anchorId);
    if (anchor) {
      merged.push({ id: anchorId, type: 'entity', position: anchorPos, data: { entity: anchor, expanded: true, isRoot: true } });
    }
  }
  for (const n of fresh) {
    merged.push({
      id: n.id,
      type: 'entity',
      position: positions.get(n.id) ?? anchorPos,
      data: { entity: n, expanded: false, isRoot: false },
    });
  }

  const nodeIds = new Set(merged.map((n) => n.id));
  const edgeIds = new Set(edges.map((e) => e.id));
  const newEdges = sub.edges
    .filter((e) => !edgeIds.has(e.id) && nodeIds.has(e.source) && nodeIds.has(e.target))
    .map(toFlowEdge);

  return {
    nodes: merged,
    edges: [...edges, ...newEdges],
    addedNodes: merged.length - nodes.length,
    addedEdges: newEdges.length,
  };
}

/** Sets `hidden` on nodes/edges according to the sidebar filters. */
export function applyFilters(
  nodes: EntityFlowNode[],
  edges: RelFlowEdge[],
  hiddenNodeTypes: ReadonlySet<NodeType>,
  hiddenRelTypes: ReadonlySet<RelType>,
): { nodes: EntityFlowNode[]; edges: RelFlowEdge[] } {
  const hiddenIds = new Set<string>();
  const outNodes = nodes.map((n) => {
    // The root stays visible so the user never loses the starting point.
    const hidden = !n.data.isRoot && hiddenNodeTypes.has(n.data.entity.type);
    if (hidden) hiddenIds.add(n.id);
    return n.hidden === hidden ? n : { ...n, hidden };
  });
  const outEdges = edges.map((e) => {
    const t = e.data?.rel.type;
    const hidden = hiddenIds.has(e.source) || hiddenIds.has(e.target) || (t !== undefined && hiddenRelTypes.has(t));
    return e.hidden === hidden ? e : { ...e, hidden };
  });
  return { nodes: outNodes, edges: outEdges };
}
