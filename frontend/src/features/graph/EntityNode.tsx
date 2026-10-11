import { Handle, Position, type NodeProps } from '@xyflow/react';
import { memo } from 'react';
import type { EntityFlowNode } from './graphModel';
import { NODE_STYLE, nodeTitle } from './presentation';

// Handles are hidden: edges are drawn "floating" between node borders (RelationshipEdge).
function EntityNodeComponent({ data, selected }: NodeProps<EntityFlowNode>) {
  const { entity, expanded, isRoot } = data;
  const style = NODE_STYLE[entity.type];
  const flagged = entity.type === 'BankAccount' && entity.properties.status === 'under_review';

  return (
    <div
      className={`entity-node${selected ? ' is-selected' : ''}${isRoot ? ' is-root' : ''}`}
      style={{ '--node-color': style.color } as React.CSSProperties}
      title={`${style.label} · ${entity.id}`}
    >
      <Handle type="target" position={Position.Top} className="entity-handle" isConnectable={false} />
      <span className="entity-glyph" aria-hidden="true">
        {style.glyph}
      </span>
      <span className="entity-text">
        <span className="entity-type">{style.label}</span>
        <span className="entity-title">{nodeTitle(entity)}</span>
      </span>
      {flagged && <span className="entity-flag" title="Conta em revisão">!</span>}
      {!expanded && <span className="entity-more" title="Conexões ainda não expandidas" aria-label="não expandido" />}
      <Handle type="source" position={Position.Bottom} className="entity-handle" isConnectable={false} />
    </div>
  );
}

export const EntityNode = memo(EntityNodeComponent);
