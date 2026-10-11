import { describe, expect, it } from 'vitest';
import type { GraphEdge, GraphNode, Subgraph } from '../../types/graph';
import { applyFilters, hopDistances, layoutAround, mergeSubgraph, type EntityFlowNode, type RelFlowEdge } from './graphModel';

const node = (id: string, type: GraphNode['type'] = 'Person'): GraphNode => ({ id, type, properties: {} });
const edge = (source: string, type: GraphEdge['type'], target: string): GraphEdge => ({
  id: `${source}:${type}:${target}`,
  type,
  source,
  target,
});
const sub = (center: GraphNode, nodes: GraphNode[], edges: GraphEdge[]): Subgraph => ({
  center,
  nodes,
  edges,
  meta: { depth: 1, nodeLimit: 25, edgeLimit: 75, relationshipTypes: [], nodeCount: nodes.length, edgeCount: edges.length, truncated: false },
});

describe('mergeSubgraph', () => {
  const per = node('per_001');
  const acc = node('acc_001', 'BankAccount');
  const cmp = node('cmp_001', 'Company');
  const first = sub(per, [per, acc, cmp], [edge('per_001', 'OWNS', 'acc_001'), edge('per_001', 'CONTROLS', 'cmp_001')]);

  it('builds the initial graph with the root at the origin', () => {
    const r = mergeSubgraph([], [], first, 'per_001');
    expect(r.nodes.map((n) => n.id).sort()).toEqual(['acc_001', 'cmp_001', 'per_001']);
    expect(r.edges).toHaveLength(2);
    const root = r.nodes.find((n) => n.id === 'per_001')!;
    expect(root.position).toEqual({ x: 0, y: 0 });
    expect(root.data.isRoot).toBe(true);
    expect(root.data.expanded).toBe(true);
  });

  it('does not duplicate nodes or edges on repeated expansion', () => {
    const a = mergeSubgraph([], [], first, 'per_001');
    const b = mergeSubgraph(a.nodes, a.edges, first, 'per_001');
    expect(b.nodes).toHaveLength(3);
    expect(b.edges).toHaveLength(2);
    expect(b.addedNodes).toBe(0);
    expect(b.addedEdges).toBe(0);
  });

  it('keeps positions of existing nodes and marks the anchor as expanded', () => {
    const a = mergeSubgraph([], [], first, 'per_001');
    const moved = a.nodes.map((n) => (n.id === 'acc_001' ? { ...n, position: { x: 500, y: 500 } } : n));
    const txn = node('txn_001', 'Transaction');
    const second = sub(acc, [acc, txn, per], [edge('acc_001', 'SENT', 'txn_001'), edge('per_001', 'OWNS', 'acc_001')]);
    const b = mergeSubgraph(moved, a.edges, second, 'acc_001');
    const accNode = b.nodes.find((n) => n.id === 'acc_001')!;
    expect(accNode.position).toEqual({ x: 500, y: 500 });
    expect(accNode.data.expanded).toBe(true);
    expect(b.addedNodes).toBe(1);
    expect(b.addedEdges).toBe(1);
    expect(b.nodes.find((n) => n.id === 'txn_001')!.position).not.toEqual({ x: 500, y: 500 });
  });

  it('ignores edges whose endpoints are not loaded', () => {
    const r = mergeSubgraph([], [], sub(per, [per], [edge('per_001', 'OWNS', 'acc_999')]), 'per_001');
    expect(r.edges).toHaveLength(0);
  });
});

describe('layout helpers', () => {
  it('computes hop distances ignoring direction', () => {
    const d = hopDistances('a', [edge('b', 'SENT', 'a'), edge('b', 'TO_ACCOUNT', 'c')]);
    expect(d.get('b')).toBe(1);
    expect(d.get('c')).toBe(2);
  });

  it('places nodes on distinct positions per ring', () => {
    const pos = layoutAround({ x: 0, y: 0 }, ['x', 'y', 'z'], new Map([['x', 1], ['y', 1], ['z', 2]]), []);
    const r = (id: string) => Math.hypot(pos.get(id)!.x, pos.get(id)!.y);
    expect(r('x')).toBeCloseTo(r('y'));
    expect(r('z')).toBeGreaterThan(r('x'));
    expect(pos.get('x')).not.toEqual(pos.get('y'));
  });
});

describe('applyFilters', () => {
  const nodes: EntityFlowNode[] = [
    { id: 'per_001', type: 'entity', position: { x: 0, y: 0 }, data: { entity: node('per_001'), expanded: true, isRoot: true } },
    { id: 'dev_001', type: 'entity', position: { x: 0, y: 0 }, data: { entity: node('dev_001', 'Device'), expanded: false, isRoot: false } },
    { id: 'acc_001', type: 'entity', position: { x: 0, y: 0 }, data: { entity: node('acc_001', 'BankAccount'), expanded: false, isRoot: false } },
  ];
  const rels = [edge('per_001', 'USES', 'dev_001'), edge('per_001', 'OWNS', 'acc_001')];
  const edges: RelFlowEdge[] = rels.map((rel) => ({ id: rel.id, source: rel.source, target: rel.target, data: { rel } }));

  it('hides nodes of filtered types and their edges, never the root', () => {
    const r = applyFilters(nodes, edges, new Set(['Device', 'Person']), new Set());
    expect(r.nodes.find((n) => n.id === 'dev_001')!.hidden).toBe(true);
    expect(r.nodes.find((n) => n.id === 'per_001')!.hidden).toBe(false);
    expect(r.edges.find((e) => e.target === 'dev_001')!.hidden).toBe(true);
    expect(r.edges.find((e) => e.target === 'acc_001')!.hidden).toBe(false);
  });

  it('hides edges of filtered relationship types', () => {
    const r = applyFilters(nodes, edges, new Set(), new Set(['OWNS']));
    expect(r.edges.find((e) => e.target === 'acc_001')!.hidden).toBe(true);
  });
});
