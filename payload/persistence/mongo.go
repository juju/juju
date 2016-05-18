// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package persistence

import (
	"fmt"

	"github.com/juju/errors"
	"gopkg.in/juju/charm.v6-unstable"
	"gopkg.in/mgo.v2/bson"
	"gopkg.in/mgo.v2/txn"

	"github.com/juju/juju/payload"
)

const (
	payloadsC = "payloads"
)

// TODO(ericsnow) Move the methods under their own type (payloadcollection?).

func (pp Persistence) extractPayload(id string, payloadDocs map[string]payloadDoc) (*payload.FullPayloadInfo, bool) {
	doc, ok := payloadDocs[id]
	if !ok {
		return nil, false
	}
	p := doc.payload()
	p.Unit = pp.unit
	return &p, true
}

func (pp Persistence) all(query bson.D, docs interface{}) error {
	return errors.Trace(pp.db.All(payloadsC, query, docs))
}

func (pp Persistence) allID(query bson.D, docs interface{}) error {
	if query != nil {
		query = bson.D{{"_id", query}}
	}
	return errors.Trace(pp.all(query, docs))
}

func (pp Persistence) payloadID(name string) string {
	return payloadID(pp.unit, name)
}

func payloadID(unit, name string) string {
	return fmt.Sprintf("payload#%s#%s", unit, name)
}

func (pp Persistence) newInsertPayloadOps(id string, p payload.FullPayloadInfo) []txn.Op {
	// We must also ensure that there isn't any collision on the
	// state-provided ID. However, that isn't something we can do in
	// a transaction.
	doc := pp.newPayloadDoc(id, p)
	return []txn.Op{{
		C:      payloadsC,
		Id:     doc.DocID,
		Assert: txn.DocMissing,
		Insert: doc,
	}}
}

func (pp Persistence) newSetRawStatusOps(name, stID, status string) []txn.Op {
	id := pp.payloadID(name)
	updates := bson.D{
		{"state", status},
	}
	return []txn.Op{{
		C:      payloadsC,
		Id:     id,
		Assert: txn.DocExists,
		Update: bson.D{{"$set", updates}},
	}, {
		C:      payloadsC,
		Id:     id,
		Assert: bson.D{{"state-id", stID}},
	}}
}

func (pp Persistence) newRemovePayloadOps(name, stID string) []txn.Op {
	id := pp.payloadID(name)
	return []txn.Op{{
		C:      payloadsC,
		Id:     id,
		Assert: txn.DocExists,
		Remove: true,
	}, {
		C:      payloadsC,
		Id:     id,
		Assert: bson.D{{"state-id", stID}},
	}}
}

// payloadDoc is the top-level document for payloads.
type payloadDoc struct {
	DocID string `bson:"_id"`

	// UnitID and Name are encoded in DocID.
	UnitID string `bson:"unitid"`
	Name   string `bson:"name"`

	MachineID string `bson:"machine-id"`

	// StateID is the unique ID that State gave this payload for this unit.
	StateID string `bson:"state-id"`

	Type string `bson:"type"`

	// TODO(ericsnow) Store status in the "statuses" collection?

	State string `bson:"state"`

	// TODO(ericsnow) Store labels in the "annotations" collection?

	Labels []string `bson:"labels"`

	RawID string `bson:"rawid"`
}

func (d payloadDoc) payload() payload.FullPayloadInfo {
	labels := make([]string, len(d.Labels))
	copy(labels, d.Labels)
	p := payload.FullPayloadInfo{
		Payload: payload.Payload{
			PayloadClass: d.definition(),
			ID:           d.RawID,
			Status:       d.State,
			Labels:       labels,
			Unit:         d.UnitID,
		},
		Machine: d.MachineID,
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

func (pp Persistence) newPayloadDoc(stID string, p payload.FullPayloadInfo) *payloadDoc {
	p.Unit = pp.unit
	return newPayloadDoc(stID, p)
}

func newPayloadDoc(stID string, p payload.FullPayloadInfo) *payloadDoc {
	id := payloadID(p.Unit, p.Name)

	definition := p.PayloadClass

	labels := make([]string, len(p.Labels))
	copy(labels, p.Labels)

	return &payloadDoc{
		DocID:  id,
		UnitID: p.Unit,
		Name:   definition.Name,

		MachineID: p.Machine,

		StateID: stID,

		Type: definition.Type,

		State: p.Status,

		Labels: labels,

		RawID: p.ID,
	}
}

func (pp Persistence) allModelPayloads() ([]payloadDoc, error) {
	var docs []payloadDoc
	if err := pp.all(nil, &docs); err != nil {
		return nil, errors.Trace(err)
	}
	return docs, nil
}

func (pp Persistence) allPayloads() (map[string]payloadDoc, error) {
	var docs []payloadDoc
	query := bson.D{{"unitid", pp.unit}}
	if err := pp.all(query, &docs); err != nil {
		return nil, errors.Trace(err)
	}

	results := make(map[string]payloadDoc)
	for _, doc := range docs {
		id := doc.StateID
		results[id] = doc
	}
	return results, nil
}

func (pp Persistence) payloads(ids []string) (map[string]payloadDoc, []string, error) {
	all, err := pp.allPayloads()
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
