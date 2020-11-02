// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package query

import (
	"fmt"
	"strings"

	"github.com/juju/errors"
)

// Builder allows the building up of a query when calling the Build method
type Builder interface {
	// Build takes a given prefix and returns the newly created query or returns
	// an error.
	Build(string) (string, error)
}

// Builders is a factory node that allows building of builders in a much more
// fluid style.
type Builders []Builder

// LogicalAND constructs a LogicalANDBuilder thats sole purpose is to add
// logical && in between queries.
func (q Builders) LogicalAND() Builder {
	return LogicalANDBuilder{
		Queries: q,
	}
}

// ForEach constructs a ForEachBuilder which allows the construction of a
// forEach loop over a given collection.
func (q Builders) ForEach(collection, prefix string, fn func() (Builder, error)) Builder {
	return ForEachBuilder{
		Collection: collection,
		Prefix:     prefix,
		Func:       fn,
	}
}

// ForEachBuilder creates a forEach loop with a given prefix over a collection
// of items.
type ForEachBuilder struct {
	Collection string
	Prefix     string
	Func       func() (Builder, error)
}

// Build takes a prefix and constructs a forEach loop in the following form.
//
//     forEach(<collection>, <prefix> => <prefix>.<func result>)
//
func (q ForEachBuilder) Build(prefix string) (string, error) {
	if prefix == "" {
		prefix = q.Prefix
	} else {
		prefix = fmt.Sprintf("%s.%s", prefix, q.Prefix)
	}
	expression, err := q.Func()
	if err != nil {
		return "", errors.Trace(err)
	}
	result, err := expression.Build(prefix)
	if err != nil {
		return "", errors.Trace(err)
	}
	return fmt.Sprintf("forEach(%s, %s => %s)", q.Collection, prefix, result), nil
}

// LogicalANDBuilder collapses a slice of queries and makes them a query
// seperated by logical &&.
type LogicalANDBuilder struct {
	Queries []Builder
}

// Build takes a prefix and constructs a query with logical &&.
func (q LogicalANDBuilder) Build(prefix string) (string, error) {
	var results []string
	for _, query := range q.Queries {
		r, err := query.Build(prefix)
		if err != nil {
			return "", errors.Trace(err)
		}
		results = append(results, r)
	}
	return strings.Join(results, " && "), nil
}

// OperatorNode takes a name and value and constructs a operator query.
type OperatorNode struct {
	Name, Value string
	Operator    string
}

// Equality provides a helper function for creating a operator builder.
func Equality(name, value string) Builder {
	return OperatorNode{
		Name:     name,
		Value:    value,
		Operator: "==",
	}
}

// Inequality provides a helper function for creating a operator builder.
func Inequality(name, value string) Builder {
	return OperatorNode{
		Name:     name,
		Value:    value,
		Operator: "!=",
	}
}

// Build takes a prefix and constructs a operator query.
func (q OperatorNode) Build(prefix string) (string, error) {
	query := fmt.Sprintf("%s %s %q", q.Name, q.Operator, q.Value)
	if prefix != "" {
		return fmt.Sprintf("%s.%s", prefix, query), nil
	}
	return query, nil
}
