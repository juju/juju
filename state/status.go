package state

import (
	"fmt"
	"labix.org/v2/mgo"
	"labix.org/v2/mgo/txn"
)

// UnitStatus represents the status of the unit agent.
type UnitStatus string

const (
	UnitPending   UnitStatus = "pending"   // Agent hasn't started
	UnitInstalled UnitStatus = "installed" // Agent has run the installed hook
	UnitStarted   UnitStatus = "started"   // Agent is running properly
	UnitStopped   UnitStatus = "stopped"   // Agent has stopped running on request
	UnitError     UnitStatus = "error"     // Agent is waiting in an error state
	UnitDown      UnitStatus = "down"      // Agent is down or not communicating
)

// MachineStatus represents the status of the machine agent.
type MachineStatus string

const (
	MachinePending MachineStatus = "pending" // Agent hasn't started
	MachineStarted MachineStatus = "started" // Agent is running properly
	MachineStopped MachineStatus = "stopped" // Agent has stopped running on request
	MachineError   MachineStatus = "error"   // Agent is waiting in an error state
	MachineDown    MachineStatus = "down"    // Agent is down or not communicating
)

// globalKeyer represents the interfave, used to get a global key of a
// state object.
type globalKeyer interface {
	globalKey() string
}

// unitStatusDoc represents the internal state of a unit status in MongoDB.
// There is an implicit _id field here, which mongo creates, which is the
// global key of the unit which is referred to.
type unitStatusDoc struct {
	Status     UnitStatus
	StatusInfo string
}

// machineStatusDoc represents the internal state of a machine status in MongoDB
// There is an implicit _id field here, which mongo creates, which is the
// global key of the unit which is referred to.
type machineStatusDoc struct {
	Status     MachineStatus
	StatusInfo string
}

// getStatus retrieves the status document associated with the given
// globalKeyer and copies it to outStatusDoc, which needs to be
// created by the caller before.
func getStatus(st *State, keyer globalKeyer, outStatusDoc interface{}) error {
	key := keyer.globalKey()
	err := st.statuses.FindId(key).One(outStatusDoc)
	if err == mgo.ErrNotFound {
		return NotFoundf("status %q", key)
	}
	if err != nil {
		return fmt.Errorf("cannot get status %q: %v", key, err)
	}
	return nil
}

// setStatusOp returns the operation needed to set the status document
// associated with the given globalKeyer to the given statusDoc.
func setStatusOp(st *State, keyer globalKeyer, statusDoc interface{}) txn.Op {
	key := keyer.globalKey()
	// We don't care about the error here, we just want to know
	// whether the document exists or not; we're setting it anyway.
	if count, _ := st.statuses.FindId(key).Count(); count > 0 {
		return txn.Op{
			C:      st.statuses.Name,
			Id:     key,
			Assert: txn.DocExists,
			Update: D{{"$set", statusDoc}},
		}
	}
	return txn.Op{
		C:      st.statuses.Name,
		Id:     key,
		Assert: txn.DocMissing,
		Insert: statusDoc,
	}
}
