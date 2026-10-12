package rag

import (
	"regexp"
	"slices"
	"strings"
	"unicode"

	"github.com/gabrielaquino/financial-graphrag/backend/internal/graph"
)

// NamedEntity is a node that can be referenced by name in a question.
type NamedEntity struct {
	ID   string
	Type graph.NodeType
	Name string
}

var idInText = regexp.MustCompile(`\b(per|cmp|acc|txn|dev|doc)_[0-9]{3,9}\b`)

// Generic words that never identify an entity on their own.
var genericTokens = map[string]bool{
	"ltda": true, "sa": true, "me": true, "eireli": true, "ficticia": true, "ficticio": true,
	"comercio": true, "consultoria": true, "logistica": true, "importadora": true,
	"servicos": true, "digitais": true, "banco": true, "empresa": true, "conta": true,
	"da": true, "de": true, "do": true, "das": true, "dos": true, "e": true,
}

var accentReplacer = strings.NewReplacer(
	"á", "a", "à", "a", "â", "a", "ã", "a", "ä", "a",
	"é", "e", "è", "e", "ê", "e", "ë", "e",
	"í", "i", "ì", "i", "î", "i", "ï", "i",
	"ó", "o", "ò", "o", "ô", "o", "õ", "o", "ö", "o",
	"ú", "u", "ù", "u", "û", "u", "ü", "u", "ç", "c", "ñ", "n",
)

// tokens lowercases, removes accents and parenthetical notes, and splits into words.
func tokens(s string) []string {
	s = accentReplacer.Replace(strings.ToLower(s))
	if i := strings.Index(s, "("); i >= 0 {
		if j := strings.Index(s[i:], ")"); j >= 0 {
			s = s[:i] + " " + s[i+j+1:]
		}
	}
	return strings.FieldsFunc(s, func(r rune) bool { return !unicode.IsLetter(r) && !unicode.IsDigit(r) })
}

// MatchEntities finds entities referenced in the question, in order of appearance:
// explicit ids (e.g. "acc_007"), full names ("Ana Ribeiro"), or a distinctive single
// name token ("Ribeiro", "Borealis"). Returns at most max entities.
func MatchEntities(question string, entities []NamedEntity, max int) []NamedEntity {
	type hit struct {
		e   NamedEntity
		pos int
	}
	var hits []hit
	seen := map[string]bool{}
	add := func(e NamedEntity, pos int) {
		if !seen[e.ID] {
			seen[e.ID] = true
			hits = append(hits, hit{e, pos})
		}
	}

	lower := strings.ToLower(question)
	for _, loc := range idInText.FindAllStringIndex(lower, -1) {
		id := lower[loc[0]:loc[1]]
		if t, ok := graph.TypeForID(id); ok {
			name := ""
			for _, e := range entities {
				if e.ID == id {
					name = e.Name
				}
			}
			add(NamedEntity{ID: id, Type: t, Name: name}, loc[0])
		}
	}

	q := tokens(question)
	for _, e := range entities {
		name := tokens(e.Name)
		if len(name) == 0 {
			continue
		}
		if pos := indexOfSeq(q, name); pos >= 0 {
			add(e, 1000+pos)
			continue
		}
		for _, cand := range []string{name[0], name[len(name)-1]} {
			if len(cand) < 3 || genericTokens[cand] {
				continue
			}
			if pos := slices.Index(q, cand); pos >= 0 {
				add(e, 1000+pos)
				break
			}
		}
	}

	slices.SortStableFunc(hits, func(a, b hit) int { return a.pos - b.pos })
	out := make([]NamedEntity, 0, min(len(hits), max))
	for _, h := range hits {
		if len(out) == max {
			break
		}
		out = append(out, h.e)
	}
	return out
}

func indexOfSeq(haystack, needle []string) int {
	for i := 0; i+len(needle) <= len(haystack); i++ {
		if slices.Equal(haystack[i:i+len(needle)], needle) {
			return i
		}
	}
	return -1
}
