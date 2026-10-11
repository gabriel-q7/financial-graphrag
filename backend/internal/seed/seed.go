package seed

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/gabrielaquino/financial-graphrag/backend/internal/graph"
	"github.com/neo4j/neo4j-go-driver/v6/neo4j"
)

// nodeQueries maps each label to a fixed MERGE statement. Labels and property
// names come from this table, never from the input file, so values are always
// passed as parameters. SET (not ON CREATE SET) keeps re-runs convergent.
var nodeQueries = map[graph.NodeType]string{
	graph.Person: `UNWIND $rows AS row MERGE (n:Entity {id: row.id})
SET n:Person, n.name = row.name`,
	graph.Company: `UNWIND $rows AS row MERGE (n:Entity {id: row.id})
SET n:Company, n.name = row.name, n.sector = row.sector`,
	graph.BankAccount: `UNWIND $rows AS row MERGE (n:Entity {id: row.id})
SET n:BankAccount, n.bank = row.bank, n.status = row.status`,
	graph.Transaction: `UNWIND $rows AS row MERGE (n:Entity {id: row.id})
SET n:Transaction, n.amount = toFloat(row.amount), n.currency = row.currency,
    n.status = row.status, n.timestamp = datetime(row.timestamp)`,
	graph.Device: `UNWIND $rows AS row MERGE (n:Entity {id: row.id})
SET n:Device, n.device_type = row.device_type`,
	graph.Document: `UNWIND $rows AS row MERGE (n:Entity {id: row.id})
SET n:Document, n.text = row.text, n.source = row.source, n.created_at = datetime(row.created_at)`,
}

// relQueries holds one fixed MERGE statement per relationship type.
var relQueries = func() map[graph.RelType]string {
	m := map[graph.RelType]string{}
	for _, s := range graph.RelSpecs {
		// s.Type is a compile-time constant from graph.RelSpecs.
		m[s.Type] = fmt.Sprintf(`UNWIND $rows AS row
MATCH (a:Entity {id: row.from}), (b:Entity {id: row.to})
MERGE (a)-[:%s]->(b)`, s.Type)
	}
	return m
}()

// Result summarizes a seed run. Created counters are 0 on an idempotent re-run.
type Result struct {
	Version              string `json:"version"`
	Nodes                int    `json:"nodes"`
	Relationships        int    `json:"relationships"`
	NodesCreated         int    `json:"nodesCreated"`
	RelationshipsCreated int    `json:"relationshipsCreated"`
	PropertiesSet        int    `json:"propertiesSet"`
}

// Run writes the dataset in a single write transaction.
func Run(ctx context.Context, driver neo4j.Driver, database string, d Dataset) (Result, error) {
	if err := d.Validate(); err != nil {
		return Result{}, err
	}
	nodeRows := map[graph.NodeType][]map[string]any{}
	for _, n := range d.Nodes {
		row := map[string]any{"id": n.ID}
		for k, v := range n.Properties {
			if num, ok := v.(json.Number); ok {
				f, _ := num.Float64()
				v = f
			}
			row[k] = v
		}
		nodeRows[n.Label] = append(nodeRows[n.Label], row)
	}
	relRows := map[graph.RelType][]map[string]any{}
	for _, r := range d.Relationships {
		relRows[r.Type] = append(relRows[r.Type], map[string]any{"from": r.From, "to": r.To})
	}

	session := driver.NewSession(ctx, neo4j.SessionConfig{DatabaseName: database, AccessMode: neo4j.AccessModeWrite})
	defer session.Close(ctx)

	res, err := neo4j.ExecuteWrite(ctx, session, func(tx neo4j.ManagedTransaction) (Result, error) {
		var out Result
		run := func(query string, rows []map[string]any) error {
			if len(rows) == 0 {
				return nil
			}
			r, err := tx.Run(ctx, query, map[string]any{"rows": rows})
			if err != nil {
				return err
			}
			sum, err := r.Consume(ctx)
			if err != nil {
				return err
			}
			c := sum.Counters()
			out.NodesCreated += c.NodesCreated()
			out.RelationshipsCreated += c.RelationshipsCreated()
			out.PropertiesSet += c.PropertiesSet()
			return nil
		}
		for _, spec := range graph.NodeSpecs {
			if err := run(nodeQueries[spec.Type], nodeRows[spec.Type]); err != nil {
				return out, fmt.Errorf("seed %s nodes: %w", spec.Type, err)
			}
		}
		for _, spec := range graph.RelSpecs {
			if err := run(relQueries[spec.Type], relRows[spec.Type]); err != nil {
				return out, fmt.Errorf("seed %s relationships: %w", spec.Type, err)
			}
		}
		return out, nil
	})
	if err != nil {
		return Result{}, err
	}
	res.Version = d.Version
	res.Nodes = len(d.Nodes)
	res.Relationships = len(d.Relationships)
	return res, nil
}
