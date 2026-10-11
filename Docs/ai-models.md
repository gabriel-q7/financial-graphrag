# IA local (Ollama)

## Configuração

| Variável | Valor atual | Uso |
| --- | --- | --- |
| `OLLAMA_URL` | `http://localhost:11434` | API do Ollama (contêiner `fg-ollama`) |
| `OLLAMA_CHAT_MODEL` | `qwen3:4b-instruct` | extração e geração de respostas (etapa 2) |
| `OLLAMA_EMBED_MODEL` | `nomic-embed-text` | embeddings (etapa 2) |
| `OLLAMA_EMBED_DIMENSIONS` | `768` | precisa bater com o modelo de embeddings; usado no índice vetorial |

Modelos são baixados manualmente e ficam no volume `ollama_models`:

```bash
docker compose exec ollama ollama pull qwen3:4b-instruct
docker compose exec ollama ollama pull nomic-embed-text
docker compose exec ollama ollama list
```

`/api/health` mostra se cada modelo configurado está disponível.

## Hardware de referência

Notebook de desenvolvimento: AMD Ryzen 5 série 5000 (APU "Lucienne", Zen 2, 6 núcleos /
12 threads, AVX2, sem AVX-512), 22 GB de RAM, GPU integrada Radeon Vega. O Ollama **não usa
essa iGPU** (sem suporte ROCm), então toda inferência roda em CPU. Isso limita a escolha a
modelos de ~4B parâmetros quantizados em 4 bits.

## Modelo de chat: `qwen3:4b-instruct`

Qwen3-4B-Instruct-2507, Q4_K_M, 2,5 GB em disco, ~3,2 GB de RAM carregado (contexto 4096).

### Candidatos avaliados

| Modelo (tag Ollama) | Tamanho | Resultado |
| --- | --- | --- |
| `qwen3:4b-instruct` | 2,5 GB | **Escolhido.** Resposta fundamentada correta em pt-BR, citando ids |
| `qwen3:4b` | 2,5 GB | Rejeitado: a tag aponta para Qwen3-4B-**Thinking**-2507 (mesmo arquivo de `qwen3:4b-thinking`); raciocina sempre, ignora `think: false` e `/no_think`; passou de 8 min numa resposta sem limite de tokens |
| `qwen3.5:4b` | 3,3 GB | Baixado, não medido (inclui projetor de visão de 676 MB, inútil aqui) |
| `llama3.2:3b` | 2,0 GB | Baixado, não medido; pior em português que o Qwen3 |

### Medições (Ollama 0.34.4, CPU, `num_ctx` 4096, temperatura 0)

| Tarefa | Tokens de entrada | Processamento do prompt | Geração | Tempo total |
| --- | --- | --- | --- | --- |
| Extração com JSON schema (modelo frio) | 116 | ~50 tok/s | 7,1 tok/s | 58,8 s (11,6 s de carga) |
| Extração com JSON schema (modelo quente) | 116 | ~900 tok/s* | 7,4 tok/s | 46,4 s |
| Resposta fundamentada em pt-BR | 457 | ~42 tok/s | 7,2 tok/s | 45,2 s |

\* inclui reaproveitamento de cache do prompt de sistema.

Conclusões práticas:

- **~7 tokens/s de geração**: uma resposta de 250 tokens leva ~35–45 s. A interface da etapa 2
  deve usar streaming e mostrar progresso.
- **Prompt sem cache: ~40–50 tok/s**: um contexto de 2.000 tokens custa ~45 s só para ser lido.
  O construtor de contexto precisa ser enxuto (fatos compactos com ids, não JSON verboso).
- Sempre definir `num_predict` (ex.: 300–400) para limitar o tempo de geração.
- O modelo fica carregado por `OLLAMA_KEEP_ALIVE=5m`; a primeira chamada paga 6–12 s de carga.

### Qualidade observada

**Resposta fundamentada** (contexto com a cadeia `acc_001 → … → acc_001`): descreveu o ciclo
corretamente, citou `[acc_001]`, `[acc_005]`, `[acc_008]`, `[acc_010]`, `[per_004]`, `[cmp_002]`,
`[cmp_004]`, `[doc_001]` e não afirmou fraude — seguiu a instrução de descrever sinais.
Imprecisão menor: chamou a conta de "controlada" pela empresa (o grafo diz `OWNS`).

**Extração de entidades** (texto de `doc_003`, com JSON schema via `format`): sempre JSON
válido e encontrou as 3 pessoas e o dispositivo, mas **inventou relacionamentos**
(`CONTROLS` entre pessoas), inverteu a direção de `USES` e, numa execução, classificou
"intervalo de 48 horas" como `Person`.

Consequências para a etapa 2:

- Validar toda extração contra `graph.RelSpecs` (tipo e direção) e descartar o resto.
- Resolver entidades extraídas contra nós existentes (nome → id) antes de criar algo novo.
- Gravar relacionamentos extraídos como **propostas** com origem (`doc_id`), não como fatos.
- Considerar extração em duas etapas (entidades, depois relações entre entidades conhecidas).

## Modelo de embeddings

Configurado: `nomic-embed-text` (768 dimensões, 270 MB), ainda **não baixado**. Ele é
orientado a inglês e os documentos são em português. Alternativas multilíngues a avaliar com
o mesmo método antes de criar o índice vetorial:

| Modelo | Tamanho | Observação |
| --- | --- | --- |
| `nomic-embed-text` | 0,27 GB | 768 dim; foco em inglês |
| `qwen3-embedding:0.6b` | 0,64 GB | multilíngue |
| `embeddinggemma:300m` | 0,62 GB | multilíngue |

A dimensão do modelo escolhido define `OLLAMA_EMBED_DIMENSIONS` e a migração do índice
vetorial; trocar de modelo depois exige recriar o índice e recalcular todos os embeddings.

## Cliente Go (`internal/ollama`)

| Método | Endpoint Ollama | Uso |
| --- | --- | --- |
| `Version` | `GET /api/version` | saúde |
| `Models` | `GET /api/tags` | saúde (modelos baixados) |
| `Embed` | `POST /api/embed` | embeddings em lote (implementa `rag.Embedder`) |
| `HasModel` | — | compara nomes; nome sem tag equivale a `:latest` |

Ainda não há método de chat/geração. Ao implementá-lo: `stream: true`, `think: false`,
`options.num_predict`, `options.num_ctx` e `format` (JSON schema) para extração.

## Reproduzir o benchmark

O script do benchmark não está no repositório; as tarefas foram as duas descritas acima
(extração do texto de `doc_003` com JSON schema e resposta sobre a cadeia de `acc_001`),
temperatura 0 e `num_ctx` 4096. Para medir rapidamente outro modelo:

```bash
curl -s http://localhost:11434/api/chat -d '{
  "model": "qwen3:4b-instruct", "stream": false, "think": false,
  "options": {"temperature": 0, "num_ctx": 4096, "num_predict": 400},
  "messages": [{"role": "user", "content": "Resuma em 2 frases: ..."}]
}' | python3 -c "import sys,json;r=json.load(sys.stdin);print(r['message']['content']);print(r['eval_count']/(r['eval_duration']/1e9),'tok/s')"
```
