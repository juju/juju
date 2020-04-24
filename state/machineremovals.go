// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"sort"
	"strings"

	"github.com/juju/collections/set"
	"github.com/juju/errors"
	"github.com/juju/names/v4"
	"gopkg.in/mgo.v2/bson"
	"gopkg.in/mgo.v2/txn"
)

// machineRemovalDoc indicates that this machine needs to be removed
// and any necessary provider-level cleanup should now be done.
type machineRemovalDoc struct {
	DocID     string `bson:"_id"`
	MachineID string `bson:"machine-id"`
}

func (m *Machine) markForRemovalOps() ([]txn.Op, error) {
	if m.doc.Life != Dead {
		return nil, errors.Errorf("machine is not dead")
	}
	ops := []txn.Op{{
		C:  machinesC,
		Id: m.doc.DocID,
		// Check that the machine is still dead (and implicitly that
		// it still exists).
		Assert: isDeadDoc,
	}, {
		C:      machineRemovalsC,
		Id:     m.globalKey(),
		Insert: &machineRemovalDoc{MachineID: m.Id()},
		// No assert here - it's ok if the machine has already been
		// marked. The id will prevent duplicates.
	}}
	return ops, nil
}

// MarkForRemoval requests that this machine be removed after any
// needed provider-level cleanup is done.
func (m *Machine) MarkForRemoval() (err error) {
	defer errors.DeferredAnnotatef(&err, "cannot remove machine %s", m.doc.Id)
	// Local variable so we can refresh the machine if needed.
	machine := m
	buildTxn := func(attempt int) ([]txn.Op, error) {
		if attempt != 0 {
			if machine, err = machine.st.Machine(machine.Id()); err != nil {
				return nil, errors.Trace(err)
			}
		}
		ops, err := machine.markForRemovalOps()
		if err != nil {
			return nil, errors.Trace(err)
		}
		return ops, nil
	}
	return m.st.db().Run(buildTxn)
}

// AllMachineRemovals returns (the ids of) all of the machines that
// need to be removed but need provider-level cleanup.
func (st *State) AllMachineRemovals() ([]string, error) {
	removals, close := st.db().GetCollection(machineRemovalsC)
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
	machines, close := st.db().GetCollection(machinesC)
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

func plural(count int) string {
	if count == 1 {
		return ""
	}
	return "s"
}

func collectMissingMachineIds(expectedIds []string, machines []*Machine) []string {
	expectedSet := set.NewStrings(expectedIds...)
	actualSet := set.NewStrings()
	for _, machine := range machines {
		actualSet.Add(machine.Id())
	}
	return expectedSet.Difference(actualSet).SortedValues()
}

func checkValidMachineIds(machineIds []string) error {
	var invalidIds []string
	for _, id := range machineIds {
		if !names.IsValidMachine(id) {
			invalidIds = append(invalidIds, id)
		}
	}
	if len(invalidIds) == 0 {
		return nil
	}
	return errors.Errorf("Invalid machine id%s: %s",
		plural(len(invalidIds)),
		strings.Join(invalidIds, ", "),
	)
}

func (st *State) completeMachineRemovalsOps(ids []string) ([]txn.Op, error) {
	removals, err := st.AllMachineRemovals()
	if err != nil {
		return nil, errors.Trace(err)
	}
	removalSet := set.NewStrings(removals...)
	query := bson.D{{"machineid", bson.D{{"$in", ids}}}}
	machinesToRemove, err := st.allMachinesMatching(query)
	if err != nil {
		return nil, errors.Trace(err)
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
			Assert: txn.DocExists,
			Remove: true,
		})
		removeMachineOps, err := machine.removeOps()
		if err != nil {
			return nil, errors.Trace(err)
		}
		ops = append(ops, removeMachineOps...)
	}
	// We should complain about machines that still exist but haven't
	// been marked for removal.
	if len(missingRemovals) > 0 {
		sort.Strings(missingRemovals)
		return nil, errors.Errorf(
			"cannot remove machine%s %s: not marked for removal",
			plural(len(missingRemovals)),
			strings.Join(missingRemovals, ", "),
		)
	}

	// Log last to reduce the likelihood of repeating the message on
	// retries.
	if len(machinesToRemove) < len(ids) {
		missingMachines := collectMissingMachineIds(ids, machinesToRemove)
		logger.Debugf("skipping nonexistent machine%s: %s",
			plural(len(missingMachines)),
			strings.Join(missingMachines, ", "),
		)
	}

	return ops, nil
}

// CompleteMachineRemovals finishes the removal of the specified
// machines. The machines must have been marked for removal
// previously. Valid-looking-but-unknown machine ids are ignored so
// that this is idempotent.
func (st *State) CompleteMachineRemovals(ids ...string) error {
	if err := checkValidMachineIds(ids); err != nil {
		return errors.Trace(err)
	}

	buildTxn := func(int) ([]txn.Op, error) {
		// We don't need to reget state for subsequent attempts since
		// completeMachineRemovalsOps gets the removals and the
		// machines each time anyway.
		ops, err := st.completeMachineRemovalsOps(ids)
		if err != nil {
			return nil, errors.Trace(err)
		}
		return ops, nil
	}
	return st.db().Run(buildTxn)
}
