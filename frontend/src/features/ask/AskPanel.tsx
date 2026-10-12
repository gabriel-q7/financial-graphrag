import { useEffect, useRef, useState } from 'react';
import { ApiError, api } from '../../lib/api';
import type { AskAnswer, Retrieval } from '../../types/ask';
import type { GraphNode } from '../../types/graph';
import { NODE_STYLE, nodeTitle } from '../graph/presentation';
import { splitAnswer } from './citations';

const EXAMPLES = [
  'Por que a conta da Ana Ribeiro merece revisão?',
  'Quais pessoas compartilham o mesmo dispositivo?',
  'Para onde foi o dinheiro que a conta acc_007 recebeu?',
];

type Phase = 'idle' | 'retrieving' | 'reading' | 'generating' | 'done' | 'error';

const ERRORS: Record<string, string> = {
  busy: 'Outra pergunta está sendo respondida. Aguarde ela terminar.',
  model_unavailable: 'O modelo de IA está indisponível. Verifique se o Ollama está rodando com os modelos baixados (make models).',
  graph_unavailable: 'O banco de grafos (Neo4j) está indisponível.',
  timeout: 'A resposta excedeu o tempo limite.',
  invalid_parameter: 'A pergunta deve ter entre 3 e 500 caracteres.',
};

interface Props {
  /** Ollama health; questions need it. */
  aiAvailable: boolean;
  /** Focus an entity cited in the answer or listed as evidence. */
  onCitation: (id: string, retrieval: Retrieval) => void;
  onShowEvidence: (retrieval: Retrieval, cited: string[]) => void;
  onCited: (ids: string[]) => void;
}

export function AskPanel({ aiAvailable, onCitation, onShowEvidence, onCited }: Props) {
  const [question, setQuestion] = useState('');
  const [phase, setPhase] = useState<Phase>('idle');
  const [retrieval, setRetrieval] = useState<Retrieval | null>(null);
  const [text, setText] = useState('');
  const [result, setResult] = useState<AskAnswer | null>(null);
  const [error, setError] = useState<string | null>(null);
  const [elapsed, setElapsed] = useState(0);
  const ctrlRef = useRef<AbortController | null>(null);

  const running = phase === 'retrieving' || phase === 'reading' || phase === 'generating';

  useEffect(() => {
    if (!running) return;
    const start = Date.now();
    const timer = setInterval(() => setElapsed(Math.round((Date.now() - start) / 1000)), 500);
    return () => clearInterval(timer);
  }, [running]);

  useEffect(() => () => ctrlRef.current?.abort(), []);

  const submit = (q: string) => {
    const trimmed = q.trim();
    if (trimmed.length < 3 || running) return;
    ctrlRef.current?.abort();
    const ctrl = new AbortController();
    ctrlRef.current = ctrl;
    setQuestion(trimmed);
    setPhase('retrieving');
    setRetrieval(null);
    setText('');
    setResult(null);
    setError(null);
    setElapsed(0);
    onCited([]);

    let ended = false;
    api
      .ask(
        trimmed,
        (e) => {
          switch (e.type) {
            case 'retrieval':
              setRetrieval(e.data);
              setPhase(e.data.noEvidence ? 'generating' : 'reading');
              break;
            case 'status':
              setPhase('reading');
              break;
            case 'token':
              setPhase('generating');
              setText((t) => t + e.data.text);
              break;
            case 'done':
              ended = true;
              setResult(e.data);
              setText(e.data.answer);
              setPhase('done');
              onCited(e.data.citations.valid);
              break;
            case 'error':
              ended = true;
              setError(ERRORS[e.data.code] ?? e.data.message);
              setPhase('error');
              break;
          }
        },
        ctrl.signal,
      )
      .then(() => {
        if (!ended && !ctrl.signal.aborted) {
          setError('A conexão terminou antes da resposta completa.');
          setPhase('error');
        }
      })
      .catch((err: unknown) => {
        if (ctrl.signal.aborted) return;
        setError(err instanceof ApiError ? (ERRORS[err.code] ?? err.message) : 'Erro inesperado.');
        setPhase('error');
      });
  };

  const cancel = () => {
    ctrlRef.current?.abort();
    setPhase(text ? 'done' : 'idle');
    if (!result) setError('Pergunta cancelada.');
  };

  const nodesById = new Map<string, GraphNode>();
  for (const n of retrieval?.nodes ?? []) nodesById.set(n.id, n);
  for (const d of retrieval?.documents ?? []) nodesById.set(d.node.id, d.node);
  const invalid = new Set(result?.citations.invalid ?? []);

  return (
    <div className="ask">
      <form
        className="ask-form"
        onSubmit={(e) => {
          e.preventDefault();
          submit(question);
        }}
      >
        <label htmlFor="ask-input" className="ask-label">
          Pergunte sobre o grafo
        </label>
        <textarea
          id="ask-input"
          value={question}
          maxLength={500}
          rows={3}
          placeholder="Ex.: Por que a conta da Ana Ribeiro merece revisão?"
          onChange={(e) => setQuestion(e.target.value)}
          onKeyDown={(e) => {
            if (e.key === 'Enter' && !e.shiftKey) {
              e.preventDefault();
              submit(question);
            }
          }}
          disabled={running}
        />
        <div className="ask-actions">
          {running ? (
            <button type="button" className="btn" onClick={cancel}>
              Cancelar
            </button>
          ) : (
            <button type="submit" className="btn btn-primary" disabled={question.trim().length < 3}>
              Perguntar
            </button>
          )}
          <span className="hint">{question.length}/500</span>
        </div>
        {!aiAvailable && (
          <p className="hint error">O Ollama não está disponível ou faltam modelos; as perguntas podem falhar.</p>
        )}
      </form>

      {phase === 'idle' && !error && (
        <div className="panel-section">
          <h3>Exemplos</h3>
          <ul className="ask-examples">
            {EXAMPLES.map((q) => (
              <li key={q}>
                <button type="button" onClick={() => submit(q)}>
                  {q}
                </button>
              </li>
            ))}
          </ul>
          <p className="hint">
            A IA busca documentos parecidos com a pergunta, percorre o grafo a partir deles e das entidades citadas e
            responde só com essas evidências. No processador do notebook, cada resposta leva cerca de 40–90 s.
          </p>
        </div>
      )}

      {running && (
        <div className="ask-progress" role="status" aria-live="polite">
          <span className="spinner small" aria-hidden="true" />
          {phase === 'retrieving' && 'Buscando evidências no grafo…'}
          {phase === 'reading' && 'O modelo está lendo as evidências…'}
          {phase === 'generating' && 'Gerando resposta…'}
          <span className="ask-elapsed">{elapsed}s</span>
        </div>
      )}

      {error && (
        <p className="ask-error" role="alert">
          {error}
        </p>
      )}

      {text && (
        <div className="panel-section">
          <h3>Resposta</h3>
          <p className="ask-answer">
            {splitAnswer(text).map((part, i) =>
              part.kind === 'text' ? (
                <span key={i}>{part.text}</span>
              ) : (
                <button
                  key={i}
                  type="button"
                  className={`cite${invalid.has(part.id) ? ' cite-invalid' : ''}`}
                  title={
                    invalid.has(part.id)
                      ? 'Id citado pela IA que não está nas evidências'
                      : nodesById.get(part.id)
                        ? nodeTitle(nodesById.get(part.id)!)
                        : part.id
                  }
                  onClick={() => retrieval && onCitation(part.id, retrieval)}
                  disabled={!retrieval || invalid.has(part.id)}
                >
                  {part.id}
                </button>
              ),
            )}
            {phase === 'generating' && <span className="caret" aria-hidden="true" />}
          </p>
          {result && result.citations.invalid.length > 0 && (
            <p className="hint error">
              A resposta cita ids que não estão nas evidências ({result.citations.invalid.join(', ')}). Trate essas
              partes com cautela.
            </p>
          )}
          {result && !result.noEvidence && (
            <p className="hint">
              Gerado por {result.model} em {(result.durationMs / 1000).toFixed(0)}s · {result.stats.promptTokens} tokens de
              evidência · {result.stats.generatedTokens} tokens a {result.stats.tokensPerSecond.toFixed(1)}/s. Respostas de
              modelos pequenos podem conter imprecisões: confira as evidências.
            </p>
          )}
        </div>
      )}

      {retrieval && !retrieval.noEvidence && (
        <div className="panel-section">
          <h3>
            Evidências ({retrieval.nodes.length + retrieval.documents.length} nós, {retrieval.edges.length} relações)
          </h3>
          <button
            type="button"
            className="btn"
            onClick={() => onShowEvidence(retrieval, result?.citations.valid ?? [])}
          >
            Ver evidências no grafo
          </button>
          {retrieval.entities.length > 0 && (
            <>
              <p className="ask-sub">Entidades citadas na pergunta</p>
              <ul className="ask-list">
                {retrieval.entities.map((n) => (
                  <li key={n.id}>
                    <button type="button" onClick={() => onCitation(n.id, retrieval)}>
                      <span className="swatch" style={{ background: NODE_STYLE[n.type].color }} />
                      {nodeTitle(n)} <code>{n.id}</code>
                    </button>
                  </li>
                ))}
              </ul>
            </>
          )}
          {retrieval.documents.length > 0 && (
            <>
              <p className="ask-sub">Documentos usados</p>
              <ul className="ask-list">
                {retrieval.documents.map((d) => (
                  <li key={d.node.id}>
                    <button type="button" onClick={() => onCitation(d.node.id, retrieval)}>
                      <span className="swatch" style={{ background: NODE_STYLE.Document.color }} />
                      {nodeTitle(d.node)}
                      <span className="count">{d.score < 0 ? 'via grafo' : `similaridade ${d.score.toFixed(2)}`}</span>
                    </button>
                  </li>
                ))}
              </ul>
            </>
          )}
        </div>
      )}
    </div>
  );
}
