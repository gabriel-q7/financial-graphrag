import {
  BaseEdge,
  EdgeLabelRenderer,
  getStraightPath,
  useInternalNode,
  type EdgeProps,
  type InternalNode,
  type XYPosition,
} from '@xyflow/react';
import { memo } from 'react';
import type { RelFlowEdge } from './graphModel';
import { REL_STYLE } from './presentation';

/** Point where the line between two node centers crosses the border of `node`. */
function borderPoint(node: InternalNode, other: InternalNode): XYPosition {
  const w = (node.measured.width ?? 0) / 2;
  const h = (node.measured.height ?? 0) / 2;
  const p = node.internals.positionAbsolute;
  const o = other.internals.positionAbsolute;
  const cx = p.x + w;
  const cy = p.y + h;
  const ox = o.x + (other.measured.width ?? 0) / 2;
  const oy = o.y + (other.measured.height ?? 0) / 2;
  if (w === 0 || h === 0) return { x: cx, y: cy };
  const xx1 = (ox - cx) / (2 * w) - (oy - cy) / (2 * h);
  const yy1 = (ox - cx) / (2 * w) + (oy - cy) / (2 * h);
  const a = 1 / (Math.abs(xx1) + Math.abs(yy1) || 1);
  const xx3 = a * xx1;
  const yy3 = a * yy1;
  return { x: w * (xx3 + yy3) + cx, y: h * (-xx3 + yy3) + cy };
}

function RelationshipEdgeComponent({ id, source, target, data, markerEnd, selected }: EdgeProps<RelFlowEdge>) {
  const sourceNode = useInternalNode(source);
  const targetNode = useInternalNode(target);
  if (!sourceNode || !targetNode || !data) return null;

  const s = borderPoint(sourceNode, targetNode);
  const t = borderPoint(targetNode, sourceNode);
  const [path, labelX, labelY] = getStraightPath({ sourceX: s.x, sourceY: s.y, targetX: t.x, targetY: t.y });
  const style = REL_STYLE[data.rel.type];

  return (
    <>
      <BaseEdge
        id={id}
        path={path}
        {...(markerEnd ? { markerEnd } : {})}
        style={{
          stroke: style.color,
          strokeWidth: selected ? 2.5 : 1.5,
          strokeDasharray: style.dashed ? '6 4' : undefined,
        }}
      />
      <EdgeLabelRenderer>
        <div
          className="edge-label nodrag nopan"
          style={{ transform: `translate(-50%, -50%) translate(${labelX}px, ${labelY}px)`, color: style.color }}
          title={data.rel.type}
        >
          {data.rel.type}
        </div>
      </EdgeLabelRenderer>
    </>
  );
}

export const RelationshipEdge = memo(RelationshipEdgeComponent);
