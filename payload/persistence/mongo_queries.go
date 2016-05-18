// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package persistence

import (
	"github.com/juju/errors"
	"gopkg.in/mgo.v2/bson"
)

type payloadsDBQueryer interface {
	// All populates docs with the list of the documents corresponding
	// to the provided query.
	All(collName string, query, docs interface{}) error
}

type payloadsQueries struct {
	querier payloadsDBQueryer
}

func (pq payloadsQueries) one(query bson.D) (payloadDoc, error) {
	var docs []payloadDoc
	if err := pq.querier.All(payloadsC, query, &docs); err != nil {
		return payloadDoc{}, errors.Trace(err)
	}
	if len(docs) > 1 {
		return payloadDoc{}, errors.NewNotValid(nil, "query too broad, got more than one doc")
	}
	if len(docs) == 0 {
		return payloadDoc{}, errors.NotFoundf("")
	}
	return docs[0], nil
}

func (pq payloadsQueries) all(unit string) ([]payloadDoc, error) {
	var docs []payloadDoc
	var query bson.D
	if unit != "" {
		query = bson.D{{"unitid", unit}}
	}
	if err := pq.querier.All(payloadsC, query, &docs); err != nil {
		return nil, errors.Trace(err)
	}
	return docs, nil
}

func (pq payloadsQueries) allByName() (map[string]payloadDoc, error) {
	docs, err := pq.all("")
	if err != nil {
		return nil, errors.Trace(err)
	}
	results := make(map[string]payloadDoc, len(docs))
	for _, doc := range docs {
		results[doc.Name] = doc
	}
	return results, nil
}

func (pq payloadsQueries) unitPayloadsByName(unit string) (map[string]payloadDoc, error) {
	if unit == "" {
		return nil, errors.NewNotValid(nil, "missing unit ID")
	}
	docs, err := pq.all("")
	if err != nil {
		return nil, errors.Trace(err)
	}
	results := make(map[string]payloadDoc)
	for _, doc := range docs {
		results[doc.Name] = doc
	}
	return results, nil
}

func (pq payloadsQueries) someUnitPayloads(unit string, names []string) ([]payloadDoc, []string, error) {
	all, err := pq.unitPayloadsByName(unit)
	if err != nil {
		return nil, nil, errors.Trace(err)
	}

	var results []payloadDoc
	var missing []string
	for _, name := range names {
		if doc, ok := all[name]; ok {
			results = append(results, doc)
		} else {
			missing = append(missing, name)
		}
	}
	return results, missing, nil
}
