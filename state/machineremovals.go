// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"sort"
	"strings"

	"github.com/juju/utils/set"

	"github.com/juju/errors"
	"gopkg.in/mgo.v2/bson"
	"gopkg.in/mgo.v2/txn"
)

// machineRemovalDoc is a record indicating that a machine needs to be
// removed and any necessary provider-level cleanup should now be done.
type machineRemovalDoc struct {
	DocID     string `bson:"_id"`
	MachineID string `bson:"machine-id"`
}

func (m *Machine) MarkForRemoval() (err error) {
	defer errors.DeferredAnnotatef(&err, "can't remove machine %s", m.doc.Id)
	if m.doc.Life != Dead {
		return errors.Errorf("machine is not dead")
	}
	ops := []txn.Op{{
		C:      machineRemovalsC,
		Id:     m.globalKey(),
		Insert: &machineRemovalDoc{MachineID: m.Id()},
	}}
	return m.st.runTransaction(ops)
}

// AllMachineRemovals returns (the ids of) all of the machines that
// need to be removed but need provider-level cleanup.
func (st *State) AllMachineRemovals() ([]string, error) {
	removals, close := st.getCollection(machineRemovalsC)
	defer close()

	var docs []machineRemovalDoc
	err := removals.Find(nil).All(&docs)
	if err != nil {
		return nil, errors.Trace(err)
	}
	results := make([]string, len(docs))
	for i := range docs {
		results[i] = docs[i].MachineID
	}
	return results, nil
}

func (st *State) allMachinesMatching(query bson.D) ([]*Machine, error) {
	machines, close := st.getCollection(machinesC)
	defer close()

	var docs []machineDoc
	err := machines.Find(query).All(&docs)
	if err != nil {
		return nil, errors.Trace(err)
	}
	results := make([]*Machine, len(docs))
	for i, doc := range docs {
		results[i] = newMachine(st, &doc)
	}
	return results, nil
}

// CompleteMachineRemovals finishes the removal of the specified
// machines. The machines must have been marked for removal
// previously. Unknown machine ids are ignored so that this is
// idempotent.
func (st *State) CompleteMachineRemovals(ids []string) error {
	removals, err := st.AllMachineRemovals()
	if err != nil {
		return errors.Trace(err)
	}
	removalSet := set.NewStrings(removals...)
	query := bson.D{{"machineid", bson.D{{"$in", ids}}}}
	machinesToRemove, err := st.allMachinesMatching(query)
	if err != nil {
		return errors.Trace(err)
	}

	var ops []txn.Op
	var missingRemovals []string
	for _, machine := range machinesToRemove {
		if !removalSet.Contains(machine.Id()) {
			missingRemovals = append(missingRemovals, machine.Id())
			continue
		}

		ops = append(ops, txn.Op{
			C:      machineRemovalsC,
			Id:     machine.globalKey(),
			Remove: true,
		})
		removeMachineOps, err := machine.removeOps()
		if err != nil {
			return errors.Trace(err)
		}
		ops = append(ops, removeMachineOps...)
	}
	// We should complain about machines that still exist but haven't
	// been marked for removal.
	if len(missingRemovals) > 0 {
		sort.Strings(missingRemovals)
		return errors.Errorf(
			"can't remove machines [%s]: not marked for removal",
			strings.Join(missingRemovals, ", "),
		)
	}

	return st.runTransaction(ops)
}
