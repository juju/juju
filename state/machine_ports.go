// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"github.com/juju/errors"
	"github.com/juju/mgo/v3"
	"github.com/juju/mgo/v3/bson"
	"github.com/juju/mgo/v3/txn"
	jujutxn "github.com/juju/txn/v3"

	"github.com/juju/juju/core/network"
)

// machinePortRangesDoc represents the state of ports opened on machines by
// individual units.
// machinePortRangesDoc is used for the IaaS application only.
// We should really consider to use the applicationPortRangesDoc for IaaS as well.
type machinePortRangesDoc struct {
	DocID      string                               `bson:"_id"`
	ModelUUID  string                               `bson:"model-uuid"`
	MachineID  string                               `bson:"machine-id"`
	UnitRanges map[string]network.GroupedPortRanges `bson:"unit-port-ranges"`
	TxnRevno   int64                                `bson:"txn-revno"`
}

// machinePortRanges implements MachineUnitPortRanges using the DB as the
// backing store.
type machinePortRanges struct {
	st  *State
	doc machinePortRangesDoc

	machineExists bool

	// docExists is false if the port range doc has not yet been persisted
	// to the backing store.
	docExists bool

	// The set of pending port ranges that have not yet been persisted.
	pendingOpenRanges  map[string]network.GroupedPortRanges
	pendingCloseRanges map[string]network.GroupedPortRanges
}

// MachineID returns the machine ID associated with this set of port ranges.
func (p *machinePortRanges) MachineID() string {
	return p.doc.MachineID
}

// Persisted returns true if the underlying document for this instance exists
// in the database.
func (p *machinePortRanges) Persisted() bool {
	return p.docExists
}

// ByUnit returns the set of port ranges opened by each unit in a particular
// machine subnet grouped by unit name.
func (p *machinePortRanges) ByUnit() map[string]UnitPortRanges {
	if len(p.doc.UnitRanges) == 0 {
		return nil
	}
	res := make(map[string]UnitPortRanges)
	for unitName := range p.doc.UnitRanges {
		res[unitName] = &unitPortRangesForMachine{
			unitName:          unitName,
			machinePortRanges: p,
		}
	}
	return res
}

// ForUnit returns the set of port ranges opened by the specified unit
// in a particular machine subnet.
func (p *machinePortRanges) ForUnit(unitName string) UnitPortRanges {
	return &unitPortRangesForMachine{
		unitName:          unitName,
		machinePortRanges: p,
	}
}

// UniquePortRanges returns a slice of unique open PortRanges for all units on
// this machine.
func (p *machinePortRanges) UniquePortRanges() []network.PortRange {
	var allRanges []network.PortRange
	for _, unitRanges := range p.ByUnit() {
		allRanges = append(allRanges, unitRanges.UniquePortRanges()...)
	}

	network.SortPortRanges(allRanges)
	return allRanges
}

// Changes returns a ModelOperation for applying any changes that were made to
// this port range instance for all machine units.
func (p *machinePortRanges) Changes() ModelOperation {
	return &openClosePortRangesOperation{
		mpr: p,
	}
}

// Refresh refreshes the port document from state.
func (p *machinePortRanges) Refresh() error {
	openedPorts, closer := p.st.db().GetCollection(openedPortsC)
	defer closer()

	err := openedPorts.FindId(p.doc.DocID).One(&p.doc)
	if err == mgo.ErrNotFound {
		return errors.NotFoundf("open port ranges for machine %q", p.MachineID())
	} else if err != nil {
		return errors.Annotatef(err, "refresh open port ranges for machine %q", p.MachineID())
	}
	return nil
}

// Remove removes the ports document from state.
func (p *machinePortRanges) Remove() error {
	doc := &machinePortRanges{st: p.st, doc: p.doc, docExists: p.docExists}
	buildTxn := func(attempt int) ([]txn.Op, error) {
		if attempt > 0 {
			err := doc.Refresh()
			if errors.IsNotFound(err) {
				return nil, jujutxn.ErrNoOperations
			} else if err != nil {
				return nil, errors.Trace(err)
			}
		}
		return doc.removeOps(), nil
	}
	if err := p.st.db().Run(buildTxn); err != nil {
		return errors.Trace(err)
	}

	p.docExists = false
	return nil
}

func insertPortsDocOps(doc *machinePortRangesDoc, asserts interface{}, unitRanges map[string]network.GroupedPortRanges) []txn.Op {
	// As the following insert operation might be rolled back, we should
	// not mutate our internal doc but instead work on a copy of the
	// machinePortRangesDoc.
	docCopy := new(machinePortRangesDoc)
	*docCopy = *doc
	docCopy.UnitRanges = unitRanges

	return []txn.Op{
		{
			C:      openedPortsC,
			Id:     docCopy.DocID,
			Assert: asserts,
			Insert: docCopy,
		},
	}
}

func updatePortsDocOps(doc *machinePortRangesDoc, asserts interface{}, unitRanges map[string]network.GroupedPortRanges) []txn.Op {
	return []txn.Op{
		{
			C:      openedPortsC,
			Id:     doc.DocID,
			Assert: asserts,
			Update: bson.D{{"$set", bson.D{{"unit-port-ranges", unitRanges}}}},
		},
	}
}

func (p *machinePortRanges) removeOps() []txn.Op {
	return []txn.Op{{
		C:      openedPortsC,
		Id:     p.doc.DocID,
		Assert: txn.DocExists,
		Remove: true,
	}}
}

// removePortsForUnitOps returns the ops needed to remove all opened
// ports for the given unit on its assigned machine.
func removePortsForUnitOps(st *State, unit *Unit) ([]txn.Op, error) {
	machineID, err := unit.AssignedMachineId()
	if err != nil {
		// No assigned machine, so there won't be any ports.
		return nil, nil
	}

	machinePortRanges, err := getOpenedMachinePortRanges(st, machineID)
	if err != nil {
		return nil, errors.Trace(err)
	} else if !machinePortRanges.docExists {
		// Machine is removed, so there won't be a ports doc for it.
		return nil, nil
	}

	unitName := unit.Name()
	if machinePortRanges.doc.UnitRanges == nil || machinePortRanges.doc.UnitRanges[unitName] == nil {
		// No entry for the unit; nothing to do here
		return nil, nil
	}

	// Drop unit rules and write the doc back if non-empty or remove it if empty
	delete(machinePortRanges.doc.UnitRanges, unitName)
	if len(machinePortRanges.doc.UnitRanges) != 0 {
		assert := bson.D{{"txn-revno", machinePortRanges.doc.TxnRevno}}
		return updatePortsDocOps(&machinePortRanges.doc, assert, machinePortRanges.doc.UnitRanges), nil
	}
	return machinePortRanges.removeOps(), nil
}

// OpenedPortRanges returns the set of opened port ranges for this machine.
func (m *Machine) OpenedPortRanges() (MachinePortRanges, error) {
	return getOpenedMachinePortRanges(m.st, m.Id())
}

// OpenedPortRangesForMachine returns the set of opened port ranges for one of
// the model's machines.
func (m *Model) OpenedPortRangesForMachine(machineID string) (MachinePortRanges, error) {
	return getOpenedMachinePortRanges(m.st, machineID)
}

// OpenedPortRangesForAllMachines returns a slice of opened port ranges for all
// machines managed by this model.
func (m *Model) OpenedPortRangesForAllMachines() ([]MachinePortRanges, error) {
	mprResults, err := getOpenedPortRangesForAllMachines(m.st)
	if err != nil {
		return nil, errors.Trace(err)
	}

	results := make([]MachinePortRanges, len(mprResults))
	for i, res := range mprResults {
		results[i] = res
	}
	return results, nil
}

// getOpenedPortRangesForAllMachines returns a slice of machine port ranges for
// all machines managed by this model.
func getOpenedPortRangesForAllMachines(st *State) ([]*machinePortRanges, error) {
	machines, err := st.AllMachines()
	if err != nil {
		return nil, errors.Trace(err)
	}
	var machineIDs []string
	for _, m := range machines {
		machineIDs = append(machineIDs, m.Id())
	}
	openedPorts, closer := st.db().GetCollection(openedPortsC)
	defer closer()

	docs := []machinePortRangesDoc{}
	err = openedPorts.Find(bson.D{{"machine-id", bson.D{{"$in", machineIDs}}}}).All(&docs)
	if err != nil {
		return nil, errors.Trace(err)
	}
	results := make([]*machinePortRanges, len(docs))
	for i, doc := range docs {
		results[i] = &machinePortRanges{
			st:        st,
			doc:       doc,
			docExists: true,
		}
	}
	return results, nil
}

// getOpenedMachinePortRanges attempts to retrieve the set of opened ports for
// a particular machine. If the underlying document does not exist, a blank
// machinePortRanges instance with the docExists flag set to false will be
// returned instead.
func getOpenedMachinePortRanges(st *State, machineID string) (*machinePortRanges, error) {
	_, err := st.getMachineDoc(machineID)
	if err != nil && !errors.Is(err, errors.NotFound) {
		return nil, errors.Annotatef(err, "looking for machine %q", machineID)
	}
	machineExists := err == nil

	openedPorts, closer := st.db().GetCollection(openedPortsC)
	defer closer()

	var doc machinePortRangesDoc
	err = openedPorts.FindId(machineID).One(&doc)
	if err != nil {
		if err != mgo.ErrNotFound {
			return nil, errors.Annotatef(err, "cannot get opened port ranges for machine %q", machineID)
		}
		doc.DocID = st.docID(machineID)
		doc.MachineID = machineID
		doc.ModelUUID = st.ModelUUID()
		return &machinePortRanges{
			st:            st,
			doc:           doc,
			docExists:     false,
			machineExists: machineExists,
		}, nil
	}

	return &machinePortRanges{
		st:            st,
		doc:           doc,
		docExists:     true,
		machineExists: machineExists,
	}, nil
}
