# Documentação — Financial GraphRAG

Documentação técnica do projeto. O [README da raiz](../README.md) é o guia rápido de instalação;
estes documentos aprofundam cada parte. Para rodar tudo: `make dev` (detalhes em [Desenvolvimento](development.md)).

| Documento | Conteúdo | Para quem |
| --- | --- | --- |
| [Arquitetura](architecture.md) | Componentes, fluxos de requisição, ciclo de vida, decisões de projeto | Quem vai evoluir o sistema |
| [Desenvolvimento](development.md) | Ambiente local, comandos, testes, convenções, solução de problemas | Quem vai rodar e alterar o código |
| [Referência da API](api.md) | Endpoints, parâmetros, limites, formatos de resposta e de erro | Quem consome a API (frontend, scripts) |
| [Modelo de dados](data-model.md) | Schema do grafo, ids, migrações, dataset sintético e padrões | Quem consulta ou altera o grafo |
| [Frontend](frontend.md) | Estrutura React, estado do grafo, merge/layout, estados de UI | Quem altera a interface |
| [IA local (Ollama)](ai-models.md) | Modelos escolhidos, benchmark no notebook, embeddings, parâmetros | Quem vai implementar o GraphRAG |
| [Roadmap GraphRAG](graphrag-roadmap.md) | Plano da próxima etapa: ingestão, embeddings, índice vetorial, recuperação, respostas | Planejamento |
| [Segurança](security.md) | Controles implementados, superfície exposta, riscos conhecidos | Revisão de segurança |

## Visão geral em uma frase

Uma API Go lê um grafo financeiro sintético no Neo4j e o entrega a uma interface React Flow
que o explora de forma progressiva; perguntas em linguagem natural são respondidas por uma LLM
local (Ollama, via langchaingo) usando só evidências recuperadas do grafo — busca vetorial nos
documentos, entidades citadas na pergunta e travessia a partir delas — com os ids citados
validados.

## Estado atual

| Item | Estado |
| --- | --- |
| Neo4j + Ollama via Docker Compose | Implementado |
| Migrações e seed idempotentes | Implementado e testado contra Neo4j real |
| API: saúde, meta, busca, grafo, vizinhança, seed admin | Implementado |
| Frontend de exploração | Implementado e verificado em navegador |
| Embeddings dos documentos + índice vetorial no Neo4j | Implementado (`make index`, idempotente) |
| Perguntas com GraphRAG (`POST /api/ask`, streaming SSE) | Implementado e verificado com os modelos reais |
| Aba "Perguntar à IA" com citações clicáveis e evidências no grafo | Implementado e verificado em navegador |
| Camada de modelos via langchaingo | Implementado (`backend/internal/llm`) |
| Ingestão de documentos novos, extração de entidades | Pendente ([Roadmap](graphrag-roadmap.md)) |
