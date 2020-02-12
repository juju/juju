// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"github.com/juju/errors"
	jujutxn "github.com/juju/txn"
	"gopkg.in/mgo.v2"
	"gopkg.in/mgo.v2/bson"
	"gopkg.in/mgo.v2/txn"

	mgoutils "github.com/juju/juju/mongo/utils"
)

type unitSetStateOperation struct {
	u        *Unit
	newState map[string]string
}

// Build implements ModelOperation.
func (op *unitSetStateOperation) Build(attempt int) ([]txn.Op, error) {
	annotateErr := func(err error) error {
		return errors.Annotatef(err, "cannot persist state for unit %q", op.u)
	}

	if attempt > 0 {
		if err := op.u.Refresh(); err != nil {
			return nil, annotateErr(errors.Trace(err))
		}
	}

	if op.u.Life() != Alive {
		return nil, annotateErr(errors.NotFoundf("unit %s", op.u.Name()))
	}

	coll, closer := op.u.st.db().GetCollection(unitStatesC)
	defer closer()

	// The state of a unit can only be updated if it is currently alive.
	unitAliveOp := txn.Op{
		C:      unitsC,
		Id:     op.u.doc.DocID,
		Assert: isAliveDoc,
	}

	var stDoc unitStateDoc
	unitGlobalKey := op.u.globalKey()
	if err := coll.FindId(unitGlobalKey).One(&stDoc); err != nil {
		if err != mgo.ErrNotFound {
			return nil, errors.Trace(err)
		}

		escapedState := make(map[string]string, len(op.newState))
		for k, v := range op.newState {
			escapedState[mgoutils.EscapeKey(k)] = v
		}

		return []txn.Op{unitAliveOp, {
			C:      unitStatesC,
			Id:     unitGlobalKey,
			Assert: txn.DocMissing,
			Insert: unitStateDoc{
				DocID: unitGlobalKey,
				State: escapedState,
			},
		}}, nil
	}

	// State keys may contain dots or dollar chars which need to be escaped.
	escapedState := make(bson.M, len(op.newState))
	for k, v := range op.newState {
		escapedState[mgoutils.EscapeKey(k)] = v
	}

	// Check if we need to update
	if stDoc.stateMatches(escapedState) {
		return nil, jujutxn.ErrNoOperations
	}

	return []txn.Op{unitAliveOp, {
		C:  unitStatesC,
		Id: unitGlobalKey,
		Assert: bson.D{
			{"txn-revno", stDoc.TxnRevno},
		},
		Update: bson.D{{"$set", bson.D{{"state", escapedState}}}},
	}}, nil
}

// Done implements ModelOperation.
func (op *unitSetStateOperation) Done(err error) error { return err }
