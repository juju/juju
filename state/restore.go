// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"github.com/juju/errors"
	"github.com/juju/mgo/v2"
	"github.com/juju/mgo/v2/bson"
	"github.com/juju/mgo/v2/txn"
	jujutxn "github.com/juju/txn"
)

// RestoreStatus is the type of the statuses
type RestoreStatus string

// Validate returns an errors if status' value is not known.
func (status RestoreStatus) Validate() error {
	switch status {
	case RestorePending, RestoreInProgress, RestoreFinished:
	case RestoreChecked, RestoreFailed, RestoreNotActive:
	default:
		return errors.Errorf("unknown restore status: %v", status)
	}
	return nil
}

const (
	currentRestoreId = "current"

	// RestoreNotActive is not persisted in the database, and is
	// used to indicate the absence of a current restore doc.
	RestoreNotActive RestoreStatus = "NOT-RESTORING"

	// RestorePending is a status to signal that a restore is about
	// to start any change done in this status will be lost.
	RestorePending RestoreStatus = "PENDING"

	// RestoreInProgress indicates that a Restore is in progress.
	RestoreInProgress RestoreStatus = "RESTORING"

	// RestoreFinished it is set by restore upon a successful run.
	RestoreFinished RestoreStatus = "RESTORED"

	// RestoreChecked is set when the server comes up after a
	// successful restore.
	RestoreChecked RestoreStatus = "CHECKED"

	// RestoreFailed indicates that the process failed in a
	// recoverable step.
	RestoreFailed RestoreStatus = "FAILED"
)

// RestoreInfo exposes restore status.
func (st *State) RestoreInfo() *RestoreInfo {
	return &RestoreInfo{st: st}
}

// RestoreInfo exposes restore status.
type RestoreInfo struct {
	st *State
}

// Status returns the current Restore doc status
func (info *RestoreInfo) Status() (RestoreStatus, error) {
	restoreInfo, closer := info.st.db().GetCollection(restoreInfoC)
	defer closer()

	var doc struct {
		Status RestoreStatus `bson:"status"`
	}
	err := restoreInfo.FindId(currentRestoreId).One(&doc)
	switch errors.Cause(err) {
	case nil:
	case mgo.ErrNotFound:
		return RestoreNotActive, nil
	default:
		return "", errors.Annotate(err, "cannot read restore info")
	}

	if err := doc.Status.Validate(); err != nil {
		return "", errors.Trace(err)
	}
	return doc.Status, nil
}

// PurgeTxn purges missing transition from restoreInfoC collection.
// These can be caused because this collection is heavy use while backing
// up and mongo 3.2 does not like this.
func (info *RestoreInfo) PurgeTxn() error {
	restoreInfo, closer := info.st.db().GetRawCollection(restoreInfoC)
	defer closer()
	r := txn.NewRunner(restoreInfo)
	return r.PurgeMissing(restoreInfoC)
}

// SetStatus sets the status of the current restore. Checks are made
// to ensure that status changes are performed in the correct order.
func (info *RestoreInfo) SetStatus(status RestoreStatus) error {
	if err := status.Validate(); err != nil {
		return errors.Trace(err)
	}
	buildTxn := func(_ int) ([]txn.Op, error) {
		current, err := info.Status()
		if err != nil {
			return nil, errors.Annotate(err, "cannot read current status")
		}
		if current == status {
			return nil, jujutxn.ErrNoOperations
		}

		ops, err := setRestoreStatusOps(current, status)
		if err != nil {
			return nil, errors.Trace(err)
		}
		return ops, nil
	}
	if err := info.st.db().Run(buildTxn); err != nil {
		return errors.Annotatef(err, "setting status %q", status)
	}
	return nil
}

// setRestoreStatusOps checks the validity of the supplied transition,
// and returns either an error or a list of transaction operations that
// will apply the transition.
func setRestoreStatusOps(before, after RestoreStatus) ([]txn.Op, error) {
	errInvalid := errors.Errorf("invalid restore transition: %s => %s", before, after)
	switch after {
	case RestorePending:
		switch before {
		case RestoreNotActive:
			return createRestoreStatusPendingOps(), nil
		case RestoreFailed, RestoreChecked:
		default:
			return nil, errInvalid
		}
	case RestoreFailed:
		switch before {
		case RestoreNotActive, RestoreChecked:
			return nil, errInvalid
		}
	case RestoreInProgress:
		if before != RestorePending {
			return nil, errInvalid
		}
	case RestoreFinished:
		// RestoreFinished is set after a restore so we cannot ensure
		// what will be on the db state since it will deppend on
		// what was set during backup.
		switch before {
		case RestoreNotActive:
			return createRestoreStatusFinishedOps(), nil
		case RestoreFailed:
			// except for the case of Failed,this is most likely a race condition.
			return nil, errInvalid
		}

	case RestoreChecked:
		if before != RestoreFinished {
			return nil, errInvalid
		}
	default:
		return nil, errInvalid
	}
	return updateRestoreStatusChangeOps(before, after), nil
}

// createRestoreStatusFinishedOps is useful when setting finished on
// a non initated restore document.
func createRestoreStatusFinishedOps() []txn.Op {
	return []txn.Op{{
		C:      restoreInfoC,
		Id:     currentRestoreId,
		Assert: txn.DocMissing,
		Insert: bson.D{{"status", RestoreFinished}},
	}}
}

// createRestoreStatusPendingOps is the only valid way to create a
// restore document.
func createRestoreStatusPendingOps() []txn.Op {
	return []txn.Op{{
		C:      restoreInfoC,
		Id:     currentRestoreId,
		Assert: txn.DocMissing,
		Insert: bson.D{{"status", RestorePending}},
	}}
}

// updateRestoreStatusChangeOps will set the restore doc status to
// after, so long as the doc's status is before.
func updateRestoreStatusChangeOps(before, after RestoreStatus) []txn.Op {
	return []txn.Op{{
		C:      restoreInfoC,
		Id:     currentRestoreId,
		Assert: bson.D{{"status", before}},
		Update: bson.D{{"$set", bson.D{{"status", after}}}},
	}}
}
