# Neo4j migrations

Files named `NNN_description.cypher` are applied in lexical order by the backend
(`go run ./cmd/seed --migrate-only` or automatically at API start-up).

- Statements are separated by `;`. Lines starting with `//` are comments.
- Every statement must be idempotent (`IF NOT EXISTS`), because the runner may
  re-apply a file. Applied versions are also recorded as `(:Migration {version})`
  nodes so already applied files are skipped.
- Never edit an applied migration; add a new file instead.

## Vector index (next stage — not applied)

The vector index for GraphRAG depends on the dimension of the embedding model
configured in `OLLAMA_EMBED_MODEL`. It is deliberately **not** created here.
When the embedding pipeline is implemented, add a migration such as
`002_document_embeddings.cypher`, replacing `<DIMENSIONS>` with the real output
size of the selected model (e.g. `nomic-embed-text` → 768), and verify it with
`OLLAMA_EMBED_DIMENSIONS`:

```cypher
CREATE VECTOR INDEX document_embedding IF NOT EXISTS
FOR (d:Document) ON (d.embedding)
OPTIONS { indexConfig: {
  `vector.dimensions`: <DIMENSIONS>,
  `vector.similarity_function`: 'cosine'
}};
```
