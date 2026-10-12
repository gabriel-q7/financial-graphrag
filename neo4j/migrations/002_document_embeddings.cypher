// 002 - Vector index for Document embeddings (GraphRAG retrieval).
// Dimension 768 = output size of OLLAMA_EMBED_MODEL=embeddinggemma:300m.
// The indexer refuses to write vectors when OLLAMA_EMBED_DIMENSIONS or the
// model output differ from this value. Changing the embedding model requires a
// new migration that drops and recreates this index, then re-indexing.
CREATE VECTOR INDEX document_embedding IF NOT EXISTS
FOR (d:Document) ON (d.embedding)
OPTIONS { indexConfig: {
  `vector.dimensions`: 768,
  `vector.similarity_function`: 'cosine'
}};
