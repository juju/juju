// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package persistence

import (
	"fmt"

	"gopkg.in/juju/charm.v6-unstable"
	"gopkg.in/mgo.v2/bson"

	"github.com/juju/juju/payload"
)

const (
	payloadsC = "payloads"
)

func payloadID(unit, name string) string {
	return fmt.Sprintf("payload#%s#%s", unit, name)
}

func payloadIDQuery(unit, name string) bson.D {
	id := payloadID(unit, name)
	return bson.D{{"_id", id}}
}

// payloadDoc is the top-level document for payloads.
type payloadDoc struct {
	DocID string `bson:"_id"`

	// UnitID and Name are encoded in DocID.
	UnitID string `bson:"unitid"`
	Name   string `bson:"name"`

	MachineID string `bson:"machine-id"`

	Type string `bson:"type"`

	// TODO(ericsnow) Store status in the "statuses" collection?

	State string `bson:"state"`

	// TODO(ericsnow) Store labels in the "annotations" collection?

	Labels []string `bson:"labels"`

	RawID string `bson:"rawid"`
}

func newPayloadDoc(p payload.FullPayloadInfo) *payloadDoc {
	id := payloadID(p.Unit, p.Name)

	definition := p.PayloadClass

	labels := make([]string, len(p.Labels))
	copy(labels, p.Labels)

	return &payloadDoc{
		DocID:  id,
		UnitID: p.Unit,
		Name:   definition.Name,

		MachineID: p.Machine,

		Type: definition.Type,

		State: p.Status,

		Labels: labels,

		RawID: p.ID,
	}
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
	return charm.PayloadClass{
		Name: d.Name,
		Type: d.Type,
	}
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
