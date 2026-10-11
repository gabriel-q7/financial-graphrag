# Referência da API

Base: `http://localhost:8080` (configurável por `HTTP_ADDR`). Todas as respostas são
`application/json; charset=utf-8`. Toda resposta inclui `X-Request-ID` (gerado, ou o enviado
pelo cliente se tiver até 64 caracteres `[A-Za-z0-9_-]`).

## Formato de erro

```json
{ "error": { "code": "invalid_parameter", "message": "must be an integer between 1 and 3", "field": "depth" } }
```

| `code` | HTTP | Quando |
| --- | --- | --- |
| `invalid_parameter` | 400 | id ou parâmetro de consulta inválido (`field` indica qual) |
| `unauthorized` | 401 | token administrativo ausente ou errado |
| `forbidden` | 403 | rota administrativa chamada de fora do loopback |
| `not_found` | 404 | entidade inexistente ou rota desconhecida |
| `seed_failed` | 500 | falha no seed (detalhes só no log) |
| `internal` | 500 | panic capturado |
| `graph_unavailable` | 503 | Neo4j inacessível ou erro de consulta |
| `timeout` | 504 | consulta excedeu 10 s |

Mensagens nunca contêm credenciais, URIs internas, mensagens do driver ou stack traces.

## Identificadores

`^(per|cmp|acc|txn|dev|doc)_[0-9]{3,9}$` — o prefixo determina o tipo
(ver [Modelo de dados](data-model.md#identificadores)). Ids fora do padrão retornam `400`
antes de qualquer acesso ao banco.

## Objetos

```ts
Node     { id: string; type: NodeType; properties: Record<string, string | number | boolean | array> }
Edge     { id: string; type: RelType; source: string; target: string }   // direção source → target
Meta     { depth; nodeLimit; edgeLimit; relationshipTypes: RelType[]; nodeCount; edgeCount; truncated: boolean }
Subgraph { center: Node; nodes: Node[]; edges: Edge[]; meta: Meta }      // nodes inclui o center
```

- `Edge.id` = `"<source>:<TYPE>:<target>"`, estável entre chamadas — use para deduplicar.
- Datas (`timestamp`, `created_at`) em RFC 3339 UTC.
- `nodes` e `edges` vêm ordenados por `id`.

---

## `GET /api/health`

Estado da API e das dependências. Sempre `Cache-Control: no-store`.

| Status geral | HTTP | Significado |
| --- | --- | --- |
| `ok` | 200 | tudo disponível |
| `degraded` | 200 | Ollama inacessível ou sem os modelos configurados |
| `unavailable` | 503 | Neo4j inacessível |

```json
{
  "status": "degraded",
  "checks": {
    "neo4j": { "status": "ok", "essential": true, "latencyMs": 2 },
    "ollama": {
      "status": "degraded", "essential": false, "latencyMs": 1,
      "error": "configured models are not pulled yet (see README)",
      "details": {
        "version": "0.34.4",
        "chatModel": { "name": "qwen3:4b-instruct", "available": true },
        "embeddingModel": { "name": "nomic-embed-text", "available": false }
      }
    }
  },
  "timestamp": "2026-10-11T21:29:28Z"
}
```

## `GET /api/meta`

Schema exposto e limites do servidor. O frontend usa para montar filtros e opções.

```json
{
  "nodeTypes": [{ "type": "Person", "idPrefix": "per", "properties": ["name"] }, "..."],
  "relationshipTypes": [{ "type": "OWNS", "from": ["Person", "Company"], "to": ["BankAccount"] }, "..."],
  "limits": { "defaultDepth": 1, "maxDepth": 3, "defaultLimit": 25, "maxLimit": 100, "edgeLimitFactor": 3 }
}
```

## `GET /api/entities`

Busca entidades por id ou nome (substring, sem diferenciar maiúsculas).

| Parâmetro | Padrão | Regra |
| --- | --- | --- |
| `q` | vazio (todos) | até 100 caracteres |
| `type` | todos | um de `Person, Company, BankAccount, Transaction, Device, Document` |
| `limit` | 20 | 1–50 |

```bash
curl -s 'http://localhost:8080/api/entities?q=lins'
# {"items":[{"id":"per_006","type":"Person","properties":{"name":"Fábio Lins"}}],"limit":20}
```

## `GET /api/graph/{id}`

Entidade e suas conexões diretas. Profundidade fixa em 1 e todos os tipos de relacionamento;
`depth` e `relationshipTypes` são ignorados aqui.

| Parâmetro | Padrão | Regra |
| --- | --- | --- |
| `limit` | 25 | 1–100 vizinhos |

```bash
curl -s http://localhost:8080/api/graph/per_001
```

Retorna 6 nós (Ana + conta, 2 empresas, dispositivo, documento) e 6 arestas — as 5 diretas
mais `doc_001 MENTIONS cmp_002`, pois o subgrafo inclui relacionamentos entre os nós retornados.

## `GET /api/graph/{id}/neighbors`

Vizinhança com travessia em largura limitada.

| Parâmetro | Padrão | Regra |
| --- | --- | --- |
| `depth` | 1 | 1–3 |
| `limit` | 25 | 1–100 nós além do central |
| `relationshipTypes` | todos | lista de `OWNS, CONTROLS, SENT, TO_ACCOUNT, USES, MENTIONS`; vírgula ou repetido |

Máximo de relacionamentos: `3 × limit`. O filtro de tipos vale tanto para a travessia quanto
para as arestas retornadas.

```bash
# Seguir o dinheiro a partir da conta de Ana
curl -s 'http://localhost:8080/api/graph/acc_001/neighbors?depth=3&limit=10&relationshipTypes=SENT,TO_ACCOUNT'
# meta: {"depth":3,"nodeLimit":10,"edgeLimit":30,"relationshipTypes":["SENT","TO_ACCOUNT"],"nodeCount":9,"edgeCount":8,"truncated":false}
# nós: acc_001, acc_005, acc_010, txn_005, txn_006, txn_007, txn_008, txn_009, txn_017
```

Quando `meta.truncated` é `true`, algum limite cortou o resultado: os nós de um nível foram
escolhidos por ordem de `id`, não por relevância.

## `POST /api/admin/seed`

Aplica migrações pendentes e carrega o dataset. Disponível apenas quando **todas** as
condições abaixo são verdadeiras; caso contrário a rota não existe (404) ou é recusada.

1. `ADMIN_SEED_ENABLED=true` e `APP_ENV` diferente de `production` (senão 404);
2. requisição vinda de `127.0.0.1`/`::1` (senão 403);
3. header `X-Admin-Token` igual a `ADMIN_TOKEN` (senão 401).

```bash
set -a; source .env; set +a
curl -s -X POST -H "X-Admin-Token: $ADMIN_TOKEN" http://localhost:8080/api/admin/seed
# {"version":"2026-10-11.1","nodes":56,"relationships":77,"nodesCreated":0,"relationshipsCreated":0,"propertiesSet":140}
```

`nodesCreated`/`relationshipsCreated` iguais a 0 numa reexecução confirmam a idempotência.

## CORS

Apenas a origem `CORS_ALLOWED_ORIGIN` (padrão `http://localhost:5173`) recebe
`Access-Control-Allow-Origin`. Métodos `GET, POST, OPTIONS`; headers
`Content-Type, X-Admin-Token, X-Request-ID`; preflight responde `204` com `Max-Age: 600`.

## Rotas desconhecidas

Qualquer outra rota (ou método) responde `404 not_found` em JSON.
