// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"github.com/juju/errors"
	jujutxn "github.com/juju/txn"
	"gopkg.in/mgo.v2"
	"gopkg.in/mgo.v2/bson"
	"gopkg.in/mgo.v2/txn"

	"github.com/juju/juju/core/quota"
	mgoutils "github.com/juju/juju/mongo/utils"
)

type unitSetStateOperation struct {
	u        *Unit
	newState *UnitState

	// Quota limits for updating the charm and uniter state data.
	limits UnitStateSizeLimits
}

// Build implements ModelOperation.
func (op *unitSetStateOperation) Build(attempt int) ([]txn.Op, error) {
	if op.newState == nil || !op.newState.Modified() {
		return nil, jujutxn.ErrNoOperations
	}
	return op.buildTxn(attempt)
}

func (op *unitSetStateOperation) buildTxn(attempt int) ([]txn.Op, error) {
	if attempt > 0 {
		if err := op.u.Refresh(); err != nil {
			return nil, errors.Annotatef(err, "cannot persist state for unit %q", op.u)
		}
	}

	// Normally this would be if Life() != Alive.  However the uniter
	// needs to write its state during the Dying period to complete
	// operations such as resigning leadership.
	if op.u.Life() == Dead {
		return nil, errors.Annotatef(errors.NotFoundf("unit %s", op.u.Name()), "cannot persist state for unit %q", op.u)
	}

	coll, closer := op.u.st.db().GetCollection(unitStatesC)
	defer closer()

	// The state of a unit can only be updated if it is currently alive.
	unitNotDeadOp := txn.Op{
		C:      unitsC,
		Id:     op.u.doc.DocID,
		Assert: notDeadDoc,
	}

	var stDoc unitStateDoc
	unitGlobalKey := op.u.globalKey()
	if err := coll.FindId(unitGlobalKey).One(&stDoc); err != nil {
		if err != mgo.ErrNotFound {
			return nil, errors.Annotatef(err, "cannot persist state for unit %q", op.u)
		}

		// Create new doc and enforce quota limits
		newDoc, err := op.newUnitStateDoc(unitGlobalKey)
		if err != nil {
			return nil, errors.Trace(err)
		}
		return []txn.Op{unitNotDeadOp, {
			C:      unitStatesC,
			Id:     unitGlobalKey,
			Assert: txn.DocMissing,
			Insert: newDoc,
		}}, nil
	}

	// We have an existing doc, see what changes need to be made.
	setFields, unsetFields, err := op.fields(stDoc)
	if err != nil {
		return nil, errors.Trace(err)
	} else if len(setFields) <= 0 && len(unsetFields) <= 0 {
		return nil, jujutxn.ErrNoOperations
	}

	updateFields := bson.D{}
	if len(setFields) > 0 {
		updateFields = append(updateFields, bson.DocElem{"$set", setFields})
	}
	if len(unsetFields) > 0 {
		updateFields = append(updateFields, bson.DocElem{"$unset", unsetFields})
	}
	return []txn.Op{unitNotDeadOp, {
		C:  unitStatesC,
		Id: unitGlobalKey,
		Assert: bson.D{
			{"txn-revno", stDoc.TxnRevno},
		},
		Update: updateFields,
	}}, nil
}

func (op *unitSetStateOperation) newUnitStateDoc(unitGlobalKey string) (unitStateDoc, error) {
	newStDoc := unitStateDoc{
		DocID: unitGlobalKey,
	}
	if chState, found := op.newState.CharmState(); found {
		escapedCharmState := make(map[string]string, len(chState))
		for k, v := range chState {
			escapedCharmState[mgoutils.EscapeKey(k)] = v
		}
		newStDoc.CharmState = escapedCharmState

		quotaChecker := op.getCharmStateQuotaChecker()
		quotaChecker.Check(newStDoc.CharmState)
		if err := quotaChecker.Outcome(); err != nil {
			return unitStateDoc{}, errors.Annotatef(err, "persisting charm state")
		}
	}

	quotaChecker := op.getUniterStateQuotaChecker()
	if rState, found := op.newState.relationStateBSONFriendly(); found {
		newStDoc.RelationState = rState
		quotaChecker.Check(rState)
	}
	if uniterState, found := op.newState.UniterState(); found {
		newStDoc.UniterState = uniterState
		quotaChecker.Check(uniterState)
	}
	if storState, found := op.newState.StorageState(); found {
		newStDoc.StorageState = storState
		quotaChecker.Check(storState)
	}
	if meterStatusState, found := op.newState.MeterStatusState(); found {
		newStDoc.MeterStatusState = meterStatusState
		quotaChecker.Check(meterStatusState)
	}
	if err := quotaChecker.Outcome(); err != nil {
		return unitStateDoc{}, errors.Annotatef(err, "persisting uniter state")
	}
	return newStDoc, nil
}

// fields returns set and unset bson required to update the unit state doc
// based the current data stored compared to this operation.
func (op *unitSetStateOperation) fields(currentDoc unitStateDoc) (bson.D, bson.D, error) {
	// Handling fields of op.newState:
	// If a pointer is nil, ignore it.
	// If the value referenced by the pointer is empty, remove that thing.
	// If there is a value referenced by the pointer, set the value if a string, or merge the data.
	setFields := bson.D{}
	unsetFields := bson.D{}

	// Check if we need to update the charm state
	if chState, found := op.newState.CharmState(); found {
		if len(chState) == 0 {
			unsetFields = append(unsetFields, bson.DocElem{Name: "charm-state"})
		} else {
			// State keys may contain dots or dollar chars which need to be escaped.
			escapedCharmState := make(bson.M, len(chState))
			for k, v := range chState {
				escapedCharmState[mgoutils.EscapeKey(k)] = v
			}
			if !currentDoc.charmStateMatches(escapedCharmState) {
				setFields = append(setFields, bson.DocElem{"charm-state", escapedCharmState})

				quotaChecker := op.getCharmStateQuotaChecker()
				quotaChecker.Check(escapedCharmState)
				if err := quotaChecker.Outcome(); err != nil {
					if errors.IsQuotaLimitExceeded(err) {
						return nil, nil, errors.Annotatef(err, "persisting charm state")
					}
					return nil, nil, errors.Trace(err)
				}
			}
		}
	}

	// Enforce max uniter internal state size by accumulating the size of
	// the various uniter-related state bits.
	quotaChecker := op.getUniterStateQuotaChecker()
	if uniterState, found := op.newState.UniterState(); found {
		if uniterState == "" {
			unsetFields = append(unsetFields, bson.DocElem{Name: "uniter-state"})
		} else if uniterState != currentDoc.UniterState {
			setFields = append(setFields, bson.DocElem{"uniter-state", uniterState})
			quotaChecker.Check(uniterState)
		}
	} else {
		quotaChecker.Check(currentDoc.UniterState)
	}

	if rState, found := op.newState.relationStateBSONFriendly(); found {
		if len(rState) == 0 {
			unsetFields = append(unsetFields, bson.DocElem{Name: "relation-state"})
		} else if matches := currentDoc.relationStateMatches(rState); !matches {
			setFields = append(setFields, bson.DocElem{"relation-state", rState})
			quotaChecker.Check(rState)
		}
	} else {
		quotaChecker.Check(currentDoc.RelationState)
	}

	if storState, found := op.newState.StorageState(); found {
		if storState == "" {
			unsetFields = append(unsetFields, bson.DocElem{Name: "storage-state"})
		} else if storState != currentDoc.StorageState {
			setFields = append(setFields, bson.DocElem{"storage-state", storState})
			quotaChecker.Check(storState)
		}
	}

	if meterStatusState, found := op.newState.MeterStatusState(); found {
		if meterStatusState == "" {
			unsetFields = append(unsetFields, bson.DocElem{Name: "meter-status-state"})
		} else if meterStatusState != currentDoc.MeterStatusState {
			setFields = append(setFields, bson.DocElem{"meter-status-state", meterStatusState})
			quotaChecker.Check(meterStatusState)
		}
	}

	if err := quotaChecker.Outcome(); err != nil {
		if errors.IsQuotaLimitExceeded(err) {
			return nil, nil, errors.Annotatef(err, "persisting internal uniter state")
		}
		return nil, nil, errors.Trace(err)
	}

	return setFields, unsetFields, nil
}

func (op *unitSetStateOperation) getCharmStateQuotaChecker() quota.Checker {
	// Enforce max key/value length (fixed) and maximum
	// charm state size (configured by the operator).
	return quota.NewMultiChecker(
		quota.NewMapKeyValueSizeChecker(quota.MaxCharmStateKeySize, quota.MaxCharmStateValueSize),
		quota.NewBSONTotalSizeChecker(op.limits.MaxCharmStateSize),
	)
}

func (op *unitSetStateOperation) getUniterStateQuotaChecker() quota.Checker {
	return quota.NewMultiChecker(
		quota.NewBSONTotalSizeChecker(op.limits.MaxAgentStateSize),
	)
}

// Done implements ModelOperation.
func (op *unitSetStateOperation) Done(err error) error { return err }
