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
	DocID            string               `bson:"_id"`
	MachineID        string               `bson:"machine-id"`
	LinkLayerDevices []linkLayerDeviceDoc `bson:"link-layer-devices"`
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

func (m *MachineRemoval) LinkLayerDevices() []*LinkLayerDevice {
	var result []*LinkLayerDevice
	for _, deviceDoc := range m.doc.LinkLayerDevices {
		result = append(result, newLinkLayerDevice(m.st, deviceDoc))
	}
	return result
}

func (m *Machine) machineRemovalOp() (txn.Op, error) {
	var linkLayerDevices []linkLayerDeviceDoc
	appendDevice := func(doc *linkLayerDeviceDoc) {
		linkLayerDevices = append(linkLayerDevices, *doc)
	}
	err := m.forEachLinkLayerDeviceDoc(nil, appendDevice)
	if err != nil {
		return txn.Op{}, errors.Trace(err)
	}
	return txn.Op{
		C:  machineRemovalsC,
		Id: m.Id(),
		Insert: &machineRemovalDoc{
			MachineID:        m.Id(),
			LinkLayerDevices: linkLayerDevices,
		},
	}, nil
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
