package rag

import "strings"

// Citations splits the ids mentioned in an answer into those present in the
// context (valid) and those that are not (invalid, i.e. possibly hallucinated).
type Citations struct {
	Valid   []string `json:"valid"`
	Invalid []string `json:"invalid"`
}

// ExtractCitations finds every entity id in the answer, in order of first appearance.
func ExtractCitations(answer string, contextIDs map[string]bool) Citations {
	c := Citations{Valid: []string{}, Invalid: []string{}}
	seen := map[string]bool{}
	for _, id := range idInText.FindAllString(strings.ToLower(answer), -1) {
		if seen[id] {
			continue
		}
		seen[id] = true
		if contextIDs[id] {
			c.Valid = append(c.Valid, id)
		} else {
			c.Invalid = append(c.Invalid, id)
		}
	}
	return c
}
