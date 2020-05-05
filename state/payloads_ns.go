// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"fmt"

	"github.com/juju/charm/v7"
	"github.com/juju/errors"
	"gopkg.in/mgo.v2/bson"
	"gopkg.in/mgo.v2/txn"

	"github.com/juju/juju/mongo"
	"github.com/juju/juju/payload"
)

// payloadDoc is the top-level document for payloads.
type payloadDoc struct {
	// _id encodes UnitID and Name (which should theoretically
	// match the name of a payload-class defined in the charm --
	// for example "my-payload" -- but nothing really checks).
	UnitID string `bson:"unitid"`
	Name   string `bson:"name"`

	// MachineID doesn't belong here.
	MachineID string `bson:"machine-id"`

	// Type is again a freeform field that might match that of a
	// payload-class defined in the charm -- for example, "docker".
	Type string `bson:"type"`

	// RawID records the substrate-specific payload id -- for
	// example, "9cd6338abdf09beb", the actual docker container
	// we're tracking.
	RawID string `bson:"rawid"`

	// State is sort of like status, valid values are defined in
	// package payloads.
	State string `bson:"state"`

	// Labels contain whatever additional arbitrary strings were
	// left over after we hoovered up <Type> <Name> <RawID> from the
	// command line.
	Labels []string `bson:"labels"`
}

// nsPayloads_ backs nsPayloads.
type nsPayloads_ struct{}

// nsPayloads namespaces low-level unit-payload functionality: it's
// meant to be the one place in the code where we wrangle queries,
// serialization, and updates to payload data. (The UnitPayloads and
// ModelPayloads types may run queries directly, because it's silly
// to build *another* mongo-aping layer with its own idiosyncratic
// implementations of One and All and so on; but they should be getting
// all their queries from here, and using these methods to convert
// types, and generally making a point of *not* knowing anything about
// how the actual data is represented.)
var nsPayloads = nsPayloads_{}

// docID is globalKey as written by someone who thought it would be
// helpful to reinvent the 'u#<unit>#' prefix (which *would* indicate
// that payloads are things-that-exist-per-unit, and do so in a way
// consistent with the rest of the DB. /sigh.)
func (nsPayloads_) docID(unit, name string) string {
	return fmt.Sprintf("payload#%s#%s", unit, name)
}

// forUnit returns a selector that matches all payloads for the unit.
func (nsPayloads_) forUnit(unit string) bson.D {
	return bson.D{{"unitid", unit}}
}

// forUnitWithNames returns a selector that matches any payloads for the
// supplied unit that have the supplied names.
func (nsPayloads_) forUnitWithNames(unit string, names []string) bson.D {
	ids := make([]string, 0, len(names))
	for _, name := range names {
		ids = append(ids, nsPayloads.docID(unit, name))
	}
	return bson.D{{"_id", bson.D{{"$in", ids}}}}
}

// asDoc converts a FullPayloadInfo into an independent payloadDoc.
func (nsPayloads_) asDoc(p payload.FullPayloadInfo) payloadDoc {
	labels := make([]string, len(p.Labels))
	copy(labels, p.Labels)
	return payloadDoc{
		UnitID:    p.Unit,
		Name:      p.PayloadClass.Name,
		MachineID: p.Machine,
		Type:      p.PayloadClass.Type,
		RawID:     p.ID,
		State:     p.Status,
		Labels:    labels,
	}
}

// asPayload converts a payloadDoc into an independent FullPayloadInfo.
func (nsPayloads_) asPayload(doc payloadDoc) payload.FullPayloadInfo {
	labels := make([]string, len(doc.Labels))
	copy(labels, doc.Labels)
	p := payload.FullPayloadInfo{
		Payload: payload.Payload{
			PayloadClass: charm.PayloadClass{
				Name: doc.Name,
				Type: doc.Type,
			},
			ID:     doc.RawID,
			Status: doc.State,
			Labels: labels,
			Unit:   doc.UnitID,
		},
		Machine: doc.MachineID,
	}
	return p
}

// asPayloads converts a slice of payloadDocs into a corresponding slice
// of independent FullPayloadInfos.
func (nsPayloads_) asPayloads(docs []payloadDoc) []payload.FullPayloadInfo {
	payloads := make([]payload.FullPayloadInfo, 0, len(docs))
	for _, doc := range docs {
		payloads = append(payloads, nsPayloads.asPayload(doc))
	}
	return payloads
}

// asResults converts a slice of payloadDocs into a corresponding slice
// of independent payload.Results.
func (nsPayloads_) asResults(docs []payloadDoc) []payload.Result {
	results := make([]payload.Result, 0, len(docs))
	for _, doc := range docs {
		full := nsPayloads.asPayload(doc)
		results = append(results, payload.Result{
			ID:      doc.Name,
			Payload: &full,
		})
	}
	return results
}

// orderedResults converts payloadDocs into payload.Results, in the
// order defined by names, and represents missing names in the highly
// baroque fashion apparently designed into Results.
func (nsPayloads_) orderedResults(docs []payloadDoc, names []string) []payload.Result {
	found := make(map[string]payloadDoc)
	for _, doc := range docs {
		found[doc.Name] = doc
	}
	results := make([]payload.Result, len(names))
	for i, name := range names {
		results[i].ID = name
		if doc, ok := found[name]; ok {
			full := nsPayloads.asPayload(doc)
			results[i].Payload = &full
		} else {
			results[i].NotFound = true
			results[i].Error = errors.NotFoundf(name)
		}
	}
	return results
}

// trackOp returns a txn.Op that will either insert or update the
// supplied payload, and fail if the observed precondition changes.
func (nsPayloads_) trackOp(payloads mongo.Collection, doc payloadDoc) (txn.Op, error) {
	docID := nsPayloads.docID(doc.UnitID, doc.Name)
	payloadOp := txn.Op{
		C:  payloads.Name(),
		Id: docID,
	}
	count, err := payloads.FindId(docID).Count()
	if err != nil {
		return txn.Op{}, errors.Trace(err)
	} else if count == 0 {
		payloadOp.Assert = txn.DocMissing
		payloadOp.Insert = doc
	} else {
		payloadOp.Assert = txn.DocExists
		payloadOp.Update = bson.D{{"$set", doc}}
	}
	return payloadOp, nil
}

// untrackOp returns a txn.Op that will unconditionally remove the
// identified document. If the payload doesn't exist, it returns
// errAlreadyRemoved.
func (nsPayloads_) untrackOp(payloads mongo.Collection, docID string) (txn.Op, error) {
	count, err := payloads.FindId(docID).Count()
	if err != nil {
		return txn.Op{}, errors.Trace(err)
	} else if count == 0 {
		return txn.Op{}, errAlreadyRemoved
	}
	return txn.Op{
		C:      payloads.Name(),
		Id:     docID,
		Assert: txn.DocExists,
		Remove: true,
	}, nil
}

// setStatusOp returns a txn.Op that updates the status of the
// identified payload. If the payload doesn't exist, it returns
// errAlreadyRemoved.
func (nsPayloads_) setStatusOp(payloads mongo.Collection, docID string, status string) (txn.Op, error) {
	count, err := payloads.FindId(docID).Count()
	if err != nil {
		return txn.Op{}, errors.Trace(err)
	} else if count == 0 {
		return txn.Op{}, errAlreadyRemoved
	}
	return txn.Op{
		C:      payloads.Name(),
		Id:     docID,
		Assert: txn.DocExists,
		Update: bson.D{{"$set", bson.D{{"state", status}}}},
	}, nil
}
