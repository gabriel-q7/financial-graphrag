import {
  Background,
  Controls,
  MarkerType,
  MiniMap,
  Panel,
  ReactFlow,
  type EdgeTypes,
  type NodeTypes,
  type OnEdgesChange,
  type OnNodesChange,
} from '@xyflow/react';
import { EntityNode } from './EntityNode';
import type { EntityFlowNode, RelFlowEdge } from './graphModel';
import { NODE_STYLE } from './presentation';
import { RelationshipEdge } from './RelationshipEdge';

const nodeTypes: NodeTypes = { entity: EntityNode };
const edgeTypes: EdgeTypes = { relationship: RelationshipEdge };
const defaultEdgeOptions = { type: 'relationship', markerEnd: { type: MarkerType.ArrowClosed, width: 16, height: 16 } };

interface Props {
  nodes: EntityFlowNode[];
  edges: RelFlowEdge[];
  onNodesChange: OnNodesChange<EntityFlowNode>;
  onEdgesChange: OnEdgesChange<RelFlowEdge>;
  onSelect: (id: string | null) => void;
  onExpand: (id: string) => void;
  onCenter: () => void;
  stats: { nodes: number; edges: number; visibleNodes: number; visibleEdges: number };
}

export function GraphCanvas({ nodes, edges, onNodesChange, onEdgesChange, onSelect, onExpand, onCenter, stats }: Props) {
  return (
    <ReactFlow<EntityFlowNode, RelFlowEdge>
      nodes={nodes}
      edges={edges}
      nodeTypes={nodeTypes}
      edgeTypes={edgeTypes}
      defaultEdgeOptions={defaultEdgeOptions}
      onNodesChange={onNodesChange}
      onEdgesChange={onEdgesChange}
      onNodeClick={(_, n) => onSelect(n.id)}
      onNodeDoubleClick={(_, n) => onExpand(n.id)}
      onPaneClick={() => onSelect(null)}
      nodesConnectable={false}
      minZoom={0.1}
      maxZoom={2.5}
      colorMode="dark"
      proOptions={{ hideAttribution: false }}
    >
      <Background gap={24} size={1} />
      <Controls showInteractive={false} position="bottom-left" />
      <MiniMap
        pannable
        zoomable
        position="bottom-right"
        nodeColor={(n) => NODE_STYLE[(n as EntityFlowNode).data.entity.type].color}
      />
      <Panel position="top-left" className="canvas-panel">
        <button type="button" className="btn" onClick={onCenter}>
          Centralizar
        </button>
        <span className="canvas-stats" aria-live="polite">
          {stats.nodes} nós · {stats.edges} relacionamentos
          {(stats.visibleNodes !== stats.nodes || stats.visibleEdges !== stats.edges) &&
            ` (visíveis: ${stats.visibleNodes} · ${stats.visibleEdges})`}
        </span>
      </Panel>
    </ReactFlow>
  );
}
