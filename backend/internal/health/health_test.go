package health

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"strings"
	"testing"
	"time"
)

type fakePinger struct{ err error }

func (f fakePinger) VerifyConnectivity(context.Context) error { return f.err }

type fakeOllama struct {
	versionErr error
	models     []string
}

func (f fakeOllama) Version(context.Context) (string, error)  { return "0.34.4", f.versionErr }
func (f fakeOllama) Models(context.Context) ([]string, error) { return f.models, nil }
func (f fakeOllama) ChatModel() string                        { return "llama3.2:3b" }
func (f fakeOllama) EmbedModel() string                       { return "nomic-embed-text" }

func checker(neoErr error, o fakeOllama) *Checker {
	return &Checker{Neo4j: fakePinger{neoErr}, Ollama: o, Timeout: time.Second, Log: slog.New(slog.NewTextHandler(io.Discard, nil))}
}

func TestHealthStatuses(t *testing.T) {
	allModels := []string{"llama3.2:3b", "nomic-embed-text:latest"}
	cases := []struct {
		name   string
		neoErr error
		ollama fakeOllama
		want   string
	}{
		{"all ok", nil, fakeOllama{models: allModels}, StatusOK},
		{"models missing", nil, fakeOllama{models: nil}, StatusDegraded},
		{"ollama down", nil, fakeOllama{versionErr: errors.New("dial tcp: refused")}, StatusDegraded},
		{"neo4j down", errors.New("auth failure for user neo4j password=hunter2"), fakeOllama{models: allModels}, StatusUnavailable},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			r := checker(c.neoErr, c.ollama).Run(context.Background())
			if r.Status != c.want {
				t.Fatalf("status = %s, want %s (%+v)", r.Status, c.want, r.Checks)
			}
			for _, ch := range r.Checks {
				if strings.Contains(ch.Error, "hunter2") || strings.Contains(ch.Error, "dial tcp") {
					t.Fatalf("internal error leaked: %q", ch.Error)
				}
			}
		})
	}
}
