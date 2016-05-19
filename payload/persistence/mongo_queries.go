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

func (pq payloadsQueries) one(unit string, query bson.D) (payloadDoc, error) {
	if unit == "" {
		return payloadDoc{}, errors.NewNotValid(nil, "missing unit ID")
	}
	var docs []payloadDoc
	query = append(bson.D{{"unitid", unit}}, query...)
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

func (pq payloadsQueries) allByStateID(unit string) (map[string]payloadDoc, error) {
	docs, err := pq.all(unit)
	if err != nil {
		return nil, errors.Trace(err)
	}
	results := make(map[string]payloadDoc, len(docs))
	for _, doc := range docs {
		id := doc.StateID
		results[id] = doc
	}
	return results, nil
}

func (pq payloadsQueries) unitPayloads(unit string, ids []string) (map[string]payloadDoc, []string, error) {
	if unit == "" {
		return nil, nil, errors.NewNotValid(nil, "missing unit ID")
	}
	all, err := pq.allByStateID(unit)
	if err != nil {
		return nil, nil, errors.Trace(err)
	}

	results := make(map[string]payloadDoc)
	var missing []string
	for _, id := range ids {
		if doc, ok := all[id]; ok {
			results[id] = doc
		} else {
			missing = append(missing, id)
		}
	}
	return results, missing, nil
}

func (pq payloadsQueries) payloadByStateID(unit, stID string) (payloadDoc, error) {
	if stID == "" {
		return payloadDoc{}, errors.NotFoundf("")
	}
	doc, err := pq.one(unit, bson.D{{"state-id", stID}})
	if err != nil {
		return payloadDoc{}, errors.Trace(err)
	}
	return doc, nil
}

func (pq payloadsQueries) payloadByName(unit, name string) (payloadDoc, error) {
	if name == "" {
		return payloadDoc{}, errors.NotFoundf("")
	}
	doc, err := pq.one(unit, bson.D{{"name", name}})
	if err != nil {
		return payloadDoc{}, errors.Trace(err)
	}
	return doc, nil
}
