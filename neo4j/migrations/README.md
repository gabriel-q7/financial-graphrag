# Neo4j migrations

Files named `NNN_description.cypher` are applied in lexical order by the backend
(`go run ./cmd/seed --migrate-only` or automatically at API start-up).

- Statements are separated by `;`. Lines starting with `//` are comments.
- Every statement must be idempotent (`IF NOT EXISTS`), because the runner may
  re-apply a file. Applied versions are also recorded as `(:Migration {version})`
  nodes so already applied files are skipped.
- Never edit an applied migration; add a new file instead.

## Vector index

`002_document_embeddings.cypher` creates `document_embedding` on `Document.embedding`
with **768 dimensions** (cosine), matching `OLLAMA_EMBED_MODEL=embeddinggemma:300m`.
The indexer (`go run ./cmd/seed`) checks the index dimension against
`OLLAMA_EMBED_DIMENSIONS` and the real model output before writing vectors.

To switch embedding models, add a new migration that drops and recreates the index
with the new dimension, update `OLLAMA_EMBED_MODEL`/`OLLAMA_EMBED_DIMENSIONS`, and run
`make index` (vectors from another model are re-generated automatically).
