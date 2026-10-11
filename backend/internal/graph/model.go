// Package graph holds the financial graph domain model, request validation,
// DTOs returned by the API and the Neo4j repository that produces them.
package graph

import (
	"errors"
	"regexp"
	"slices"
)

// NodeType is a domain label. Every node also carries the technical :Entity label.
type NodeType string

const (
	Person      NodeType = "Person"
	Company     NodeType = "Company"
	BankAccount NodeType = "BankAccount"
	Transaction NodeType = "Transaction"
	Device      NodeType = "Device"
	Document    NodeType = "Document"
)

// RelType is a relationship type.
type RelType string

const (
	Owns      RelType = "OWNS"
	Controls  RelType = "CONTROLS"
	Sent      RelType = "SENT"
	ToAccount RelType = "TO_ACCOUNT"
	Uses      RelType = "USES"
	Mentions  RelType = "MENTIONS"
)

// RelSpec documents the allowed direction of a relationship type.
type RelSpec struct {
	Type RelType    `json:"type"`
	From []NodeType `json:"from"`
	To   []NodeType `json:"to"`
}

// NodeSpec describes a node type: id prefix and properties exposed by the API.
type NodeSpec struct {
	Type       NodeType `json:"type"`
	IDPrefix   string   `json:"idPrefix"`
	Properties []string `json:"properties"`
}

// NodeSpecs is the closed set of node types. Only listed properties are ever
// returned to clients, so technical fields (e.g. future embeddings) stay internal.
var NodeSpecs = []NodeSpec{
	{Person, "per", []string{"name"}},
	{Company, "cmp", []string{"name", "sector"}},
	{BankAccount, "acc", []string{"bank", "status"}},
	{Transaction, "txn", []string{"amount", "currency", "status", "timestamp"}},
	{Device, "dev", []string{"device_type"}},
	{Document, "doc", []string{"text", "source", "created_at"}},
}

// RelSpecs is the closed set of relationship types and their direction.
var RelSpecs = []RelSpec{
	{Owns, []NodeType{Person, Company}, []NodeType{BankAccount}},
	{Controls, []NodeType{Person}, []NodeType{Company}},
	{Sent, []NodeType{BankAccount}, []NodeType{Transaction}},
	{ToAccount, []NodeType{Transaction}, []NodeType{BankAccount}},
	{Uses, []NodeType{Person}, []NodeType{Device}},
	{Mentions, []NodeType{Document}, []NodeType{Person, Company}},
}

// Entity IDs look like "per_001": a type prefix, underscore and 3-9 digits.
var idPattern = regexp.MustCompile(`^(per|cmp|acc|txn|dev|doc)_[0-9]{3,9}$`)

// ErrInvalidID is returned when an id does not match the adopted pattern.
var ErrInvalidID = errors.New("invalid entity id")

// ErrNotFound is returned when the requested entity does not exist.
var ErrNotFound = errors.New("entity not found")

// ValidateID checks an entity id against the adopted pattern.
func ValidateID(id string) error {
	if !idPattern.MatchString(id) {
		return ErrInvalidID
	}
	return nil
}

// SpecFor returns the spec of a node type.
func SpecFor(t NodeType) (NodeSpec, bool) {
	i := slices.IndexFunc(NodeSpecs, func(s NodeSpec) bool { return s.Type == t })
	if i < 0 {
		return NodeSpec{}, false
	}
	return NodeSpecs[i], true
}

// RelSpecFor returns the spec of a relationship type.
func RelSpecFor(t RelType) (RelSpec, bool) {
	i := slices.IndexFunc(RelSpecs, func(s RelSpec) bool { return s.Type == t })
	if i < 0 {
		return RelSpec{}, false
	}
	return RelSpecs[i], true
}

// TypeForID derives the node type from the id prefix.
func TypeForID(id string) (NodeType, bool) {
	if ValidateID(id) != nil {
		return "", false
	}
	prefix := id[:3]
	for _, s := range NodeSpecs {
		if s.IDPrefix == prefix {
			return s.Type, true
		}
	}
	return "", false
}
