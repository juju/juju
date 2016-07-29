// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package elasticsearch // import "gopkg.in/juju/charmstore.v5-unstable/elasticsearch"

import (
	"encoding/json"
	"fmt"
)

// Query DSL - Queries

// Query represents a query in the elasticsearch DSL.
type Query interface {
	json.Marshaler
}

// Filter represents a filter in the elasticsearch DSL.
type Filter interface {
	json.Marshaler
}

// Function is a function definition for use with a FunctionScoreQuery.
type Function interface{}

// BoostField creates a string which represents a field name with a boost value.
func BoostField(field string, boost float64) string {
	return fmt.Sprintf("%s^%f", field, boost)
}

// MatchAllQuery provides a query that matches all
// documents in the index.
type MatchAllQuery struct {
}

func (m MatchAllQuery) MarshalJSON() ([]byte, error) {
	return marshalNamedObject("match_all", struct{}{})
}

// MatchQuery provides a query that matches against
// a complete field.
type MatchQuery struct {
	Field string
	Query string
	Type  string
}

func (m MatchQuery) MarshalJSON() ([]byte, error) {
	params := map[string]interface{}{"query": m.Query}
	if m.Type != "" {
		params["type"] = m.Type
	}

	return marshalNamedObject("match", map[string]interface{}{m.Field: params})
}

// MultiMatchQuery provides a query that matches on a number of fields.
type MultiMatchQuery struct {
	Query  string
	Fields []string
}

func (m MultiMatchQuery) MarshalJSON() ([]byte, error) {
	return marshalNamedObject("multi_match", map[string]interface{}{
		"query":  m.Query,
		"fields": m.Fields,
	})
}

// FilteredQuery provides a query that includes a filter.
type FilteredQuery struct {
	Query  Query
	Filter Filter
}

func (f FilteredQuery) MarshalJSON() ([]byte, error) {
	return marshalNamedObject("filtered", map[string]interface{}{
		"query":  f.Query,
		"filter": f.Filter,
	})
}

// FunctionScoreQuery provides a query that adjusts the scoring of a
// query by applying functions to it.
type FunctionScoreQuery struct {
	Query     Query
	Functions []Function
}

func (f FunctionScoreQuery) MarshalJSON() ([]byte, error) {
	return marshalNamedObject("function_score", map[string]interface{}{
		"query":     f.Query,
		"functions": f.Functions,
	})
}

// TermQuery provides a query that matches a term in a field.
type TermQuery struct {
	Field string
	Value string
}

func (t TermQuery) MarshalJSON() ([]byte, error) {
	return marshalNamedObject("term", map[string]interface{}{
		t.Field: t.Value,
	})
}

// DecayFunction provides a function that boosts depending on
// the difference in values of a certain field. See
// http://www.elasticsearch.org/guide/en/elasticsearch/reference/current/query-dsl-function-score-query.html#_decay_functions
// for details.
type DecayFunction struct {
	Function string
	Field    string
	Scale    string
}

func (f DecayFunction) MarshalJSON() ([]byte, error) {
	return marshalNamedObject(f.Function, map[string]interface{}{
		f.Field: map[string]interface{}{
			"scale": f.Scale,
		},
	})
}

// BoostFactorFunction provides a function that boosts results by the specified amount.
type BoostFactorFunction struct {
	Filter      Filter  `json:"filter,omitempty"`
	BoostFactor float64 `json:"boost_factor"`
}

// FieldValueFactorFunction boosts the results by the value of a field in the document.
type FieldValueFactorFunction struct {
	Field    string  `json:"field"`
	Factor   float64 `json:"factor,omitempty"`
	Modifier string  `json:"modifier,omitempty"`
}

func (f FieldValueFactorFunction) MarshalJSON() ([]byte, error) {
	type ffvf FieldValueFactorFunction
	return marshalNamedObject("field_value_factor", ffvf(f))
}

// AndFilter provides a filter that matches if all of the internal
// filters match.
type AndFilter []Filter

func (a AndFilter) MarshalJSON() ([]byte, error) {
	return marshalNamedObject("and", map[string]interface{}{
		"filters": []Filter(a),
	})
}

// OrFilter provides a filter that matches if any of the internal
// filters match.
type OrFilter []Filter

func (o OrFilter) MarshalJSON() ([]byte, error) {
	return marshalNamedObject("or", map[string]interface{}{
		"filters": []Filter(o),
	})
}

// NotFilter provides a filter that matches the opposite of the
// wrapped filter.
type NotFilter struct {
	Filter Filter
}

func (n NotFilter) MarshalJSON() ([]byte, error) {
	return marshalNamedObject("not", n.Filter)
}

// QueryFilter provides a filter that matches when a query matches
// on a result
type QueryFilter struct {
	Query Query
}

func (q QueryFilter) MarshalJSON() ([]byte, error) {
	return marshalNamedObject("query", q.Query)
}

// RegexpFilter provides a filter that matches a field against a
// regular expression.
type RegexpFilter struct {
	Field  string
	Regexp string
}

func (r RegexpFilter) MarshalJSON() ([]byte, error) {
	return marshalNamedObject("regexp", map[string]string{r.Field: r.Regexp})
}

// TermFilter provides a filter that requires a field to match.
type TermFilter struct {
	Field string
	Value string
}

func (t TermFilter) MarshalJSON() ([]byte, error) {
	return marshalNamedObject("term", map[string]string{t.Field: t.Value})
}

// ExistsFilter provides a filter that requres a field to be present.
type ExistsFilter string

func (f ExistsFilter) MarshalJSON() ([]byte, error) {
	return marshalNamedObject("exists", map[string]string{"field": string(f)})
}

// QueryDSL provides a structure to put together a query using the
// elasticsearch DSL.
type QueryDSL struct {
	Fields []string `json:"fields"`
	From   int      `json:"from,omitempty"`
	Size   int      `json:"size,omitempty"`
	Query  Query    `json:"query,omitempty"`
	Sort   []Sort   `json:"sort,omitempty"`
}

type Sort struct {
	Field string
	Order Order
}

type Order struct {
	Order string `json:"order"`
}

func (s Sort) MarshalJSON() ([]byte, error) {
	return json.Marshal(map[string]Order{
		s.Field: {s.Order.Order},
	})
}

// Ascending is an Order that orders a sort by ascending through the values.
var Ascending = Order{"asc"}

// Descending is an Order that orders a sort by descending throuth the values.
var Descending = Order{"desc"}

// marshalNamedObject provides a helper that creates json objects in a form
// often required by the elasticsearch query DSL. The objects created
// take the following form:
//	{
//		name: obj
//	}
func marshalNamedObject(name string, obj interface{}) ([]byte, error) {
	return json.Marshal(map[string]interface{}{name: obj})
}
