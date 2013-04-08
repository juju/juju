package state

import (
	"fmt"
	"labix.org/v2/mgo"
	"labix.org/v2/mgo/txn"
)

type statusDoc struct {
	Status     string
	StatusInfo string
}

// getStatus retrieves the status document associated with the given
// globalKey and copies it to outStatusDoc, which needs to be created
// by the caller before.
func getStatus(st *State, globalKey string) (statusDoc, error) {
	var doc statusDoc
	err := st.statuses.FindId(globalKey).One(&doc)
	if err == mgo.ErrNotFound {
		return statusDoc{}, NotFoundf("status")
	}
	if err != nil {
		return statusDoc{}, fmt.Errorf("cannot get status %q: %v", globalKey, err)
	}
	return doc, nil
}

// createStatusOp returns the operation needed to create the given
// status document associated with the given globalKey.
func createStatusOp(st *State, globalKey string, doc statusDoc) txn.Op {
	return txn.Op{
		C:      st.statuses.Name,
		Id:     globalKey,
		Assert: txn.DocMissing,
		Insert: doc,
	}
}

// updateStatusOp returns the operations needed to update the given
// status document associated with the given globalKey.
func updateStatusOp(st *State, globalKey string, doc statusDoc) txn.Op {
	return txn.Op{
		C:      st.statuses.Name,
		Id:     globalKey,
		Assert: txn.DocExists,
		Update: D{{"$set", doc}},
	}
}

// removeStatusOp returns the operation needed to remove the status
// document associated with the given globalKey.
func removeStatusOp(st *State, globalKey string) txn.Op {
	return txn.Op{
		C:      st.statuses.Name,
		Id:     globalKey,
		Remove: true,
	}
}
