// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package persistence

import (
	"fmt"
	"strings"

	"github.com/juju/errors"
	"github.com/juju/names"
	"gopkg.in/juju/charm.v6-unstable"
	"gopkg.in/mgo.v2/bson"
	"gopkg.in/mgo.v2/txn"

	"github.com/juju/juju/payload"
)

const (
	payloadsC = "payloads"
)

// Collections is the list of names of the mongo collections where state
// is stored for payloads.
// TODO(ericsnow) Not needed anymore...modify for a new registration scheme?
var Collections = []string{
	payloadsC,
}

// TODO(ericsnow) Move the methods under their own type (payloadcollection?).

func (pp Persistence) extractPayload(id string, payloadDocs map[string]payloadDoc) (*payload.Payload, bool) {
	doc, ok := payloadDocs[id]
	if !ok {
		return nil, false
	}
	p := doc.payload(pp.unit)
	return &p, true
}

func (pp Persistence) one(id string, doc interface{}) error {
	return errors.Trace(pp.st.One(payloadsC, id, doc))
}

func (pp Persistence) all(query bson.D, docs interface{}) error {
	return errors.Trace(pp.st.All(payloadsC, query, docs))
}

func (pp Persistence) allID(query bson.D, docs interface{}) error {
	if query != nil {
		query = bson.D{{"_id", query}}
	}
	return errors.Trace(pp.all(query, docs))
}

func (pp Persistence) payloadID(id string) string {
	// TODO(ericsnow) Drop the unit part.
	return fmt.Sprintf("payload#%s#%s", pp.unit, id)
}

func (pp Persistence) extractPayloadID(docID string) string {
	parts := strings.Split(docID, "#")
	return parts[len(parts)-1]
}

func (pp Persistence) newInsertPayloadOps(id string, p payload.Payload) []txn.Op {
	var ops []txn.Op

	doc := pp.newPayloadDoc(id, p)
	ops = append(ops, txn.Op{
		C:      payloadsC,
		Id:     doc.DocID,
		Assert: txn.DocMissing,
		Insert: doc,
	})

	return ops
}

func (pp Persistence) newSetRawStatusOps(id, status string) []txn.Op {
	id = pp.payloadID(id)
	updates := bson.D{
		{"state", status},
	}
	return []txn.Op{{
		C:      payloadsC,
		Id:     id,
		Assert: txn.DocExists,
		Update: bson.D{{"$set", updates}},
	}}
}

func (pp Persistence) newRemovePayloadOps(id string) []txn.Op {
	id = pp.payloadID(id)
	return []txn.Op{{
		C:      payloadsC,
		Id:     id,
		Assert: txn.DocExists,
		Remove: true,
	}}
}

// payloadDoc is the top-level document for payloads.
type payloadDoc struct {
	DocID     string `bson:"_id"`
	ModelUUID string `bson:"model-uuid"`

	UnitID string `bson:"unitid"`

	Name string `bson:"name"`
	Type string `bson:"type"`

	// TODO(ericsnow) Store status in the "statuses" collection?

	State string `bson:"state"`

	// TODO(ericsnow) Store labels in the "annotations" collection?

	Labels []string `bson:"labels"`

	RawID string `bson:"rawid"`
}

func (d payloadDoc) payload(unit string) payload.Payload {
	labels := make([]string, len(d.Labels))
	copy(labels, d.Labels)
	p := payload.Payload{
		PayloadClass: d.definition(),
		ID:           d.RawID,
		Status:       d.State,
		Labels:       labels,
		Unit:         unit,
	}
	return p
}

func (d payloadDoc) definition() charm.PayloadClass {
	definition := charm.PayloadClass{
		Name: d.Name,
		Type: d.Type,
	}
	return definition
}

func (d payloadDoc) match(name, rawID string) bool {
	if d.Name != name {
		return false
	}
	if d.RawID != rawID {
		return false
	}
	return true
}

func (pp Persistence) newPayloadDoc(id string, p payload.Payload) *payloadDoc {
	id = pp.payloadID(id)

	definition := p.PayloadClass

	labels := make([]string, len(p.Labels))
	copy(labels, p.Labels)

	return &payloadDoc{
		DocID:  id,
		UnitID: pp.unit,

		Name: definition.Name,
		Type: definition.Type,

		State: p.Status,

		Labels: labels,

		RawID: p.ID,
	}
}

func (pp Persistence) allPayloads() (map[string]payloadDoc, error) {
	var docs []payloadDoc
	query := bson.D{{"unitid", pp.unit}}
	if err := pp.all(query, &docs); err != nil {
		return nil, errors.Trace(err)
	}

	results := make(map[string]payloadDoc)
	for _, doc := range docs {
		id := pp.extractPayloadID(doc.DocID)
		results[id] = doc
	}
	return results, nil
}

func (pp Persistence) payloads(ids []string) (map[string]payloadDoc, error) {
	fullIDs := make([]string, len(ids))
	idMap := make(map[string]string, len(ids))
	for i, id := range ids {
		fullID := pp.payloadID(id)
		fullIDs[i] = fullID
		idMap[fullID] = id
	}

	var docs []payloadDoc
	query := bson.D{{"$in", fullIDs}}
	if err := pp.allID(query, &docs); err != nil {
		return nil, errors.Trace(err)
	}

	results := make(map[string]payloadDoc)
	for _, doc := range docs {
		fullID := dropModelUUID(doc.DocID)
		id := idMap[fullID]
		results[id] = doc
	}
	return results, nil
}

func dropModelUUID(id string) string {
	fullID := id
	parts := strings.SplitN(fullID, ":", 2)
	if len(parts) == 2 {
		if names.IsValidModel(parts[0]) {
			fullID = parts[1]
		}
	}
	return fullID
}
