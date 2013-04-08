package state

import (
	"fmt"
	"labix.org/v2/mgo"
	"labix.org/v2/mgo/txn"
)

// getStatus retrieves the status document associated with the given
// globalKey and copies it to outStatusDoc, which needs to be created
// by the caller before.
func getStatus(st *State, globalKey string, outStatusDoc interface{}) error {
	err := st.statuses.FindId(globalKey).One(outStatusDoc)
	if err == mgo.ErrNotFound {
		return NotFoundf("status")
	}
	if err != nil {
		return fmt.Errorf("cannot get status %q: %v", globalKey, err)
	}
	return nil
}

// createStatusOp returns the operation needed to create the given
// status document associated with the given globalKey.
func createStatusOp(st *State, globalKey string, statusDoc interface{}) txn.Op {
	return txn.Op{
		C:      st.statuses.Name,
		Id:     globalKey,
		Assert: txn.DocMissing,
		Insert: statusDoc,
	}
}

// updateStatusOp returns the operations needed to update the given
// status document associated with the given globalKey.
func updateStatusOp(st *State, globalKey string, statusDoc interface{}) txn.Op {
	return txn.Op{
		C:      st.statuses.Name,
		Id:     globalKey,
		Assert: txn.DocExists,
		Update: D{{"$set", statusDoc}},
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
