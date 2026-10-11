# Segurança

Escopo: ambiente de **desenvolvimento local** com dados sintéticos. O projeto não está pronto
para ser exposto em rede; os itens em [Riscos conhecidos](#riscos-conhecidos) precisam ser
tratados antes disso.

## Superfície exposta

| Serviço | Endereço | Autenticação |
| --- | --- | --- |
| API Go | `127.0.0.1:8080` | nenhuma nas leituras; token + loopback no seed admin |
| Vite dev server | `localhost:5173` | nenhuma |
| Neo4j Bolt / Browser | `127.0.0.1:7687` / `127.0.0.1:7474` | usuário e senha do `.env` |
| Ollama | `127.0.0.1:11434` | nenhuma (padrão do Ollama) |

Todas as portas do Docker são publicadas apenas no loopback.

## Controles implementados

### Entrada e consultas

- Ids validados por regex antes de qualquer consulta (`400` caso contrário).
- `depth`, `limit` e `relationshipTypes` validados contra faixas e lista fechada; o servidor
  impõe máximos (profundidade 3, 100 nós, 300 relacionamentos, busca até 50 itens e 100 caracteres).
- Cypher sempre parametrizado. Rótulos e tipos de relacionamento usados em texto de consulta
  vêm de constantes do código (`graph.RelSpecs`, `seed.nodeQueries`), nunca de entrada externa.
- Testes cobrem tentativas como `per_001' OR 1=1` e `OWNS]->() DETACH DELETE n//`.
- Corpo de requisição limitado a 1 MB; seed JSON rejeita campos desconhecidos.

### Respostas e erros

- Erros ao cliente são genéricos (`graph_unavailable`, `seed_failed`); detalhes só no log.
- `Recover` converte panics em `500` sem stack trace.
- Testes garantem que mensagens com senha/URI não vazam (health, grafo, seed).
- Apenas propriedades na whitelist de cada tipo são retornadas.
- Cabeçalhos: `X-Content-Type-Options: nosniff`, `X-Frame-Options: DENY`,
  `Referrer-Policy: no-referrer`, `Content-Security-Policy: default-src 'none'; frame-ancestors 'none'`.

### CORS

Uma única origem permitida (`CORS_ALLOWED_ORIGIN`), sem curingas e sem credenciais.

### Segredos

- Nenhuma credencial no código; `.env` está no `.gitignore`; `.env.example` só tem valores ilustrativos.
- A API não inicia sem `NEO4J_PASSWORD`; com o seed admin habilitado, exige `ADMIN_TOKEN` ≥ 16 caracteres.
- O token é comparado em tempo constante (`crypto/subtle`).
- Gerar valores locais: `openssl rand -hex 16` (senha) e `openssl rand -hex 24` (token).

### Endpoint administrativo

`POST /api/admin/seed` exige, cumulativamente: `ADMIN_SEED_ENABLED=true`, `APP_ENV≠production`
(senão a rota nem é registrada), cliente loopback e `X-Admin-Token` correto.

### Disponibilidade

Timeouts em todas as camadas (servidor HTTP, consultas, driver, health, cliente Ollama) e
travessias com custo limitado, evitando que uma requisição prenda recursos indefinidamente.

### Dados

Todo o dataset é fictício. Bancos têm nomes marcados como fictícios; não há CPF, CNPJ,
número de conta ou agência. Os documentos registram que padrões não são conclusões.

## Riscos conhecidos

| Risco | Impacto | Antes de expor em rede |
| --- | --- | --- |
| Leituras sem autenticação | qualquer processo local lê o grafo | autenticação (ex.: OIDC) e autorização por recurso |
| Sem rate limiting | abuso de consultas pesadas | rate limit por cliente |
| Ollama sem autenticação | processos locais usam/baixam modelos | manter só no loopback ou proxy autenticado |
| Seed admin depende de `RemoteAddr` | atrás de proxy reverso, todo tráfego pareceria loopback | desabilitar (`ADMIN_SEED_ENABLED=false`) ou autenticar de outra forma |
| Neo4j sem TLS | tráfego Bolt em claro | `neo4j+s://` e certificados |
| Vite dev server | não é servidor de produção | servir o build estático (`npm run build`) |
| Etapa 2: texto de documentos vai para a LLM | injeção de instruções | ver [Roadmap — riscos](graphrag-roadmap.md#riscos) |

## Checklist para mudanças

- [ ] Nenhuma entrada de usuário concatenada em Cypher.
- [ ] Novos parâmetros validados e com máximo no servidor.
- [ ] Novas propriedades expostas adicionadas conscientemente à whitelist.
- [ ] Erros novos não devolvem `err.Error()` ao cliente.
- [ ] Nada de segredo em código, logs ou `.env.example`.
- [ ] Testes cobrindo entrada inválida.
