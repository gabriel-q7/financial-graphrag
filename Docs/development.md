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
# 1. Serviços
docker compose up -d --wait

# 2. Variáveis no shell (backend lê do ambiente, não do arquivo)
set -a; source .env; set +a

# 3. API (terminal 1)
cd backend && go run ./cmd/api

# 4. Frontend (terminal 2)
cd frontend && npm run dev     # http://localhost:5173
```

Parar: `Ctrl+C` na API e no Vite; `docker compose stop` para os contêineres
(`docker compose down -v` apaga também os dados e os modelos baixados).

## Comandos

| Objetivo | Comando | Diretório |
| --- | --- | --- |
| Migrações + seed | `go run ./cmd/seed` | `backend` |
| Só migrações | `go run ./cmd/seed --migrate-only` | `backend` |
| Build da API | `go build -o bin/api ./cmd/api` | `backend` |
| Testes unitários Go | `go test ./...` | `backend` |
| Testes de integração Go | `go test -tags integration -count=1 ./internal/integration/` (com `.env` carregado e Neo4j no ar) | `backend` |
| Lint Go | `gofmt -l . && go vet ./... && go vet -tags integration ./...` | `backend` |
| Testes frontend | `npm test` | `frontend` |
| Typecheck frontend | `npm run typecheck` | `frontend` |
| Build frontend | `npm run build` | `frontend` |
| Shell Cypher | `docker compose exec neo4j cypher-shell -u neo4j -p "$NEO4J_PASSWORD"` | raiz |
| Modelos Ollama | `docker compose exec ollama ollama list` | raiz |

## Testes

| Camada | Arquivo | O que cobre |
| --- | --- | --- |
| Config | `internal/config/config_test.go` | padrões, senha obrigatória, regras do seed admin, valores inválidos |
| Grafo | `internal/graph/params_test.go` | validação de id, `depth`/`limit`/`relationshipTypes`, tentativas de injeção |
| Grafo | `internal/graph/dto_test.go` | conversão de registros Neo4j em DTOs, whitelist de propriedades, datas, valores não suportados |
| Migrações | `internal/database/migrate_test.go` | parsing de comandos, ordem, versões duplicadas, arquivos do repositório |
| Seed | `internal/seed/dataset_test.go` | dataset do repositório válido e completo; rejeição de dados inválidos |
| Saúde | `internal/health/health_test.go` | status `ok`/`degraded`/`unavailable`; erros internos não vazam |
| Ollama | `internal/ollama/client_test.go` | cliente contra servidor HTTP falso |
| HTTP | `internal/http/routes_test.go` | códigos de status, erros sanitizados, CORS, seed admin (token, loopback, desabilitado) |
| Integração | `internal/integration/neo4j_test.go` | seed idempotente, vizinhança real, filtro por tipo, limites, busca |
| Frontend | `src/features/graph/graphModel.test.ts` | merge sem duplicação, posições preservadas, layout, filtros |

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
| API não inicia: `NEO4J_PASSWORD is required` | `.env` não carregado no shell | `set -a; source ../.env; set +a` |
| API não inicia: `ADMIN_TOKEN must have at least 16 characters` | seed admin habilitado sem token | gere um token ou `ADMIN_SEED_ENABLED=false` |
| `/api/health` 503, `neo4j is unreachable` | contêiner parado ou ainda subindo | `docker compose up -d --wait neo4j` |
| `Neo.ClientError.Security.Unauthorized` no log | senha do `.env` diferente da usada ao criar o volume | volte a senha ou `docker compose down -v` (apaga dados) |
| Frontend: "Não foi possível conectar à API" | API parada ou `VITE_API_BASE_URL` errado | suba a API; confira o `.env` da raiz |
| Erro de CORS no console | aberto em `127.0.0.1:5173` | use `http://localhost:5173` ou ajuste `CORS_ALLOWED_ORIGIN` |
| Grafo vazio / "Entidade não encontrada" | seed não executado | `go run ./cmd/seed` |
| Ollama `degraded`: models not pulled | modelos não baixados | `docker compose exec ollama ollama pull qwen3:4b-instruct` (e o de embeddings) |
| Porta 5173 ocupada | outro Vite rodando | encerre o processo (`strictPort` impede trocar de porta) |
| Saída de `curl` truncada no terminal | proxy de terminal (ex.: RTK) resumindo JSON | `rtk proxy curl …` ou redirecione para arquivo |
| Neo4j lento ou reiniciando | pouca memória | ajuste `NEO4J_server_memory_*` no compose |
