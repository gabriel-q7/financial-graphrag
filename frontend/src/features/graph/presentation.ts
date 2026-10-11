import type { GraphNode, NodeType, PropertyValue, RelType } from '../../types/graph';

export const NODE_STYLE: Record<NodeType, { label: string; color: string; glyph: string }> = {
  Person: { label: 'Pessoa', color: '#60a5fa', glyph: 'P' },
  Company: { label: 'Empresa', color: '#a78bfa', glyph: 'E' },
  BankAccount: { label: 'Conta bancária', color: '#34d399', glyph: 'C' },
  Transaction: { label: 'Transação', color: '#fbbf24', glyph: 'T' },
  Device: { label: 'Dispositivo', color: '#f472b6', glyph: 'D' },
  Document: { label: 'Documento', color: '#94a3b8', glyph: 'Doc' },
};

export const REL_STYLE: Record<RelType, { label: string; color: string; dashed: boolean }> = {
  OWNS: { label: 'possui', color: '#34d399', dashed: false },
  CONTROLS: { label: 'controla', color: '#a78bfa', dashed: false },
  SENT: { label: 'enviou', color: '#fbbf24', dashed: false },
  TO_ACCOUNT: { label: 'para conta', color: '#fbbf24', dashed: false },
  USES: { label: 'usa', color: '#f472b6', dashed: true },
  MENTIONS: { label: 'menciona', color: '#94a3b8', dashed: true },
};

export const PROPERTY_LABEL: Record<string, string> = {
  name: 'Nome',
  sector: 'Setor',
  bank: 'Banco',
  status: 'Status',
  amount: 'Valor',
  currency: 'Moeda',
  timestamp: 'Data/hora',
  device_type: 'Tipo de dispositivo',
  text: 'Texto',
  source: 'Origem',
  created_at: 'Criado em',
};

function str(v: PropertyValue | undefined): string {
  if (v === undefined) return '';
  return Array.isArray(v) ? v.join(', ') : String(v);
}

export function formatMoney(amount: PropertyValue | undefined, currency: PropertyValue | undefined): string {
  if (typeof amount !== 'number') return str(amount);
  const code = typeof currency === 'string' && /^[A-Z]{3}$/.test(currency) ? currency : 'BRL';
  return new Intl.NumberFormat('pt-BR', { style: 'currency', currency: code }).format(amount);
}

export function formatDate(v: PropertyValue | undefined): string {
  if (typeof v !== 'string') return str(v);
  const d = new Date(v);
  return Number.isNaN(d.getTime()) ? v : d.toLocaleString('pt-BR', { dateStyle: 'short', timeStyle: 'short' });
}

/** Short, human readable title for a node. */
export function nodeTitle(n: GraphNode): string {
  const p = n.properties;
  switch (n.type) {
    case 'Person':
    case 'Company':
      return str(p.name) || n.id;
    case 'BankAccount':
      return `${str(p.bank) || 'Conta'} · ${n.id}`;
    case 'Transaction':
      return formatMoney(p.amount, p.currency);
    case 'Device':
      return `${str(p.device_type) || 'dispositivo'} · ${n.id}`;
    case 'Document':
      return `${str(p.source) || 'documento'} · ${n.id}`;
  }
}

/** Formats a property for the details panel. */
export function formatProperty(n: GraphNode, key: string): string {
  const v = n.properties[key];
  if (key === 'amount') return formatMoney(v, n.properties.currency);
  if (key === 'timestamp' || key === 'created_at') return formatDate(v);
  return str(v);
}
