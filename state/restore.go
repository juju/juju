// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"github.com/juju/errors"
	"gopkg.in/mgo.v2"
	"gopkg.in/mgo.v2/bson"
	"gopkg.in/mgo.v2/txn"
)

type RestoreStatus string

const (
	currentRestoreId = "curent"

	UnknownRestoreStatus RestoreStatus = "UNKNOWN"
	RestorePending RestoreStatus = "PENDING"
	RestoreInProgress RestoreStatus = "RESTORING"
	RestoreFinished RestoreStatus	= "RESTORED"
)


type restoreInfoDoc struct {
	Id             string `bson:"_id"`
	status             RestoreStatus `bson:"status"`
}

type RestoreInfo struct {
	st  *State
	doc restoreInfoDoc
}

func currentRestoreInfoDoc(st *State) (*restoreInfoDoc, error) {
	var doc restoreInfoDoc
	restoreInfo, closer := st.getCollection(restoreInfoC)
	defer closer()
	if err := restoreInfo.FindId(currentRestoreId).One(&doc); err == mgo.ErrNotFound {
		return nil, nil
	} else if err != nil {
		return nil, errors.Annotate(err, "cannot read restore info")
	}
	return &doc, nil
}

func (info *RestoreInfo) Status() RestoreStatus {
	return info.doc.status
}

// SetStatus sets the status of the current restore. Checks are made
// to ensure that status changes are performed in the correct order.
func (info *RestoreInfo) SetStatus(status RestoreStatus) error {
	var assertSane bson.D
	switch status {
	case RestoreInProgress:
		assertSane = bson.D{{"status", RestorePending}}
	}

	ops := []txn.Op{{
		C:  restoreInfoC,
		Id: currentRestoreId,
		Assert: assertSane,
		Update: bson.D{{"$set", bson.D{{"status", status}}}},
	}}
	err := info.st.runTransaction(ops)
	if err == txn.ErrAborted {
		return errors.Errorf("cannot set restore status to %q: Another "+
			"status change may have occurred concurrently", status)
	}
	return errors.Annotate(err, "cannot set restore status")
}

func (st *State) EnsureRestoreInfo() (*RestoreInfo, error) {
	var doc restoreInfoDoc
	cdoc, err := currentRestoreInfoDoc(st)
	if err != nil {
		return nil, errors.Annotate(err, "cannot ensure restore info")
	}
	if cdoc == nil{	
		doc = restoreInfoDoc{
			Id: currentRestoreId,
			status:	UnknownRestoreStatus,
		}
		ops := []txn.Op{{
			C:      restoreInfoC,
			Id:     currentRestoreId,
			Assert: txn.DocMissing,
			Insert: doc,
		},}
		if err := st.runTransaction(ops); err != nil {
			return nil, errors.Annotate(err, "cannot create restore info")
		}
	} else {
		doc = *cdoc
	}
	return &RestoreInfo{st: st, doc: doc}, nil


}
