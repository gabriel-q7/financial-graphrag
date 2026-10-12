import { useEdgesState, useNodesInitialized, useNodesState, useReactFlow } from '@xyflow/react';
import { useCallback, useEffect, useMemo, useRef, useState } from 'react';
import { DetailsPanel } from '../../components/DetailsPanel';
import { HealthBadge } from '../../components/HealthBadge';
import { Sidebar, type ExpansionOptions } from '../../components/Sidebar';
import { StateMessage } from '../../components/StateMessage';
import { AskPanel } from '../ask/AskPanel';
import { ApiError, api } from '../../lib/api';
import type { Retrieval } from '../../types/ask';
import { REL_TYPES, type ApiMeta, type HealthReport, type NodeType, type RelType, type Subgraph } from '../../types/graph';
import { GraphCanvas } from './GraphCanvas';
import { applyFilters, mergeSubgraph, type EntityFlowNode, type RelFlowEdge } from './graphModel';

const DEFAULT_ENTITY = import.meta.env.VITE_DEFAULT_ENTITY_ID ?? 'per_001';
const HEALTH_INTERVAL_MS = 30_000;

type LoadState = { status: 'idle' | 'loading' | 'ready' } | { status: 'error'; message: string; retry: () => void };

const ERROR_MESSAGES: Record<string, string> = {
  not_found: 'Entidade não encontrada. Execute o seed ou busque outra entidade.',
  graph_unavailable: 'O banco de grafos (Neo4j) está indisponível. Verifique se o contêiner está em execução.',
  timeout: 'A consulta ao grafo excedeu o tempo limite. Tente reduzir a profundidade ou o limite.',
  invalid_parameter: 'Parâmetro inválido.',
};

function errorMessage(err: unknown): string {
  if (err instanceof ApiError) {
    const known = ERROR_MESSAGES[err.code];
    if (err.code === 'invalid_parameter') return `${known} ${err.message}`;
    return known ?? err.message;
  }
  return 'Erro inesperado.';
}

/** Builds a Subgraph from the evidence of an answer so it can be merged into the canvas. */
function evidenceSubgraph(r: Retrieval, anchorId: string): Subgraph | null {
  const nodes = [...r.nodes, ...r.documents.map((d) => d.node)];
  const center = nodes.find((n) => n.id === anchorId);
  if (!center) return null;
  return {
    center,
    nodes,
    edges: r.edges,
    meta: { depth: 0, nodeLimit: nodes.length, edgeLimit: r.edges.length, relationshipTypes: [], nodeCount: nodes.length, edgeCount: r.edges.length, truncated: false },
  };
}

function toggle<T>(set: ReadonlySet<T>, v: T): Set<T> {
  const next = new Set(set);
  if (next.has(v)) next.delete(v);
  else next.add(v);
  return next;
}

export function GraphExplorer() {
  const [meta, setMeta] = useState<ApiMeta | null>(null);
  const [metaState, setMetaState] = useState<LoadState>({ status: 'loading' });
  const [health, setHealth] = useState<HealthReport | null>(null);
  const [healthError, setHealthError] = useState<string | null>(null);

  const [nodes, setNodes, onNodesChange] = useNodesState<EntityFlowNode>([]);
  const [edges, setEdges, onEdgesChange] = useEdgesState<RelFlowEdge>([]);
  const [graphState, setGraphState] = useState<LoadState>({ status: 'idle' });
  const [selectedId, setSelectedId] = useState<string | null>(null);
  const [expandingId, setExpandingId] = useState<string | null>(null);
  const [notice, setNotice] = useState<string | null>(null);

  const [hiddenNodeTypes, setHiddenNodeTypes] = useState<ReadonlySet<NodeType>>(new Set());
  const [hiddenRelTypes, setHiddenRelTypes] = useState<ReadonlySet<RelType>>(new Set());
  const [options, setOptions] = useState<ExpansionOptions>({ depth: 1, limit: 25 });
  const [rightTab, setRightTab] = useState<'ask' | 'details'>('ask');
  const [citedIds, setCitedIds] = useState<ReadonlySet<string>>(new Set());

  const { fitView, setCenter, getNode } = useReactFlow<EntityFlowNode, RelFlowEdge>();
  const requestRef = useRef<AbortController | null>(null);

  // Keep the latest graph in a ref so async callbacks merge into fresh state.
  const graphRef = useRef({ nodes, edges });
  graphRef.current = { nodes, edges };

  const fitAll = useCallback(() => void fitView({ padding: 0.2, duration: 300 }), [fitView]);

  // Fitting must wait until React Flow has measured the new nodes.
  // null = fit everything; a list = fit those nodes (anchor + new neighbors).
  const pendingFit = useRef<string[] | null | undefined>(undefined);
  const nodesInitialized = useNodesInitialized();
  useEffect(() => {
    if (!nodesInitialized || pendingFit.current === undefined) return;
    const ids = pendingFit.current;
    pendingFit.current = undefined;
    void fitView(
      ids ? { nodes: ids.map((id) => ({ id })), padding: 0.3, maxZoom: 1.2, duration: 300 } : { padding: 0.2, duration: 300 },
    );
  }, [nodesInitialized, nodes.length, fitView]);

  // --- Health polling -------------------------------------------------------
  useEffect(() => {
    const ctrl = new AbortController();
    const check = () =>
      api
        .health(ctrl.signal)
        .then((h) => {
          setHealth(h);
          setHealthError(null);
        })
        .catch((err: unknown) => {
          if (!ctrl.signal.aborted) setHealthError(errorMessage(err));
        });
    void check();
    const timer = setInterval(check, HEALTH_INTERVAL_MS);
    return () => {
      clearInterval(timer);
      ctrl.abort();
    };
  }, []);

  // --- Root loading ---------------------------------------------------------
  const loadRoot = useCallback(
    (id: string, limit: number) => {
      requestRef.current?.abort();
      const ctrl = new AbortController();
      requestRef.current = ctrl;
      setGraphState({ status: 'loading' });
      setNotice(null);
      api
        .entity(id, limit, ctrl.signal)
        .then((sub) => {
          const merged = mergeSubgraph([], [], sub, sub.center.id);
          setNodes(merged.nodes);
          setEdges(merged.edges);
          setSelectedId(sub.center.id);
          setGraphState({ status: 'ready' });
          if (sub.meta.truncated) setNotice(`Resultado limitado a ${sub.meta.nodeLimit} vizinhos.`);
          pendingFit.current = null;
        })
        .catch((err: unknown) => {
          if (ctrl.signal.aborted) return;
          setNodes([]);
          setEdges([]);
          setSelectedId(null);
          setGraphState({ status: 'error', message: errorMessage(err), retry: () => loadRoot(id, limit) });
        });
    },
    [setEdges, setNodes],
  );

  const loadMeta = useCallback(() => {
    setMetaState({ status: 'loading' });
    api
      .meta()
      .then((m) => {
        setMeta(m);
        setOptions({ depth: m.limits.defaultDepth, limit: m.limits.defaultLimit });
        setMetaState({ status: 'ready' });
        loadRoot(DEFAULT_ENTITY, m.limits.defaultLimit);
      })
      .catch((err: unknown) => setMetaState({ status: 'error', message: errorMessage(err), retry: loadMeta }));
  }, [loadRoot]);

  useEffect(() => {
    loadMeta();
    return () => requestRef.current?.abort();
  }, [loadMeta]);

  // --- Expansion ------------------------------------------------------------
  const expand = useCallback(
    (id: string) => {
      if (expandingId) return;
      const ctrl = new AbortController();
      requestRef.current = ctrl;
      const relationshipTypes = hiddenRelTypes.size === 0 ? [] : REL_TYPES.filter((t) => !hiddenRelTypes.has(t));
      setExpandingId(id);
      setNotice(null);
      api
        .neighbors(id, { ...options, relationshipTypes }, ctrl.signal)
        .then((sub) => {
          const { nodes: cur, edges: curEdges } = graphRef.current;
          const merged = mergeSubgraph(cur, curEdges, sub, id);
          if (merged.addedNodes > 0) {
            const known = new Set(cur.map((n) => n.id));
            pendingFit.current = [id, ...merged.nodes.filter((n) => !known.has(n.id)).map((n) => n.id)];
          }
          setNodes(merged.nodes);
          setEdges(merged.edges);
          const parts = [
            merged.addedNodes + merged.addedEdges === 0
              ? 'Nenhuma conexão nova encontrada.'
              : `+${merged.addedNodes} nós, +${merged.addedEdges} relacionamentos.`,
          ];
          if (sub.meta.truncated) parts.push(`Resultado limitado pelo servidor (limite ${sub.meta.nodeLimit}).`);
          setNotice(parts.join(' '));
        })
        .catch((err: unknown) => {
          if (!ctrl.signal.aborted) setNotice(`Falha ao expandir: ${errorMessage(err)}`);
        })
        .finally(() => setExpandingId(null));
    },
    [expandingId, hiddenRelTypes, options, setEdges, setNodes],
  );

  // --- Derived view state ---------------------------------------------------
  const view = useMemo(() => {
    const filtered = applyFilters(nodes, edges, hiddenNodeTypes, hiddenRelTypes);
    if (citedIds.size === 0) return filtered;
    return {
      ...filtered,
      nodes: filtered.nodes.map((n) => (citedIds.has(n.id) ? { ...n, className: 'is-cited' } : n)),
    };
  }, [nodes, edges, hiddenNodeTypes, hiddenRelTypes, citedIds]);

  const stats = useMemo(
    () => ({
      nodes: nodes.length,
      edges: edges.length,
      visibleNodes: view.nodes.filter((n) => !n.hidden).length,
      visibleEdges: view.edges.filter((e) => !e.hidden).length,
    }),
    [nodes.length, edges.length, view],
  );

  const counts = useMemo(() => {
    const nodeCounts: Partial<Record<NodeType, number>> = {};
    const edgeCounts: Partial<Record<RelType, number>> = {};
    for (const n of nodes) nodeCounts[n.data.entity.type] = (nodeCounts[n.data.entity.type] ?? 0) + 1;
    for (const e of edges) if (e.data) edgeCounts[e.data.rel.type] = (edgeCounts[e.data.rel.type] ?? 0) + 1;
    return { nodeCounts, edgeCounts };
  }, [nodes, edges]);

  const selected = useMemo(() => nodes.find((n) => n.id === selectedId) ?? null, [nodes, selectedId]);
  const connections = useMemo(() => {
    if (!selected) return [];
    const byId = new Map(nodes.map((n) => [n.id, n.data.entity]));
    return edges
      .filter((e) => e.data && (e.source === selected.id || e.target === selected.id))
      .map((e) => {
        const outgoing = e.source === selected.id;
        return { edge: e.data!.rel, outgoing, other: byId.get(outgoing ? e.target : e.source) };
      });
  }, [selected, nodes, edges]);

  const select = useCallback(
    (id: string | null) => {
      setSelectedId(id);
      setNodes((ns) => ns.map((n) => (n.selected === (n.id === id) ? n : { ...n, selected: n.id === id })));
      const n = id ? getNode(id) : undefined;
      if (n && id) {
        void setCenter(n.position.x + (n.measured?.width ?? 0) / 2, n.position.y + (n.measured?.height ?? 0) / 2, {
          zoom: 1.1,
          duration: 300,
        });
      }
    },
    [getNode, setCenter, setNodes],
  );

  // --- Evidence from answers -------------------------------------------------
  const showEvidence = useCallback(
    (r: Retrieval, cited: string[], focusId?: string) => {
      const anchor = focusId ?? cited[0] ?? r.entities[0]?.id ?? r.documents[0]?.node.id;
      const sub = anchor ? evidenceSubgraph(r, anchor) : null;
      if (!sub) return;
      requestRef.current?.abort();
      const merged = mergeSubgraph([], [], sub, sub.center.id);
      setNodes(merged.nodes.map((n) => ({ ...n, selected: n.id === sub.center.id })));
      setEdges(merged.edges);
      setSelectedId(sub.center.id);
      setCitedIds(new Set(cited));
      setGraphState({ status: 'ready' });
      setNotice(`Exibindo as evidências da resposta: ${merged.nodes.length} nós, ${merged.edges.length} relacionamentos.`);
      pendingFit.current = null;
    },
    [setEdges, setNodes],
  );

  const focusCitation = useCallback(
    (id: string, r: Retrieval) => {
      if (graphRef.current.nodes.some((n) => n.id === id)) {
        select(id);
        return;
      }
      // Not on the canvas yet: show the evidence graph centered on the cited entity.
      showEvidence(r, [...citedIds, id], id);
    },
    [citedIds, select, showEvidence],
  );

  // --- Render ---------------------------------------------------------------
  if (metaState.status === 'loading') {
    return <StateMessage kind="loading" title="Conectando à API…" />;
  }
  if (metaState.status === 'error' || !meta) {
    return (
      <StateMessage
        kind="error"
        title="Não foi possível carregar a aplicação"
        action={{ label: 'Tentar novamente', onClick: loadMeta }}
      >
        <p>{metaState.status === 'error' ? metaState.message : 'Metadados indisponíveis.'}</p>
        <p>Verifique se o backend está em execução (veja o README).</p>
      </StateMessage>
    );
  }

  return (
    <div className="layout">
      <header className="topbar">
        <h1>
          Financial GraphRAG <span>explorador de grafo</span>
        </h1>
        <HealthBadge health={health} error={healthError} />
      </header>

      <Sidebar
        limits={meta.limits}
        options={options}
        onOptionsChange={setOptions}
        hiddenNodeTypes={hiddenNodeTypes}
        hiddenRelTypes={hiddenRelTypes}
        onToggleNodeType={(t) => setHiddenNodeTypes((s) => toggle(s, t))}
        onToggleRelType={(t) => setHiddenRelTypes((s) => toggle(s, t))}
        nodeCounts={counts.nodeCounts}
        edgeCounts={counts.edgeCounts}
        onStart={(id) => loadRoot(id, options.limit)}
      />

      <main className="canvas" aria-label="Grafo">
        {graphState.status === 'loading' && <StateMessage kind="loading" title="Carregando grafo…" />}
        {graphState.status === 'error' && (
          <StateMessage kind="error" title="Falha ao carregar o grafo" action={{ label: 'Tentar novamente', onClick: graphState.retry }}>
            <p>{graphState.message}</p>
          </StateMessage>
        )}
        {graphState.status === 'ready' && nodes.length === 0 && (
          <StateMessage kind="empty" title="Grafo vazio">
            <p>Busque uma entidade na barra lateral para começar.</p>
          </StateMessage>
        )}
        {graphState.status === 'ready' && nodes.length > 0 && (
          <>
            <GraphCanvas
              nodes={view.nodes}
              edges={view.edges}
              onNodesChange={onNodesChange}
              onEdgesChange={onEdgesChange}
              onSelect={(id) => {
                select(id);
                if (id) setRightTab('details');
              }}
              onExpand={expand}
              onCenter={fitAll}
              stats={stats}
            />
            {(notice || expandingId) && (
              <div className="toast" role="status" aria-live="polite">
                {expandingId ? `Expandindo ${expandingId}…` : notice}
                {!expandingId && (
                  <button type="button" className="toast-close" onClick={() => setNotice(null)} aria-label="Fechar">
                    ×
                  </button>
                )}
              </div>
            )}
          </>
        )}
      </main>

      <aside className="details" aria-label="Painel lateral">
        <div className="tabs" role="tablist">
          <button type="button" role="tab" aria-selected={rightTab === 'ask'} className={rightTab === 'ask' ? 'active' : ''} onClick={() => setRightTab('ask')}>
            Perguntar à IA
          </button>
          <button type="button" role="tab" aria-selected={rightTab === 'details'} className={rightTab === 'details' ? 'active' : ''} onClick={() => setRightTab('details')}>
            Detalhes
          </button>
        </div>
        <div role="tabpanel" hidden={rightTab !== 'ask'}>
          <AskPanel
            aiAvailable={health?.checks.ollama?.status === 'ok'}
            onCitation={focusCitation}
            onShowEvidence={(r, cited) => showEvidence(r, cited)}
            onCited={(ids) => setCitedIds(new Set(ids))}
          />
        </div>
        <div role="tabpanel" hidden={rightTab !== 'details'}>
        <DetailsPanel
          entity={selected?.data.entity ?? null}
          expanded={selected?.data.expanded ?? false}
          isRoot={selected?.data.isRoot ?? false}
          connections={connections}
          expanding={expandingId !== null}
          onExpand={() => selected && expand(selected.id)}
          onRecenter={() => selected && loadRoot(selected.id, options.limit)}
          onSelect={select}
        />
        </div>
      </aside>
    </div>
  );
}
