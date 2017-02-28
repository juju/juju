// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"github.com/juju/errors"

	"github.com/juju/juju/state/workers"
)

// modelBackend collects together some useful internal state methods for
// accessing mongo and mapping local and global ids to one another.
type modelBackend interface {
	docID(string) string
	localID(string) string
	strictLocalID(string) (string, error)
	db() Database
	txnLogWatcher() workers.TxnLogWatcher
}

// docID generates a globally unique id value
// where the model uuid is prefixed to the
// localID.
func (st *State) docID(localID string) string {
	return ensureModelUUID(st.ModelUUID(), localID)
}

// localID returns the local id value by stripping
// off the model uuid prefix if it is there.
func (st *State) localID(ID string) string {
	modelUUID, localID, ok := splitDocID(ID)
	if !ok || modelUUID != st.ModelUUID() {
		return ID
	}
	return localID
}

// strictLocalID returns the local id value by removing the
// model UUID prefix.
//
// If there is no prefix matching the State's model, an error is
// returned.
func (st *State) strictLocalID(ID string) (string, error) {
	modelUUID, localID, ok := splitDocID(ID)
	if !ok || modelUUID != st.ModelUUID() {
		return "", errors.Errorf("unexpected id: %#v", ID)
	}
	return localID, nil
}
