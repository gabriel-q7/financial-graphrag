export type AnswerPart = { kind: 'text'; text: string } | { kind: 'id'; id: string };

const ID_PATTERN = /\[?\b((?:per|cmp|acc|txn|dev|doc)_\d{3,9})\b\]?/gi;

/** Splits an answer into plain text and entity ids (with or without brackets). */
export function splitAnswer(text: string): AnswerPart[] {
  const parts: AnswerPart[] = [];
  let last = 0;
  for (const m of text.matchAll(ID_PATTERN)) {
    const start = m.index;
    if (start > last) parts.push({ kind: 'text', text: text.slice(last, start) });
    parts.push({ kind: 'id', id: m[1]!.toLowerCase() });
    last = start + m[0].length;
  }
  if (last < text.length) parts.push({ kind: 'text', text: text.slice(last) });
  return parts;
}
