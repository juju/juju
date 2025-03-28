// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"github.com/juju/mgo/v3/bson"
	"github.com/juju/mgo/v3/txn"
	jujutxn "github.com/juju/txn/v3"

	"github.com/juju/juju/core/leadership"
	mgoutils "github.com/juju/juju/internal/mongo/utils"
)

type updateSettingsWithLeaderTokenOperation struct {
	db Database

	sets   bson.M
	unsets bson.M

	key       string
	updateDoc bson.D

	tokenAwareTxnBuilder func(int) ([]txn.Op, error)
}

// newUpdateSettingsWithLeaderTokenOperation returns a ModelOperation for
// updating the leader settings for a particular application.
func newUpdateSettingsWithLeaderTokenOperation(db Database, token leadership.Token, key string, updates map[string]interface{}) ModelOperation {
	// We can calculate the actual update ahead of time; it's not dependent
	// upon the current state of the document. (*Writing* it should depend
	// on document state, but that's handled below.)
	sets := bson.M{}
	unsets := bson.M{}
	for unescapedKey, value := range updates {
		key := mgoutils.EscapeKey(unescapedKey)
		if value == "" {
			unsets[key] = 1
		} else {
			sets[key] = value
		}
	}
	updateDoc := setUnsetUpdateSettings(sets, unsets)

	op := &updateSettingsWithLeaderTokenOperation{
		db:        db,
		sets:      sets,
		unsets:    unsets,
		key:       key,
		updateDoc: updateDoc,
	}

	op.tokenAwareTxnBuilder = func(attempt int) ([]txn.Op, error) {
		return op.buildTxn(attempt)
	}
	return op
}

// Build implements ModelOperation.
func (op *updateSettingsWithLeaderTokenOperation) Build(attempt int) ([]txn.Op, error) {
	return op.tokenAwareTxnBuilder(attempt)
}

func (op *updateSettingsWithLeaderTokenOperation) buildTxn(_ int) ([]txn.Op, error) {
	// Read the current document state so we can abort if there's
	// no actual change; and the version number so we can assert
	// on it and prevent these settings from landing late.
	doc, err := readSettingsDoc(op.db, settingsC, op.key)
	if err != nil {
		return nil, err
	}
	if op.isNullChange(doc.Settings) {
		return nil, jujutxn.ErrNoOperations
	}
	return []txn.Op{{
		C:      settingsC,
		Id:     op.key,
		Assert: bson.D{{"version", doc.Version}},
		Update: op.updateDoc,
	}}, nil
}

func (op *updateSettingsWithLeaderTokenOperation) isNullChange(rawMap map[string]interface{}) bool {
	for key := range op.unsets {
		if _, found := rawMap[key]; found {
			return false
		}
	}
	for key, value := range op.sets {
		if current := rawMap[key]; current != value {
			return false
		}
	}
	return true
}

// Done implements ModelOperation.
func (op *updateSettingsWithLeaderTokenOperation) Done(err error) error { return err }
