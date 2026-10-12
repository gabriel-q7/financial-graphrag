import { describe, expect, it } from 'vitest';
import { createSSEParser, type SSEMessage } from './sse';

describe('createSSEParser', () => {
  it('parses events split across chunks and ignores heartbeats', () => {
    const got: SSEMessage[] = [];
    const feed = createSSEParser((m) => got.push(m));
    feed('event: retrieval\ndata: {"a":');
    feed('1}\n\n: keep-alive\n\nevent: tok');
    feed('en\ndata: {"text":"Olá"}\n\n');
    expect(got).toEqual([
      { event: 'retrieval', data: '{"a":1}' },
      { event: 'token', data: '{"text":"Olá"}' },
    ]);
  });

  it('handles CRLF and multi-line data', () => {
    const got: SSEMessage[] = [];
    const feed = createSSEParser((m) => got.push(m));
    feed('data: a\r\ndata: b\r\n\r\n');
    expect(got).toEqual([{ event: 'message', data: 'a\nb' }]);
  });
});
