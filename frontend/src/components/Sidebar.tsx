import { useEffect, useState } from 'react';
import { api } from '../lib/api';
import { NODE_STYLE, REL_STYLE, nodeTitle } from '../features/graph/presentation';
import { NODE_TYPES, REL_TYPES, type ApiLimits, type GraphNode, type NodeType, type RelType } from '../types/graph';

export interface ExpansionOptions {
  depth: number;
  limit: number;
}

interface Props {
  limits: ApiLimits;
  options: ExpansionOptions;
  onOptionsChange: (o: ExpansionOptions) => void;
  hiddenNodeTypes: ReadonlySet<NodeType>;
  hiddenRelTypes: ReadonlySet<RelType>;
  onToggleNodeType: (t: NodeType) => void;
  onToggleRelType: (t: RelType) => void;
  nodeCounts: Partial<Record<NodeType, number>>;
  edgeCounts: Partial<Record<RelType, number>>;
  onStart: (id: string) => void;
}

export function Sidebar(props: Props) {
  const { limits, options, onOptionsChange } = props;
  return (
    <aside className="sidebar" aria-label="Filtros e opções">
      <EntitySearch onStart={props.onStart} />

      <section className="panel-section">
        <h2>Tipos de entidade</h2>
        <ul className="filter-list">
          {NODE_TYPES.map((t) => (
            <li key={t}>
              <label>
                <input type="checkbox" checked={!props.hiddenNodeTypes.has(t)} onChange={() => props.onToggleNodeType(t)} />
                <span className="swatch" style={{ background: NODE_STYLE[t].color }} />
                {NODE_STYLE[t].label}
                <span className="count">{props.nodeCounts[t] ?? 0}</span>
              </label>
            </li>
          ))}
        </ul>
      </section>

      <section className="panel-section">
        <h2>Relacionamentos</h2>
        <ul className="filter-list">
          {REL_TYPES.map((t) => (
            <li key={t}>
              <label>
                <input type="checkbox" checked={!props.hiddenRelTypes.has(t)} onChange={() => props.onToggleRelType(t)} />
                <span
                  className={`swatch swatch-line${REL_STYLE[t].dashed ? ' dashed' : ''}`}
                  style={{ borderColor: REL_STYLE[t].color }}
                />
                <code>{t}</code>
                <span className="count">{props.edgeCounts[t] ?? 0}</span>
              </label>
            </li>
          ))}
        </ul>
        <p className="hint">Tipos desmarcados também são excluídos das próximas expansões.</p>
      </section>

      <section className="panel-section">
        <h2>Expansão</h2>
        <label className="field">
          Profundidade
          <select value={options.depth} onChange={(e) => onOptionsChange({ ...options, depth: Number(e.target.value) })}>
            {Array.from({ length: limits.maxDepth }, (_, i) => i + 1).map((d) => (
              <option key={d} value={d}>
                {d} {d === 1 ? 'nível' : 'níveis'}
              </option>
            ))}
          </select>
        </label>
        <label className="field">
          Limite de nós (máx. {limits.maxLimit})
          <input
            type="number"
            min={1}
            max={limits.maxLimit}
            value={options.limit}
            onChange={(e) => {
              const n = Math.round(Number(e.target.value));
              if (Number.isFinite(n)) onOptionsChange({ ...options, limit: Math.min(limits.maxLimit, Math.max(1, n)) });
            }}
          />
        </label>
        <p className="hint">Duplo clique em um nó para expandir suas conexões.</p>
      </section>
    </aside>
  );
}

function EntitySearch({ onStart }: { onStart: (id: string) => void }) {
  const [q, setQ] = useState('');
  const [results, setResults] = useState<GraphNode[]>([]);
  const [error, setError] = useState<string | null>(null);
  const [loading, setLoading] = useState(false);

  useEffect(() => {
    const text = q.trim();
    if (text.length < 2) {
      setResults([]);
      setError(null);
      return;
    }
    const ctrl = new AbortController();
    const timer = setTimeout(() => {
      setLoading(true);
      api
        .search(text, ctrl.signal)
        .then((r) => {
          setResults(r.items);
          setError(null);
        })
        .catch((err: unknown) => {
          if (!ctrl.signal.aborted) setError(err instanceof Error ? err.message : 'Falha na busca.');
        })
        .finally(() => {
          if (!ctrl.signal.aborted) setLoading(false);
        });
    }, 250);
    return () => {
      clearTimeout(timer);
      ctrl.abort();
    };
  }, [q]);

  return (
    <section className="panel-section">
      <h2>Entidade inicial</h2>
      <input
        type="search"
        className="search"
        placeholder="Buscar por nome ou id (ex.: Ana, acc_007)"
        value={q}
        maxLength={100}
        onChange={(e) => setQ(e.target.value)}
        aria-label="Buscar entidade"
      />
      {loading && <p className="hint">Buscando…</p>}
      {error && (
        <p className="hint error" role="alert">
          {error}
        </p>
      )}
      {!loading && !error && q.trim().length >= 2 && results.length === 0 && <p className="hint">Nenhuma entidade encontrada.</p>}
      {results.length > 0 && (
        <ul className="search-results">
          {results.map((n) => (
            <li key={n.id}>
              <button
                type="button"
                onClick={() => {
                  onStart(n.id);
                  setQ('');
                }}
              >
                <span className="swatch" style={{ background: NODE_STYLE[n.type].color }} />
                <span className="result-title">{nodeTitle(n)}</span>
                <code>{n.id}</code>
              </button>
            </li>
          ))}
        </ul>
      )}
    </section>
  );
}
