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
que o explora de forma progressiva; o Ollama já está integrado (saúde e embeddings) para a
etapa seguinte, em que perguntas em linguagem natural serão respondidas com recuperação
vetorial + travessia do grafo.

## Estado atual (etapa 1)

| Item | Estado |
| --- | --- |
| Neo4j + Ollama via Docker Compose | Implementado |
| Migrações e seed idempotentes | Implementado e testado contra Neo4j real |
| API: saúde, meta, busca, grafo, vizinhança, seed admin | Implementado |
| Frontend de exploração | Implementado e verificado em navegador |
| Cliente Ollama: versão, modelos, embeddings | Implementado (embeddings testados só com servidor falso) |
| Extração, índice vetorial, recuperação, geração | Apenas interfaces (`backend/internal/rag`) |
| Endpoint de chat | Não existe, por decisão (ver [Arquitetura](architecture.md#decisões)) |
