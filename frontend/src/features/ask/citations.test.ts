import { describe, expect, it } from 'vitest';
import { splitAnswer } from './citations';

describe('splitAnswer', () => {
  it('extracts bracketed and bare ids', () => {
    expect(splitAnswer('Ana [per_001] enviou txn_005.')).toEqual([
      { kind: 'text', text: 'Ana ' },
      { kind: 'id', id: 'per_001' },
      { kind: 'text', text: ' enviou ' },
      { kind: 'id', id: 'txn_005' },
      { kind: 'text', text: '.' },
    ]);
  });

  it('keeps text without ids intact and ignores partial ids', () => {
    expect(splitAnswer('sem ids, per_1 não conta')).toEqual([{ kind: 'text', text: 'sem ids, per_1 não conta' }]);
  });
});
