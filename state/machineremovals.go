// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"github.com/juju/errors"
	"gopkg.in/mgo.v2/txn"
)

// machineRemovalDoc stores information needed to clean up provider
// resources once the machine has been removed.
type machineRemovalDoc struct {
	DocID     string `bson:"_id"`
	MachineID string `bson:"machine-id"`
}

type MachineRemoval struct {
	st  *State
	doc machineRemovalDoc
}

func newMachineRemoval(st *State, doc machineRemovalDoc) *MachineRemoval {
	return &MachineRemoval{st: st, doc: doc}
}

func (m *MachineRemoval) MachineID() string {
	return m.doc.MachineID
}

func addMachineRemovalOp(machineID string) txn.Op {
	return txn.Op{
		C:      machineRemovalsC,
		Id:     machineID,
		Insert: &machineRemovalDoc{MachineID: machineID},
	}
}

func (st *State) AllMachineRemovals() ([]*MachineRemoval, error) {
	removals, close := st.getCollection(machineRemovalsC)
	defer close()

	var docs []machineRemovalDoc
	err := removals.Find(nil).All(&docs)
	if err != nil {
		return nil, errors.Trace(err)
	}
	results := make([]*MachineRemoval, len(docs))
	for i, doc := range docs {
		results[i] = newMachineRemoval(st, doc)
	}
	return results, nil
}

func (st *State) ClearMachineRemovals(ids []string) error {
	ops := make([]txn.Op, len(ids))
	for i, id := range ids {
		ops[i] = txn.Op{
			C:      machineRemovalsC,
			Id:     id,
			Remove: true,
		}
	}
	return st.runTransaction(ops)
}
