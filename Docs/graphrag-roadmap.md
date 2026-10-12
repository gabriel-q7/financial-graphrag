# Roadmap GraphRAG

Objetivo: responder perguntas em linguagem natural sobre o grafo financeiro com respostas
**fundamentadas em evidências recuperadas** (cada afirmação cita ids de nós/documentos),
usando apenas modelos locais.

## Estado

| Etapa | Estado | Onde |
| --- | --- | --- |
| 1. Ingestão de documentos | Só via seed | `data/seed/`, `internal/seed` |
| 2. Extração de entidades e relacionamentos | **Pendente** (interface `rag.Extractor`) | `internal/rag/rag.go` |
| 3. Embeddings | **Feito** — documentos, `embeddinggemma:300m`, idempotente | `internal/rag/indexer.go`, `internal/llm` |
| 4. Índice vetorial | **Feito** — `document_embedding`, 768 dim, cosseno | `neo4j/migrations/002_document_embeddings.cypher` |
| 5. Recuperação vetorial | **Feito** — documentos (limiar + margem); entidades por nome/id | `internal/rag/service.go`, `entities.go`, `store.go` |
| 6. Expansão no grafo | **Feito** — vizinhança + trilhas de dinheiro, ranqueada, ≤ 30 nós | `internal/rag/service.go` |
| 7. Construção de contexto | **Feito** — linhas com ids, contas anotadas com dono | `internal/rag/context.go` |
| 8. Geração fundamentada | **Feito** — streaming, temperatura 0, validação de citações | `internal/rag/service.go`, `prompt.go`, `internal/llm` |
| 9. API e interface | **Feito** — `POST /api/ask` (SSE), aba "Perguntar à IA" | `internal/http/handlers/ask.go`, `frontend/src/features/ask` |

Arquitetura e fluxo detalhados em [Arquitetura](architecture.md#fluxo-de-uma-pergunta-post-apiask);
modelos e medições em [IA local](ai-models.md).

## Próximos passos

### 1. Ingestão de documentos novos

- Endpoint/CLI para inserir `Document` (texto, origem, data) com id `doc_…`, reaproveitando a
  validação do seed; limite de tamanho.
- Indexar o embedding na ingestão (o indexador já é incremental).
- Documentos longos: dividir em trechos (`(:Document)-[:HAS_CHUNK]->(:Chunk)`) com
  embedding por trecho.

### 2. Extração de entidades e relacionamentos

O teste com o modelo 4B mostrou relações inventadas e direções invertidas
([IA local](ai-models.md#extração-de-entidades-etapa-futura)). Portanto:

- `format` com JSON schema e `enum` dos tipos permitidos;
- descartar o que violar `graph.RelSpecs`;
- resolver nomes para ids existentes (reaproveitar `MatchEntities`); ambiguidade → não ligar;
- gravar como proposta com proveniência (`{source:'extraction', model, at}`) e revisão humana;
- nunca criar `Transaction`/`BankAccount` a partir de texto livre;
- medir precisão/recall contra os `MENTIONS` do seed (ground truth disponível).

### 3. Recuperação mais rica

- Embeddings de entidades (texto descritivo: "Pessoa Ana Ribeiro, controla Atlas e Borealis")
  para perguntas que não citam nomes.
- Índice de texto completo (`FULLTEXT`) para nomes com variações/erros de digitação.
- Escolher tipos de relação conforme a pergunta (dinheiro, dispositivos, documentos).

### 4. Qualidade e avaliação

- Conjunto fixo de perguntas com respostas esperadas e ids obrigatórios; rodar em CI local.
- Métricas: % de ids válidos, cobertura dos ids esperados, frases sem citação, tempo.
- Verificação pós-geração de afirmações simples (ex.: "X controla Y" existe como `CONTROLS`?)
  para pegar erros como o da Ana/Delta.
- Avaliar modelos maiores quando houver GPU (o adaptador langchaingo permite trocar o provedor).

### 5. Desempenho em CPU

- Contexto ainda menor por pergunta (descrever só o necessário ao tipo de pergunta).
- Manter o prompt de sistema estável para aproveitar o cache de prefixo do Ollama.
- Respostas mais curtas por padrão, com opção "detalhar".

## Riscos

| Risco | Mitigação atual | Próximo passo |
| --- | --- | --- |
| Latência de 40–100 s em CPU | streaming, contexto ≤ 30 nós, sem LLM quando não há evidência | contexto por tipo de pergunta; GPU |
| Erros de raciocínio do modelo 4B | citações validadas; evidências visíveis e clicáveis | verificação de afirmações; avaliação |
| Alucinação de relações na extração | extração não implementada | schema + proveniência + revisão |
| Embeddings ruins em pt-BR | benchmark escolheu `embeddinggemma:300m` | repetir benchmark ao trocar modelo |
| Mudança de modelo de embeddings | `embedding_model`/hash gravados; reindexação automática | migração de índice se a dimensão mudar |
| Injeção de instruções via documentos | delimitadores, regra no prompt, ids validados | revisão de documentos ingeridos |
