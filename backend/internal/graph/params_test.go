package graph

import (
	"errors"
	"net/url"
	"slices"
	"testing"
)

func TestValidateID(t *testing.T) {
	valid := []string{"per_001", "acc_123456789", "doc_999"}
	invalid := []string{"", "per_01", "xyz_001", "per-001", "PER_001", "per_001'", "per_001 OR 1=1", "per_0010000000"}
	for _, id := range valid {
		if err := ValidateID(id); err != nil {
			t.Errorf("%q should be valid", id)
		}
	}
	for _, id := range invalid {
		if err := ValidateID(id); !errors.Is(err, ErrInvalidID) {
			t.Errorf("%q should be invalid", id)
		}
	}
}

func TestParseNeighborQueryDefaults(t *testing.T) {
	q, err := ParseNeighborQuery("per_001", url.Values{})
	if err != nil {
		t.Fatal(err)
	}
	if q.Depth != DefaultDepth || q.Limit != DefaultLimit || len(q.RelationshipTypes) != 0 {
		t.Fatalf("unexpected defaults: %+v", q)
	}
	if q.EdgeLimit() != DefaultLimit*EdgeLimitFactor {
		t.Fatalf("unexpected edge limit %d", q.EdgeLimit())
	}
}

func TestParseNeighborQueryValues(t *testing.T) {
	v := url.Values{
		"depth":             {"3"},
		"limit":             {"100"},
		"relationshipTypes": {"OWNS,SENT", "OWNS", " TO_ACCOUNT "},
	}
	q, err := ParseNeighborQuery("acc_001", v)
	if err != nil {
		t.Fatal(err)
	}
	want := []RelType{Owns, Sent, ToAccount}
	if q.Depth != 3 || q.Limit != 100 || !slices.Equal(q.RelationshipTypes, want) {
		t.Fatalf("unexpected query: %+v", q)
	}
}

func TestParseNeighborQueryRejects(t *testing.T) {
	cases := map[string]struct {
		id    string
		v     url.Values
		field string
	}{
		"bad id":          {"per_1", url.Values{}, "id"},
		"depth too big":   {"per_001", url.Values{"depth": {"4"}}, "depth"},
		"depth zero":      {"per_001", url.Values{"depth": {"0"}}, "depth"},
		"depth not int":   {"per_001", url.Values{"depth": {"two"}}, "depth"},
		"limit too big":   {"per_001", url.Values{"limit": {"101"}}, "limit"},
		"limit negative":  {"per_001", url.Values{"limit": {"-5"}}, "limit"},
		"unknown rel":     {"per_001", url.Values{"relationshipTypes": {"OWNS,KNOWS"}}, "relationshipTypes"},
		"injection in rt": {"per_001", url.Values{"relationshipTypes": {"OWNS]->() DETACH DELETE n//"}}, "relationshipTypes"},
	}
	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			_, err := ParseNeighborQuery(c.id, c.v)
			var ve *ValidationError
			if !errors.As(err, &ve) || ve.Field != c.field {
				t.Fatalf("expected validation error on %s, got %v", c.field, err)
			}
		})
	}
}

func TestParseSearchQuery(t *testing.T) {
	s, err := ParseSearchQuery(url.Values{"q": {"  Ana "}, "type": {"Person"}, "limit": {"5"}})
	if err != nil || s.Text != "Ana" || s.Type != Person || s.Limit != 5 {
		t.Fatalf("unexpected: %+v %v", s, err)
	}
	if _, err := ParseSearchQuery(url.Values{"type": {"Alien"}}); err == nil {
		t.Error("expected error for unknown type")
	}
	if _, err := ParseSearchQuery(url.Values{"limit": {"51"}}); err == nil {
		t.Error("expected error for limit above max")
	}
}

func TestTypeForID(t *testing.T) {
	if typ, ok := TypeForID("txn_001"); !ok || typ != Transaction {
		t.Fatalf("got %v %v", typ, ok)
	}
	if _, ok := TypeForID("bad"); ok {
		t.Fatal("expected false")
	}
}
