import type { HealthReport } from '../types/graph';

interface Props {
  health: HealthReport | null;
  error: string | null;
}

const LABEL = { ok: 'operacional', degraded: 'degradado', unavailable: 'indisponível' } as const;

/** Compact status of the API and its dependencies. */
export function HealthBadge({ health, error }: Props) {
  if (error) {
    return (
      <span className="health health-unavailable" title={error}>
        API inacessível
      </span>
    );
  }
  if (!health) return <span className="health">verificando…</span>;

  const details = Object.entries(health.checks)
    .map(([name, c]) => `${name}: ${LABEL[c.status]}${c.error ? ` — ${c.error}` : ''}`)
    .join('\n');
  return (
    <span className={`health health-${health.status}`} title={details}>
      {Object.entries(health.checks).map(([name, c]) => (
        <span key={name} className={`health-dot dot-${c.status}`}>
          {name}
        </span>
      ))}
    </span>
  );
}
