import type { ReactNode } from 'react';

interface Props {
  kind: 'loading' | 'error' | 'empty';
  title: string;
  children?: ReactNode;
  action?: { label: string; onClick: () => void };
}

/** Centered message for loading, error and empty states. */
export function StateMessage({ kind, title, children, action }: Props) {
  return (
    <div className={`state state-${kind}`} role={kind === 'error' ? 'alert' : 'status'}>
      {kind === 'loading' && <span className="spinner" aria-hidden="true" />}
      <p className="state-title">{title}</p>
      {children && <div className="state-body">{children}</div>}
      {action && (
        <button type="button" className="btn" onClick={action.onClick}>
          {action.label}
        </button>
      )}
    </div>
  );
}
