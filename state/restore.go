// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"github.com/juju/errors"
	"gopkg.in/mgo.v2"
	"gopkg.in/mgo.v2/bson"
	"gopkg.in/mgo.v2/txn"
)

// RestoreStatus is the type of the statuses
type RestoreStatus string

const (
	currentRestoreId = "current"

	// UnknownRestoreStatus is the initial status for restoreInfoDoc.
	UnknownRestoreStatus RestoreStatus = "UNKNOWN"
	// RestorePending is a status to signal that a restore is about to start
	// any change done in this status will be lost.
	RestorePending RestoreStatus = "PENDING"
	// RestoreInProgress indicates that a Restore is in progress.
	RestoreInProgress RestoreStatus = "RESTORING"
	// RestoreFinished it is set by restore upon a succesful run.
	RestoreFinished RestoreStatus = "RESTORED"
	// RestoreChecked is set when the server comes up after a succesful restore.
	RestoreChecked RestoreStatus = "CHECKED"
	// RestoreFailed indicates that the process failed in a recoverable step.
	RestoreFailed RestoreStatus = "FAILED"
)

type restoreInfoDoc struct {
	Id     string        `bson:"_id"`
	Status RestoreStatus `bson:"status"`
}

// RestoreInfo its used to syncronize Restore and machine agent
type RestoreInfo struct {
	st  *State
	doc restoreInfoDoc
}

// Status returns the current Restore doc status
func (info *RestoreInfo) Status() RestoreStatus {
	return info.doc.Status
}

// SetStatus sets the status of the current restore. Checks are made
// to ensure that status changes are performed in the correct order.
func (info *RestoreInfo) SetStatus(status RestoreStatus) error {
	var assertSane bson.D

	if status == RestoreInProgress {
		assertSane = bson.D{{"status", RestorePending}}
	}
	if status == RestoreChecked {
		assertSane = bson.D{{"status", RestoreFinished}}
	}

	ops := []txn.Op{{
		C:      restoreInfoC,
		Id:     currentRestoreId,
		Assert: assertSane,
		Update: bson.D{{"$set", bson.D{{"status", status}}}},
	}}
	err := info.st.runTransaction(ops)
	if err == txn.ErrAborted {
		return errors.Errorf("cannot set restore status to %q: Another "+
			"status change occurred concurrently", status)
	}
	return errors.Annotatef(err, "cannot set restore status to %q", status)
}

// RestoreInfoSetter returns the current info doc, if it does not exists
// it creates it with UnknownRestoreStatus status
func (st *State) RestoreInfoSetter() (*RestoreInfo, error) {
	doc := restoreInfoDoc{}
	restoreInfo, closer := st.getCollection(restoreInfoC)
	defer closer()
	err := restoreInfo.Find(bson.M{"_id": currentRestoreId}).One(&doc)
	if err == nil {
		return &RestoreInfo{st: st, doc: doc}, nil
	}

	if err != mgo.ErrNotFound {
		return nil, errors.Annotate(err, "cannot read restore info")
	}
	doc = restoreInfoDoc{
		Id:     currentRestoreId,
		Status: UnknownRestoreStatus,
	}
	ops := []txn.Op{{
		C:      restoreInfoC,
		Id:     currentRestoreId,
		Assert: txn.DocMissing,
		Insert: doc,
	}}

	if err := st.runTransaction(ops); err != nil {
		return nil, errors.Annotate(err, "cannot create restore info")
	}

	return &RestoreInfo{st: st, doc: doc}, nil
}
