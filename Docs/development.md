# Desenvolvimento

Guia para rodar, testar e alterar o projeto. A instalação passo a passo está no
[README](../README.md); aqui ficam o fluxo diário, as convenções e a solução de problemas.

## Ambiente

| Ferramenta | Versão | Observação |
| --- | --- | --- |
| Go | 1.27 (`go.mod`: `go 1.27.0`) | |
| Node.js | ≥ 22.12 | exigido pelo Vite 8 |
| Docker / Compose v2 | 29.x / 5.x usados no desenvolvimento | |
| Neo4j (imagem) | `neo4j:2026.08.1-community` | fixada no compose |
| Ollama (imagem) | `ollama/ollama:0.34.4` | fixada no compose |

Dependências fixadas com versão exata: `github.com/neo4j/neo4j-go-driver/v6 v6.3.0`;
no frontend, `package.json` sem `^`/`~` e `package-lock.json` versionado (use `npm ci`).

## Fluxo diário

```bash
make dev        # sobe tudo; Ctrl+C encerra API e frontend
make stop       # ao final do dia: para também os contêineres
```

`make dev` é idempotente: pode ser executado sempre que for trabalhar. Na primeira vez ele
cria o `.env`, baixa os modelos e instala dependências; nas seguintes só verifica e inicia.

## Scripts

Ficam em `scripts/` e são chamados pelo `Makefile`. Todos usam `set -euo pipefail` e as
funções de `scripts/lib.sh` (mensagens, carga do `.env`, `docker compose`, portas).

| Script | Responsabilidade |
| --- | --- |
| `lib.sh` | funções compartilhadas (não executar diretamente) |
| `env-init.sh` | cria `.env` com segredos aleatórios (permissão 600); nunca sobrescreve; avisa sobre variáveis novas no `.env.example` |
| `check-prereqs.sh` | docker/compose/daemon, Go ≥ 1.27, Node ≥ 22.12, curl, openssl, ss |
| `services-up.sh` | `docker compose up -d --wait` (timeout 180 s) |
| `models.sh` | `ollama pull` apenas dos modelos ausentes (`SKIP_MODELS=1` pula) |
| `frontend-deps.sh` | `npm ci` se `node_modules` não existe ou é mais antigo que o `package-lock.json` |
| `db-init.sh` | compila `bin/seed` e aplica migrações, seed e embeddings (`--migrate-only`, `--embeddings-only`, `--skip-embeddings`) |
| `dev.sh` | orquestra tudo e executa API + Vite em sessões próprias, com logs prefixados |
| `stop-apps.sh` | encerra o processo que estiver nas portas da API e do frontend |
| `reset-db.sh` | remove o volume `neo4j_data` e refaz serviços + seed (`FORCE=1` sem confirmação) |

Comportamento do `dev.sh` ao encerrar:

- `Ctrl+C`: envia SIGTERM aos grupos de processo da API e do Vite, espera terminarem e sai com 0.
- Uma das aplicações cai: a outra é encerrada e o comando sai com erro (falha não fica escondida).
- Terminal fechado (SIGHUP) ou `kill`: mesma limpeza.
- Contêineres não são parados; use `make stop`.

## Comandos

| Objetivo | Make | Manual (diretório) |
| --- | --- | --- |
| Tudo | `make dev` | ver README, seção "Execução manual" |
| Migrações + seed | `make seed` | `go run ./cmd/seed` (`backend`) |
| Só migrações | `make migrate` | `go run ./cmd/seed --migrate-only` (`backend`) |
| Só embeddings | `make index` | `go run ./cmd/seed --embeddings-only` (`backend`) |
| Fazer uma pergunta | interface, ou `curl -N -X POST localhost:8080/api/ask -H 'Content-Type: application/json' -d '{"question":"…"}'` | — |
| Apagar e recriar dados | `make reset-db` | `docker compose down -v` (apaga também modelos) |
| Testes unitários | `make test` | `go test ./...` (`backend`), `npm test` (`frontend`) |
| Testes de integração | `make test-integration` | `go test -tags integration -count=1 ./internal/integration/` com `.env` carregado (`backend`) |
| Lint | `make lint` | `gofmt -l . && go vet ./...` (`backend`), `npm run typecheck` (`frontend`) |
| Build | `make build` | `go build -o bin/ ./cmd/...`, `npm run build` |
| Estado | `make status` | `docker compose ps`, `curl localhost:8080/api/health` |
| Shell Cypher | — | `docker compose exec neo4j cypher-shell -u neo4j -p "$NEO4J_PASSWORD"` |
| Modelos | `make models` | `docker compose exec ollama ollama list` |

## Testes

| Camada | Arquivo | O que cobre |
| --- | --- | --- |
| Config | `internal/config/config_test.go` | padrões, senha obrigatória, regras do seed admin, valores inválidos |
| Grafo | `internal/graph/params_test.go` | validação de id, `depth`/`limit`/`relationshipTypes`, tentativas de injeção |
| Grafo | `internal/graph/dto_test.go` | conversão de registros Neo4j em DTOs, whitelist de propriedades, datas, valores não suportados |
| Migrações | `internal/database/migrate_test.go` | parsing de comandos, ordem, versões duplicadas, arquivos do repositório |
| Seed | `internal/seed/dataset_test.go` | dataset do repositório válido e completo; rejeição de dados inválidos |
| Saúde | `internal/health/health_test.go` | status `ok`/`degraded`/`unavailable`; erros internos não vazam |
| Ollama | `internal/ollama/client_test.go` | metadados (versão, modelos) contra servidor falso |
| LLM | `internal/llm/llm_test.go` | adaptadores langchaingo contra Ollama falso: streaming, opções, estatísticas, erros, embeddings |
| RAG | `internal/rag/rag_test.go` | reconhecimento de entidades, contexto, citações, serviço com fakes (limiar, trilha de dinheiro, sem evidência sem LLM) |
| HTTP | `internal/http/routes_test.go`, `ask_test.go` | códigos de status, erros sanitizados, CORS, seed admin; ordem dos eventos SSE, validação, `429`, erros antes/depois do stream |
| Integração | `internal/integration/neo4j_test.go`, `rag_test.go` | seed idempotente, vizinhança real, limites, busca; indexação idempotente e recuperação real (`doc_003`, trilha da Ana, sem evidência) |
| Frontend | `graphModel.test.ts`, `sse.test.ts`, `citations.test.ts` | merge/layout/filtros; parser SSE; ids na resposta |

Os testes de integração usam o banco `neo4j` de desenvolvimento (o Community tem um único
banco de usuário). Como só executam o seed idempotente e leituras, não alteram os dados.

## Convenções

- **Go:** `gofmt`; erros embrulhados com `%w` e contexto; handlers nunca devolvem `err.Error()`
  ao cliente; dependências por interface quando há teste com fake; sem pacotes `utils`.
- **Cypher:** sempre parâmetros (`$id`, `$types`); rótulos/tipos só de constantes do código.
- **Novos tipos de nó ou relacionamento:** atualize `graph.NodeSpecs`/`RelSpecs`, as consultas
  em `seed/seed.go` (`nodeQueries`), `frontend/src/types/graph.ts`, `presentation.ts`, o
  dataset e a [documentação do modelo](data-model.md). Os testes de seed falham se faltar consulta.
- **Frontend:** TypeScript strict com `noUncheckedIndexedAccess` e `exactOptionalPropertyTypes`;
  toda chamada HTTP passa por `src/lib/api.ts`; textos da interface em pt-BR.
- **Dependências:** versão exata e publicada há pelo menos duas semanas.
- **Branches:** trabalhe na branch atual; crie nova só a partir de `main`/`master`.

## Solução de problemas

| Sintoma | Causa provável | Solução |
| --- | --- | --- |
| `port 8080 is already in use` ao rodar `make dev` | API/Vite de outro terminal ainda rodando | `make stop-apps` |
| API não inicia: `NEO4J_PASSWORD is required` | execução manual sem carregar o `.env` | use `make dev` ou `set -a; source ../.env; set +a` |
| API não inicia: `ADMIN_TOKEN must have at least 16 characters` | seed admin habilitado sem token | gere um token ou `ADMIN_SEED_ENABLED=false` |
| `/api/health` 503, `neo4j is unreachable` | contêiner parado ou ainda subindo | `docker compose up -d --wait neo4j` |
| `Neo.ClientError.Security.Unauthorized` no log | senha do `.env` diferente da usada ao criar o volume | volte a senha ou `docker compose down -v` (apaga dados) |
| Frontend: "Não foi possível conectar à API" | API parada ou `VITE_API_BASE_URL` errado | suba a API; confira o `.env` da raiz |
| Erro de CORS no console | aberto em `127.0.0.1:5173` | use `http://localhost:5173` ou ajuste `CORS_ALLOWED_ORIGIN` |
| Grafo vazio / "Entidade não encontrada" | seed não executado | `go run ./cmd/seed` |
| Ollama `degraded`: models not pulled | modelos não baixados | `make models` |
| Pergunta: "O modelo de IA está indisponível" | Ollama parado ou sem `embeddinggemma:300m` | `make services && make models` |
| Pergunta: "Outra pergunta está sendo respondida" | uma geração em andamento (uma por vez) | aguarde ou cancele a anterior |
| Pergunta sempre "Não encontrei no grafo…" | documentos sem embeddings | `make index` |
| `vector index has N dimensions but OLLAMA_EMBED_DIMENSIONS=M` | modelo de embeddings trocado sem migração | ver [IA local](ai-models.md#modelo-de-embeddings-embeddinggemma300m) |
| Porta 5173 ocupada | outro Vite rodando | encerre o processo (`strictPort` impede trocar de porta) |
| Saída de `curl` truncada no terminal | proxy de terminal (ex.: RTK) resumindo JSON | `rtk proxy curl …` ou redirecione para arquivo |
| Neo4j lento ou reiniciando | pouca memória | ajuste `NEO4J_server_memory_*` no compose |
