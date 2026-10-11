import { NODE_STYLE, PROPERTY_LABEL, REL_STYLE, formatProperty, nodeTitle } from '../features/graph/presentation';
import type { GraphEdge, GraphNode } from '../types/graph';

interface Props {
  entity: GraphNode | null;
  expanded: boolean;
  isRoot: boolean;
  connections: { edge: GraphEdge; other: GraphNode | undefined; outgoing: boolean }[];
  expanding: boolean;
  onExpand: () => void;
  onRecenter: () => void;
  onSelect: (id: string) => void;
}

export function DetailsPanel({ entity, expanded, isRoot, connections, expanding, onExpand, onRecenter, onSelect }: Props) {
  if (!entity) {
    return (
      <aside className="details" aria-label="Detalhes">
        <p className="details-empty">Selecione um nó para ver propriedades, conexões e ações.</p>
      </aside>
    );
  }
  const style = NODE_STYLE[entity.type];
  const keys = Object.keys(entity.properties);

  return (
    <aside className="details" aria-label="Detalhes da entidade">
      <header className="details-header" style={{ borderColor: style.color }}>
        <span className="details-type" style={{ color: style.color }}>
          {style.label}
        </span>
        <h2>{nodeTitle(entity)}</h2>
        <code>{entity.id}</code>
      </header>

      <div className="details-actions">
        <button type="button" className="btn btn-primary" onClick={onExpand} disabled={expanding}>
          {expanding ? 'Expandindo…' : expanded ? 'Expandir novamente' : 'Expandir conexões'}
        </button>
        {!isRoot && (
          <button type="button" className="btn" onClick={onRecenter} disabled={expanding}>
            Explorar a partir daqui
          </button>
        )}
      </div>

      <section className="panel-section">
        <h3>Propriedades</h3>
        {keys.length === 0 ? (
          <p className="hint">Sem propriedades exibíveis.</p>
        ) : (
          <dl className="props">
            {keys.map((k) => (
              <div key={k} className={k === 'text' ? 'prop-wide' : undefined}>
                <dt>{PROPERTY_LABEL[k] ?? k}</dt>
                <dd>{formatProperty(entity, k)}</dd>
              </div>
            ))}
          </dl>
        )}
      </section>

      <section className="panel-section">
        <h3>Conexões carregadas ({connections.length})</h3>
        {connections.length === 0 ? (
          <p className="hint">Nenhuma conexão carregada. Use “Expandir conexões”.</p>
        ) : (
          <ul className="connections">
            {connections.map(({ edge, other, outgoing }) => (
              <li key={edge.id}>
                <button type="button" onClick={() => onSelect(outgoing ? edge.target : edge.source)}>
                  <span className="conn-rel" style={{ color: REL_STYLE[edge.type].color }}>
                    {outgoing ? '→' : '←'} {edge.type}
                  </span>
                  <span className="conn-target">{other ? nodeTitle(other) : outgoing ? edge.target : edge.source}</span>
                </button>
              </li>
            ))}
          </ul>
        )}
        {!expanded && connections.length > 0 && <p className="hint">Pode haver conexões ainda não carregadas.</p>}
      </section>
    </aside>
  );
}
