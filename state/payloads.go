// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"github.com/juju/errors"
	"github.com/juju/mgo/v2/bson"
	"github.com/juju/mgo/v2/txn"

	"github.com/juju/juju/payload"
)

// ModelPayloads returns a ModelPayloads for the state's model.
func (st *State) ModelPayloads() (ModelPayloads, error) {
	return ModelPayloads{
		db: st.database,
	}, nil
}

// ModelPayloads lets you read all unit payloads in a model.
type ModelPayloads struct {
	db Database
}

// ListAll builds the list of payload information that is registered in state.
func (mp ModelPayloads) ListAll() ([]payload.FullPayloadInfo, error) {
	coll, closer := mp.db.GetCollection(payloadsC)
	defer closer()

	var docs []payloadDoc
	if err := coll.Find(nil).All(&docs); err != nil {
		return nil, errors.Trace(err)
	}
	return nsPayloads.asPayloads(docs), nil
}

// UnitPayloads returns a UnitPayloads for the supplied unit.
func (st *State) UnitPayloads(unit *Unit) (UnitPayloads, error) {
	machineID, err := unit.AssignedMachineId()
	if err != nil {
		return UnitPayloads{}, errors.Trace(err)
	}
	return UnitPayloads{
		db:      st.database,
		unit:    unit.Name(),
		machine: machineID,
	}, nil
}

// UnitPayloads lets you CRUD payloads for a single unit.
type UnitPayloads struct {
	db      Database
	unit    string
	machine string
}

// List has two different modes of operation, because that's never a bad
// idea. If you pass no args, it returns information about all payloads
// tracked by the unit; if you pass names, it returns a slice of results
// corresponding to names, in which any names not tracked have both the
// NotFound field *and* an Error set.
func (up UnitPayloads) List(names ...string) ([]payload.Result, error) {

	var sel bson.D
	var out func([]payloadDoc) []payload.Result
	if len(names) == 0 {
		sel = nsPayloads.forUnit(up.unit)
		out = nsPayloads.asResults
	} else {
		sel = nsPayloads.forUnitWithNames(up.unit, names)
		out = func(docs []payloadDoc) []payload.Result {
			return nsPayloads.orderedResults(docs, names)
		}
	}

	coll, closer := up.db.GetCollection(payloadsC)
	defer closer()
	var docs []payloadDoc
	if err := coll.Find(sel).All(&docs); err != nil {
		return nil, errors.Trace(err)
	}
	return out(docs), nil
}

// LookUp returns its first argument and no error.
func (UnitPayloads) LookUp(name, rawID string) (string, error) {
	// This method *is* used in the apiserver layer, both to extract
	// the name from the payload and to implement the LookUp facade
	// method which allows clients to ask the server what the first
	// of two strings might be.
	//
	// The previous implementation would hit the db to as well, to
	// exactly the same effect as implemented here. Would drop the
	// whole useless slice, but don't want to bloat the diff.
	return name, nil
}

// Track inserts the provided payload info in state. If the payload
// is already in the DB then it is replaced.
func (up UnitPayloads) Track(pl payload.Payload) error {

	// XXX OMFG payload/context/register.go:83 launches bad data
	// which flies on a majestic unvalidated arc right through the
	// system until it lands here. This code should be:
	//
	//    if pl.Unit != up.unit {
	//        return errors.NotValidf("unexpected Unit %q", pl.Unit)
	//    }
	//
	// ...but is instead:
	pl.Unit = up.unit

	if err := pl.Validate(); err != nil {
		return errors.Trace(err)
	}

	doc := nsPayloads.asDoc(payload.FullPayloadInfo{
		Payload: pl,
		Machine: up.machine,
	})
	change := payloadTrackChange{doc}
	if err := Apply(up.db, change); err != nil {
		return errors.Trace(err)
	}
	return nil
}

// SetStatus updates the raw status for the identified payload to the
// provided value. If the payload is missing then payload.ErrNotFound
// is returned.
func (up UnitPayloads) SetStatus(name, status string) error {
	if err := payload.ValidateState(status); err != nil {
		return errors.Trace(err)
	}

	change := payloadSetStatusChange{
		Unit:   up.unit,
		Name:   name,
		Status: status,
	}
	if err := Apply(up.db, change); err != nil {
		return errors.Trace(err)
	}
	return nil
}

// Untrack removes the identified payload from state. It does not
// trigger the actual destruction of the payload. If the payload is
// missing then this is a noop.
func (up UnitPayloads) Untrack(name string) error {
	logger.Tracef("untracking %q", name)
	change := payloadUntrackChange{
		Unit: up.unit,
		Name: name,
	}
	if err := Apply(up.db, change); err != nil {
		return errors.Trace(err)
	}
	return nil
}

// payloadTrackChange records a single unit payload.
type payloadTrackChange struct {
	Doc payloadDoc
}

// Prepare is part of the Change interface.
func (change payloadTrackChange) Prepare(db Database) ([]txn.Op, error) {

	unit := change.Doc.UnitID
	units, closer := db.GetCollection(unitsC)
	defer closer()
	unitOp, err := nsLife.notDeadOp(units, unit)
	if errors.Cause(err) == errDeadOrGone {
		return nil, errors.Errorf("unit %q no longer available", unit)
	} else if err != nil {
		return nil, errors.Trace(err)
	}

	payloads, closer := db.GetCollection(payloadsC)
	defer closer()
	payloadOp, err := nsPayloads.trackOp(payloads, change.Doc)
	if err != nil {
		return nil, errors.Trace(err)
	}

	return []txn.Op{unitOp, payloadOp}, nil
}

// payloadSetStatusChange updates a single payload status.
type payloadSetStatusChange struct {
	Unit   string
	Name   string
	Status string
}

// Prepare is part of the Change interface.
func (change payloadSetStatusChange) Prepare(db Database) ([]txn.Op, error) {
	docID := nsPayloads.docID(change.Unit, change.Name)
	payloads, closer := db.GetCollection(payloadsC)
	defer closer()

	op, err := nsPayloads.setStatusOp(payloads, docID, change.Status)
	if errors.Cause(err) == errAlreadyRemoved {
		return nil, payload.ErrNotFound
	} else if err != nil {
		return nil, errors.Trace(err)
	}
	return []txn.Op{op}, nil
}

// payloadUntrackChange removes a single unit payload.
type payloadUntrackChange struct {
	Unit string
	Name string
}

// Prepare is part of the Change interface.
func (change payloadUntrackChange) Prepare(db Database) ([]txn.Op, error) {
	docID := nsPayloads.docID(change.Unit, change.Name)
	payloads, closer := db.GetCollection(payloadsC)
	defer closer()

	op, err := nsPayloads.untrackOp(payloads, docID)
	if errors.Cause(err) == errAlreadyRemoved {
		return nil, ErrChangeComplete
	} else if err != nil {
		return nil, errors.Trace(err)
	}
	return []txn.Op{op}, nil
}

// payloadCleanupChange removes all unit payloads.
type payloadCleanupChange struct {
	Unit string
}

// Prepare is part of the Change interface.
func (change payloadCleanupChange) Prepare(db Database) ([]txn.Op, error) {
	payloads, closer := db.GetCollection(payloadsC)
	defer closer()

	sel := nsPayloads.forUnit(change.Unit)
	fields := bson.D{{"_id", 1}}
	var docs []struct {
		DocID string `bson:"_id"`
	}
	err := payloads.Find(sel).Select(fields).All(&docs)
	if err != nil {
		return nil, errors.Trace(err)
	} else if len(docs) == 0 {
		return nil, ErrChangeComplete
	}

	ops := make([]txn.Op, 0, len(docs))
	for _, doc := range docs {
		op, err := nsPayloads.untrackOp(payloads, doc.DocID)
		if errors.Cause(err) == errAlreadyRemoved {
			continue
		} else if err != nil {
			return nil, errors.Trace(err)
		}
		ops = append(ops, op)
	}
	return ops, nil
}
