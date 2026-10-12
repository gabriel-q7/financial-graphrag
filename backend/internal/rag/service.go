package rag

import (
	"cmp"
	"context"
	"errors"
	"fmt"
	"log/slog"
	"maps"
	"slices"
	"strings"
	"time"

	"github.com/gabrielaquino/financial-graphrag/backend/internal/graph"
)

// Retrieval and generation bounds, sized for a 4B model on CPU (see Docs/ai-models.md).
const (
	MaxQuestionChars = 500
	DocumentsK       = 4
	// MinDocumentScore filters weak vector matches. Neo4j cosine scores are (1+cos)/2.
	// Calibrated for embeddinggemma:300m: relevant documents scored 0.71–0.85,
	// unrelated questions at most 0.57 (Docs/ai-models.md).
	MinDocumentScore = 0.66
	// DocumentScoreMargin keeps only documents close to the best match, so a weakly
	// related document does not drag an unrelated part of the graph into the context.
	DocumentScoreMargin = 0.05
	// MaxLinkedDocuments caps documents reached through the graph (not by similarity).
	MaxLinkedDocuments = 2
	MaxSeedEntities    = 5
	// MaxContextNodes bounds the prompt: on CPU the model reads ~40 tokens/s.
	MaxContextNodes = 30
	NumPredict      = 450
	// Temperature 0 makes answers deterministic for the same evidence.
	Temperature = 0.0
)

// ErrEmbedding means the question could not be embedded (Ollama or model unavailable).
var ErrEmbedding = errors.New("embedding unavailable")

// ErrGeneration means the chat model failed before or during generation.
var ErrGeneration = errors.New("generation failed")

// Service answers questions with GraphRAG.
type Service struct {
	Embedder  Embedder
	Generator Generator
	Store     Store
	Graph     graph.Reader
	Log       *slog.Logger
}

// Retrieval is the evidence gathered for a question.
type Retrieval struct {
	Question   string        `json:"question"`
	Entities   []graph.Node  `json:"entities"`
	Documents  []DocumentHit `json:"documents"`
	Nodes      []graph.Node  `json:"nodes"`
	Edges      []graph.Edge  `json:"edges"`
	NoEvidence bool          `json:"noEvidence"`
	Context    Context       `json:"-"`
	ContextLen int           `json:"contextChars"`
	DurationMS int64         `json:"durationMs"`
}

// Answer is the final result of a question.
type Answer struct {
	Text       string          `json:"answer"`
	Citations  Citations       `json:"citations"`
	Model      string          `json:"model"`
	NoEvidence bool            `json:"noEvidence"`
	Stats      GenerationStats `json:"stats"`
	DurationMS int64           `json:"durationMs"`
}

// Retrieve embeds the question, searches documents, matches entities and expands the graph.
func (s *Service) Retrieve(ctx context.Context, question string) (*Retrieval, error) {
	start := time.Now()
	r := &Retrieval{Question: question, Entities: []graph.Node{}, Documents: []DocumentHit{}}

	_, queryPrefix := embedPrefixes(s.Embedder.EmbedModel())
	vectors, err := s.Embedder.Embed(ctx, []string{queryPrefix + question})
	if err != nil || len(vectors) != 1 {
		return nil, fmt.Errorf("%w: %v", ErrEmbedding, err)
	}

	hits, err := s.Store.SearchDocuments(ctx, vectors[0], DocumentsK)
	if err != nil {
		return nil, err
	}
	for _, h := range hits {
		if h.Score >= MinDocumentScore && h.Score >= hits[0].Score-DocumentScoreMargin {
			r.Documents = append(r.Documents, h)
		}
	}

	named, err := s.Store.NamedEntities(ctx)
	if err != nil {
		return nil, err
	}
	matched := MatchEntities(question, named, MaxSeedEntities)

	if len(matched) == 0 && len(r.Documents) == 0 {
		r.NoEvidence = true
		r.Nodes, r.Edges = []graph.Node{}, []graph.Edge{}
		r.DurationMS = time.Since(start).Milliseconds()
		return r, nil
	}

	ev, err := s.expand(ctx, matched, r.Documents)
	if err != nil {
		return nil, err
	}
	for _, m := range matched {
		for _, n := range ev.Nodes {
			if n.ID == m.ID {
				r.Entities = append(r.Entities, n)
			}
		}
	}
	r.Documents = ev.Documents
	r.Nodes, r.Edges = ev.Nodes, ev.Edges
	r.Context = BuildContext(ev)
	r.ContextLen = len(r.Context.Text)
	r.DurationMS = time.Since(start).Milliseconds()
	return r, nil
}

// expand collects the neighborhood of the seeds, follows money trails from the
// people, companies and accounts involved, and keeps the MaxContextNodes closest nodes.
func (s *Service) expand(ctx context.Context, matched []NamedEntity, docs []DocumentHit) (Evidence, error) {
	nodes := map[string]graph.Node{}
	edges := map[string]graph.Edge{}
	collect := func(id string, depth, limit int, types []graph.RelType) error {
		sub, err := s.Graph.Neighborhood(ctx, graph.NeighborQuery{ID: id, Depth: depth, Limit: limit, RelationshipTypes: types})
		if errors.Is(err, graph.ErrNotFound) {
			return nil
		}
		if err != nil {
			return err
		}
		for _, n := range sub.Nodes {
			nodes[n.ID] = n
		}
		for _, e := range sub.Edges {
			edges[e.ID] = e
		}
		return nil
	}

	var seeds []string
	for _, m := range matched {
		seeds = append(seeds, m.ID)
	}
	for _, d := range docs {
		seeds = append(seeds, d.Node.ID)
	}
	for _, id := range seeds {
		if err := collect(id, 1, 25, nil); err != nil {
			return Evidence{}, err
		}
	}

	// Money trails start at matched entities and at entities mentioned by retrieved documents.
	trailStarts := map[string]bool{}
	for _, m := range matched {
		trailStarts[m.ID] = true
	}
	for _, e := range edges {
		if e.Type == graph.Mentions && slices.ContainsFunc(docs, func(d DocumentHit) bool { return d.Node.ID == e.Source }) {
			trailStarts[e.Target] = true
		}
	}
	money := []graph.RelType{graph.Owns, graph.Sent, graph.ToAccount}
	for _, id := range slices.Sorted(maps.Keys(trailStarts)) {
		// Direct context of the entity (devices, companies, accounts)…
		if err := collect(id, 1, 25, nil); err != nil {
			return Evidence{}, err
		}
		// …and where its money went.
		if t, _ := graph.TypeForID(id); t == graph.Person || t == graph.Company || t == graph.BankAccount {
			if err := collect(id, 3, 25, money); err != nil {
				return Evidence{}, err
			}
		}
	}

	// Rank by hop distance from the seeds and keep the closest nodes.
	dist := distances(seeds, edges)
	ranked := make([]graph.Node, 0, len(nodes))
	for _, n := range nodes {
		ranked = append(ranked, n)
	}
	slices.SortFunc(ranked, func(a, b graph.Node) int {
		return cmp.Or(cmp.Compare(distOr(dist, a.ID), distOr(dist, b.ID)), strings.Compare(a.ID, b.ID))
	})
	if len(ranked) > MaxContextNodes {
		ranked = ranked[:MaxContextNodes]
	}
	kept := map[string]bool{}
	ev := Evidence{Documents: slices.Clone(docs)}
	retrieved := map[string]bool{}
	for _, d := range docs {
		retrieved[d.Node.ID] = true
	}
	linked := 0
	for _, n := range ranked {
		if n.Type != graph.Document {
			kept[n.ID] = true
		}
		if n.Type == graph.Document {
			if !retrieved[n.ID] && linked < MaxLinkedDocuments {
				// Reached through the graph, not by similarity.
				ev.Documents = append(ev.Documents, DocumentHit{Node: n, Score: -1})
				linked++
			}
			continue
		}
		ev.Nodes = append(ev.Nodes, n)
	}
	for _, d := range ev.Documents {
		kept[d.Node.ID] = true
	}
	for _, e := range edges {
		if kept[e.Source] && kept[e.Target] {
			ev.Edges = append(ev.Edges, e)
		}
	}
	slices.SortFunc(ev.Edges, func(a, b graph.Edge) int { return strings.Compare(a.ID, b.ID) })
	if ev.Nodes == nil {
		ev.Nodes = []graph.Node{}
	}
	if ev.Edges == nil {
		ev.Edges = []graph.Edge{}
	}
	return ev, nil
}

// Generate streams the grounded answer. onToken receives every fragment.
func (s *Service) Generate(ctx context.Context, r *Retrieval, onToken func(string) error) (Answer, error) {
	start := time.Now()
	ans := Answer{Model: s.Generator.ChatModel(), Citations: Citations{Valid: []string{}, Invalid: []string{}}}
	if r.NoEvidence {
		ans.Text, ans.NoEvidence = NoEvidenceAnswer, true
		if err := onToken(NoEvidenceAnswer); err != nil {
			return ans, err
		}
		return ans, nil
	}

	var b strings.Builder
	stats, err := s.Generator.Generate(ctx, systemPrompt, userPrompt(r.Context.Text, r.Question),
		GenerationOptions{MaxTokens: NumPredict, Temperature: Temperature},
		func(tok string) error {
			b.WriteString(tok)
			return onToken(tok)
		})
	if err != nil {
		if ctx.Err() != nil {
			return ans, ctx.Err()
		}
		return ans, fmt.Errorf("%w: %v", ErrGeneration, err)
	}
	ans.Text = b.String()
	ans.Stats = stats
	ans.Citations = ExtractCitations(ans.Text, r.Context.IDs)
	ans.DurationMS = time.Since(start).Milliseconds()
	return ans, nil
}

// distances is a multi-source BFS over undirected edges.
func distances(seeds []string, edges map[string]graph.Edge) map[string]int {
	adj := map[string][]string{}
	for _, e := range edges {
		adj[e.Source] = append(adj[e.Source], e.Target)
		adj[e.Target] = append(adj[e.Target], e.Source)
	}
	dist := map[string]int{}
	queue := []string{}
	for _, s := range seeds {
		if _, ok := dist[s]; !ok {
			dist[s] = 0
			queue = append(queue, s)
		}
	}
	for i := 0; i < len(queue); i++ {
		cur := queue[i]
		for _, next := range adj[cur] {
			if _, ok := dist[next]; !ok {
				dist[next] = dist[cur] + 1
				queue = append(queue, next)
			}
		}
	}
	return dist
}

func distOr(d map[string]int, id string) int {
	if v, ok := d[id]; ok {
		return v
	}
	return 1 << 30
}
