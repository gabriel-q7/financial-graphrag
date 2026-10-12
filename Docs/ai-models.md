# IA local (Ollama + langchaingo)

## Configuração

| Variável | Valor atual | Uso |
| --- | --- | --- |
| `OLLAMA_URL` | `http://localhost:11434` | API do Ollama (contêiner `fg-ollama`) |
| `OLLAMA_CHAT_MODEL` | `qwen3:4b-instruct` | geração das respostas |
| `OLLAMA_EMBED_MODEL` | `embeddinggemma:300m` | embeddings de documentos e perguntas |
| `OLLAMA_EMBED_DIMENSIONS` | `768` | precisa bater com o modelo e com o índice `document_embedding` (migração 002) |

`make dev` baixa os modelos que faltarem. Manualmente:

```bash
docker compose exec ollama ollama pull qwen3:4b-instruct
docker compose exec ollama ollama pull embeddinggemma:300m
```

`/api/health` mostra se cada modelo configurado está disponível.

## Camada de modelos: langchaingo

Chat e embeddings passam por [langchaingo](https://github.com/tmc/langchaingo) **v0.1.14**
(`backend/internal/llm`). O resto do backend depende só de duas interfaces pequenas em
`internal/rag` (`Generator`, `Embedder`), então trocar de provedor (OpenAI, Anthropic, Bedrock,
etc., todos disponíveis no langchaingo) é uma mudança restrita ao pacote `llm`.

| Recurso do langchaingo | Uso no projeto |
| --- | --- |
| `llms/ollama` + `llms.WithStreamingFunc` | resposta em streaming (`/api/chat`) |
| `llms.WithMaxTokens` / `WithTemperature` | viram `num_predict` / `temperature` no Ollama |
| `ollama.WithRunnerNumCtx(4096)` | janela de contexto do modelo de chat |
| `ollama.WithKeepAlive("10m")` | mantém o modelo carregado entre perguntas |
| `embeddings.NewEmbedder` | embeddings em lote (`/api/embed`) |

Avaliado e **não usado**:

| Recurso | Motivo |
| --- | --- |
| Vector stores | não há implementação para Neo4j; usamos o índice vetorial nativo via Cypher (`rag.Neo4jStore`) |
| Chains (`RetrievalQA`, `StuffDocuments`…) | pressupõem recuperação só vetorial; não fazem travessia de grafo, validação de citações nem eventos SSE |
| v0.1.15 | publicada há menos de duas semanas (regra de versões do projeto) |

O langchaingo só informa contagem de tokens; tempos (até o primeiro token, geração,
tokens/s) são medidos pelo adaptador. Metadados do servidor (versão, modelos instalados) usam
um cliente HTTP mínimo próprio (`internal/ollama`), pois o langchaingo não os expõe.

## Hardware de referência

Notebook: AMD Ryzen 5 série 5000 (APU "Lucienne", Zen 2, 6 núcleos / 12 threads, AVX2, sem
AVX-512), 22 GB de RAM, GPU integrada Radeon Vega. O Ollama **não usa essa iGPU** (sem suporte
ROCm): toda inferência roda em CPU, o que limita a modelos de ~4B parâmetros em 4 bits.

## Modelo de chat: `qwen3:4b-instruct`

Qwen3-4B-Instruct-2507, Q4_K_M, 2,5 GB em disco, ~3,2 GB de RAM carregado.

| Modelo (tag Ollama) | Resultado |
| --- | --- |
| `qwen3:4b-instruct` | **Escolhido.** Respostas fundamentadas corretas em pt-BR, citando ids |
| `qwen3:4b` | Rejeitado: a tag aponta para Qwen3-4B-**Thinking**-2507; raciocina sempre, ignora `think: false` e `/no_think`; passou de 8 min numa resposta sem limite de tokens |
| `qwen3.5:4b` | Não medido (inclui projetor de visão de 676 MB, inútil aqui) |
| `llama3.2:3b` | Não medido; pior em português que o Qwen3 |

### Desempenho real de `POST /api/ask` (CPU de referência)

| Pergunta | Tokens de evidência | 1º token | Total |
| --- | --- | --- | --- |
| "Por que a conta da Ana Ribeiro merece revisão?" | 1.446 | 32 s | 67 s |
| "Quais pessoas compartilham o mesmo dispositivo?" | 1.615 | 40 s | 54 s |
| "Para onde foi o dinheiro que a conta acc_007 recebeu?" | 1.972 | 44 s | 75 s |
| "Qual é a capital da França?" (sem evidência) | — | imediato | < 1 s |

- Leitura do prompt: **~40–50 tokens/s** sem cache. É o maior custo, por isso o contexto é
  limitado a 30 nós e documentos ligados pelo grafo entram só como trecho.
- Geração: **~5 tokens/s** (o prompt de 1,5k tokens ocupa o cache e desacelera um pouco em
  relação aos 7 tok/s medidos com prompts curtos).
- Pergunta repetida com as mesmas evidências: o Ollama reaproveita o prompt em cache e o
  primeiro token sai em < 1 s.
- Primeira pergunta após o modelo descarregar: +5 a 12 s de carga (`keep_alive` 10 min).

Antes de limitar o contexto, a mesma pergunta sobre a Ana usava 2.574 tokens de evidência
(65 s só de leitura, 96 s no total).

### Qualidade observada

Com o prompt atual (temperatura 0, regras de citação e de linguagem neutra):

- **Dispositivo compartilhado:** "Diego Farias [per_004], Fábio Lins [per_006] e Elisa Moura
  [per_005] compartilham o mesmo dispositivo móvel, conforme indicado no alerta automatizado
  [doc_003]." — correto, todos os ids válidos.
- **Dinheiro recebido por `acc_007`:** listou as três entradas (`txn_010`–`txn_012`) e a saída
  de 14.000 BRL para `acc_012` (`txn_013`) — correto.
- **Conta da Ana:** cita corretamente `txn_005`, `acc_001`, `acc_005`, `doc_001`, mas afirma que
  a Ana controla a Delta Importadora (no grafo, quem controla é Diego). Erro de raciocínio do
  modelo de 4B que a validação de ids não detecta, pois o id citado existe nas evidências.

Ajustes que melhoraram a qualidade:

- Regra explícita "escreva o id logo depois de cada entidade" → passou de 2 para 7 ids citados.
- Lista explícita de palavras proibidas ("suspeito", "fraude", "risco"…) → o modelo parou de
  qualificar os padrões.
- Contas nas transferências anotadas com o dono (`[acc_010] (de Delta Importadora [cmp_004])`).
- Temperatura 0 → respostas determinísticas para as mesmas evidências.

### Extração de entidades (etapa futura)

Teste isolado com JSON schema (`format`): sempre JSON válido, encontrou as 3 pessoas e o
dispositivo, mas **inventou relacionamentos** (`CONTROLS` entre pessoas), inverteu a direção
de `USES` e, numa execução, classificou "intervalo de 48 horas" como `Person`. A extração
precisa de validação por schema, resolução de entidades e revisão antes de gravar no grafo.

## Modelo de embeddings: `embeddinggemma:300m`

Comparação com 8 perguntas (5 relevantes ao dataset, 2 sem relação, 1 temática) contra os 6
documentos. Valores = score do Neo4j, `(1 + cosseno) / 2`, usando o prompt de tarefa de cada
modelo.

| Pergunta | `nomic-embed-text` | `embeddinggemma:300m` | `qwen3-embedding:0.6b` |
| --- | --- | --- | --- |
| Ana Ribeiro / revisão → `doc_001` | 0,829 | 0,773 | 0,813 |
| dispositivo compartilhado → `doc_003` | 0,859 | 0,764 | 0,831 |
| depósitos fracionados → `doc_005` | 0,841 | 0,708 | 0,811 |
| parceria Atlas/Cedro → `doc_004` | 0,879 | 0,845 | 0,836 |
| cobrança contestada → `doc_006` | 0,878 | 0,749 | 0,798 |
| "Qual é a capital da França?" (melhor doc) | **0,841** | **0,563** | 0,581 |
| "Receita de bolo de chocolate" (melhor doc) | **0,797** | **0,569** | 0,647 |
| Dimensões / tempo por pergunta | 768 / — | 768 / 175 ms | 1024 / 560 ms |

- `nomic-embed-text` (orientado a inglês) não separa relevante de irrelevante em português:
  "capital da França" pontua mais que perguntas legítimas.
- `embeddinggemma:300m` separa bem (relevantes ≥ 0,71; irrelevantes ≤ 0,57), tem a mesma
  dimensão do índice e é o mais rápido. **Escolhido.**
- `qwen3-embedding:0.6b` também separa, mas tem 1024 dimensões e é 3× mais lento.

Parâmetros de recuperação derivados (em `backend/internal/rag/service.go`):

| Constante | Valor | Motivo |
| --- | --- | --- |
| `MinDocumentScore` | 0,66 | entre o pior relevante (0,71) e o melhor irrelevante (0,57) |
| `DocumentScoreMargin` | 0,05 | só documentos próximos do melhor; um documento fraco puxava partes não relacionadas do grafo |
| `DocumentsK` | 4 | candidatos da busca vetorial |
| `MaxContextNodes` | 30 | limite de nós no contexto (custo de leitura em CPU) |
| `NumPredict` | 450 | limite de tokens gerados |

Prompts de tarefa aplicados automaticamente (`embedPrefixes`):

| Modelo | Documento | Pergunta |
| --- | --- | --- |
| `embeddinggemma` | `title: none \| text: ` | `task: search result \| query: ` |
| `nomic-embed-text` | `search_document: ` | `search_query: ` |
| `qwen3-embedding` | — | `Instruct: … Query: ` |

Trocar de modelo de embeddings: ajuste `OLLAMA_EMBED_MODEL`/`OLLAMA_EMBED_DIMENSIONS`; se a
dimensão mudar, crie uma migração que recrie o índice; rode `make index` (o indexador detecta
o modelo diferente e recalcula todos os vetores). Recalibre `MinDocumentScore`.
