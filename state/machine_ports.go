// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"fmt"
	"regexp"

	"github.com/juju/errors"
	"github.com/juju/names/v4"
	jujutxn "github.com/juju/txn"
	"gopkg.in/mgo.v2"
	"gopkg.in/mgo.v2/bson"
	"gopkg.in/mgo.v2/txn"

	"github.com/juju/juju/core/network"
)

// A regular expression for parsing ports document id into corresponding machine
// and subnet ids.
var portsIDRe = regexp.MustCompile(fmt.Sprintf("m#(?P<machine>%s)#(?P<subnet>.*)$", names.MachineSnippet))

type portIDPart int

const (
	_ portIDPart = iota
	machineIDPart
	subnetIDPart
)

// portsGlobalKey returns the global database key for the opened ports
// document for the given machine and subnet.
func portsGlobalKey(machineID, subnetID string) string {
	return fmt.Sprintf("m#%s#%s", machineID, subnetID)
}

// extractPortsIDParts parses the given ports global key and extracts
// its parts.
func extractPortsIDParts(globalKey string) ([]string, error) {
	if parts := portsIDRe.FindStringSubmatch(globalKey); len(parts) == 3 {
		return parts, nil
	}
	return nil, errors.NotValidf("ports document key %q", globalKey)
}

// MachineSubnetPorts is implemented by types that can query and/or manipulate
// the set of port ranges opened by one or more units in a particular machine
// subnet.
type MachineSubnetPorts interface {
	// SubnetID returns the ID of the subnet that these port ranges relate to.
	SubnetID() string

	// MachineID returns the ID of the machine that these ports are opened in.
	MachineID() string

	// OpenClosePortsOperation creates a ModelOperation that opens and
	// closes the requested set of PortRange arguments. Each of the
	// PortRange arguments can be left empty to indicate that they should
	// not be ignored.
	OpenClosePortsOperation(unitName string, openPortRanges, closePortRanges []network.PortRange) (ModelOperation, error)

	// PortRangesByUnit returns the set of port ranges opened by each unit
	// in a particular machine subnet grouped by unit name.
	PortRangesByUnit() map[string][]network.PortRange

	// PortRangesForUnit returns the set of port ranges opened by the
	// specified unit in a particular machine subnet.
	PortRangesForUnit(unitName string) []network.PortRange
}

// portsDoc represents the state of ports opened on machines for networks
type portsDoc struct {
	DocID     string         `bson:"_id"`
	ModelUUID string         `bson:"model-uuid"`
	MachineID string         `bson:"machine-id"`
	SubnetID  string         `bson:"subnet-id"`
	Ports     []portRangeDoc `bson:"ports"`
	TxnRevno  int64          `bson:"txn-revno"`
}

// portRangeDoc represents a port range for a particular unit that is nested
// in a portsDoc.
type portRangeDoc struct {
	UnitName string
	FromPort int
	ToPort   int
	Protocol string
}

func (doc portRangeDoc) asPortRange() network.PortRange {
	return network.PortRange{
		FromPort: doc.FromPort,
		ToPort:   doc.ToPort,
		Protocol: doc.Protocol,
	}
}

// machineSubnetPorts represents the state of ports on a machine subnet.
type machineSubnetPorts struct {
	st  *State
	doc portsDoc
	// areNew is true for documents not in state yet.
	areNew bool
}

// String returns p as a user-readable string.
func (p *machineSubnetPorts) String() string {
	return fmt.Sprintf("ports for machine %q, subnet %q", p.doc.MachineID, p.doc.SubnetID)
}

// MachineID returns the machine ID associated with this ports document.
func (p *machineSubnetPorts) MachineID() string {
	return p.doc.MachineID
}

// SubnetID returns the subnet ID associated with this ports document.
func (p *machineSubnetPorts) SubnetID() string {
	return p.doc.SubnetID
}

// Persisted returns true if the underlying document for this set of ports
// exists in the database.
func (p *machineSubnetPorts) Persisted() bool {
	return !p.areNew
}

// OpenClosePortsOperation creates a ModelOperation that opens and closes the
// requested set of PortRange arguments. Each of the PortRange arguments can be
// left empty to indicate that they should not be ignored.
func (p *machineSubnetPorts) OpenClosePortsOperation(unitName string, openPortRanges, closePortRanges []network.PortRange) (ModelOperation, error) {
	// pre-flight checks
	for _, r := range openPortRanges {
		if err := r.Validate(); err != nil {
			return nil, errors.Annotatef(errors.Trace(err), "cannot open ports %v", r)
		}
	}
	for _, r := range closePortRanges {
		if err := r.Validate(); err != nil {
			return nil, errors.Annotatef(errors.Trace(err), "cannot close ports %v", r)
		}
	}

	return &openClosePortsOperation{
		p:               p,
		unitName:        unitName,
		openPortRanges:  openPortRanges,
		closePortRanges: closePortRanges,
	}, nil
}

// PortRangesByUnit returns back a map where keys are unit names and values
// are the list of PortRanges opened by the unit on the subnet represented by
// this MachineSubnetPorts instance.
func (p *machineSubnetPorts) PortRangesByUnit() map[string][]network.PortRange {
	res := make(map[string][]network.PortRange)
	for _, pr := range p.doc.Ports {
		res[pr.UnitName] = append(res[pr.UnitName], pr.asPortRange())
	}
	for unitName := range res {
		network.SortPortRanges(res[unitName])
	}
	return res
}

// PortRangesForUnit returns back a list of PortRanges opened by the specified
// unit on the subnet represented by this MachineSubnetPorts instance.
func (p *machineSubnetPorts) PortRangesForUnit(unitName string) []network.PortRange {
	var res []network.PortRange
	for _, pr := range p.doc.Ports {
		if pr.UnitName == unitName {
			res = append(res, pr.asPortRange())
		}
	}
	network.SortPortRanges(res)
	return res
}

// Refresh refreshes the port document from state.
func (p *machineSubnetPorts) Refresh() error {
	openedPorts, closer := p.st.db().GetCollection(openedPortsC)
	defer closer()

	err := openedPorts.FindId(p.doc.DocID).One(&p.doc)
	if err == mgo.ErrNotFound {
		return errors.NotFoundf(p.String())
	} else if err != nil {
		return errors.Annotatef(err, "cannot refresh %s", p)
	}
	return nil
}

// Remove removes the ports document from state.
func (p *machineSubnetPorts) Remove() error {
	ports := &machineSubnetPorts{st: p.st, doc: p.doc}
	buildTxn := func(attempt int) ([]txn.Op, error) {
		if attempt > 0 {
			err := ports.Refresh()
			if errors.IsNotFound(err) {
				return nil, jujutxn.ErrNoOperations
			} else if err != nil {
				return nil, errors.Trace(err)
			}
		}
		return ports.removeOps(), nil
	}
	return p.st.db().Run(buildTxn)
}

// OpenedPortsInSubnet returns this machine ports document for the given subnetID.
func (m *Machine) OpenedPortsInSubnet(subnetID string) (MachineSubnetPorts, error) {
	portsInSubnet, err := getOrCreateOpenedMachinePortsInSubnet(m.st, m.Id(), subnetID)
	if err != nil {
		return nil, errors.Trace(err)
	}
	return portsInSubnet, nil
}

// OpenedPorts returns a list of MachineSubnetPorts that correspond to the set
// of opened ports in each machine subnet.
func (m *Machine) OpenedPorts() ([]MachineSubnetPorts, error) {
	return getOpenedMachinePortsInAllSubnets(m.st, m.Id())
}

// OpenedPortsForAllMachines returns a list of MachineSubnetPorts that correspond to
// the set of opened ports in each subnet for all machines managed by this
// model.
func (m *Model) OpenedPortsForAllMachines() ([]MachineSubnetPorts, error) {
	openedPorts, closer := m.st.db().GetCollection(openedPortsC)
	defer closer()

	docs := []portsDoc{}
	err := openedPorts.Find(nil).All(&docs)
	if err != nil {
		return nil, errors.Trace(err)
	}
	results := make([]MachineSubnetPorts, len(docs))
	for i, doc := range docs {
		results[i] = &machineSubnetPorts{st: m.st, doc: doc}
	}
	return results, nil
}

// OpenedPortsForMachine returns a list of MachineSubnetPorts that correspond to the
// set of opened ports in each subnet for the specified machine ID.
func (m *Model) OpenedPortsForMachine(machineID string) ([]MachineSubnetPorts, error) {
	return getOpenedMachinePortsInAllSubnets(m.st, machineID)
}

// addPortsDocOps returns the ops for adding a number of port ranges
// to a new ports document. portsAssert allows specifying an assert
// statement for on the openedPorts collection op.
var addPortsDocOps = addPortsDocOpsFunc

func addPortsDocOpsFunc(st *State, pDoc *portsDoc, portsAssert interface{}, ports ...portRangeDoc) []txn.Op {
	ops := assertMachineNotDeadAndSubnetNotDeadWhenSetOps(st, pDoc)

	// As the following insert operation might be rolled back, we shouldn't
	// mutate pDoc yet; instead we need to create a copy and assign the
	// port range list to the copy.
	docCopy := new(portsDoc)
	*docCopy = *pDoc
	docCopy.Ports = ports

	return append(ops, txn.Op{
		C:      openedPortsC,
		Id:     docCopy.DocID,
		Assert: portsAssert,
		Insert: docCopy,
	})
}

// setPortsDocOps returns the ops for setting given port ranges to an
// existing ports document. portsAssert allows specifying an assert
// statement on the openedPorts collection op.
var setPortsDocOps = setPortsDocOpsFunc

func setPortsDocOpsFunc(st *State, pDoc portsDoc, portsAssert interface{}, ports ...portRangeDoc) []txn.Op {
	ops := assertMachineNotDeadAndSubnetNotDeadWhenSetOps(st, &pDoc)
	return append(ops, txn.Op{
		C:      openedPortsC,
		Id:     pDoc.DocID,
		Assert: portsAssert,
		Update: bson.D{{"$set", bson.D{{"ports", ports}}}},
	})
}

// removeOps returns the ops for removing the ports document from
// state.
func (p *machineSubnetPorts) removeOps() []txn.Op {
	return []txn.Op{{
		C:      openedPortsC,
		Id:     p.doc.DocID,
		Assert: txn.DocExists,
		Remove: true,
	}}
}

func assertMachineNotDeadAndSubnetNotDeadWhenSetOps(st *State, pDoc *portsDoc) []txn.Op {
	ops := []txn.Op{{
		C:      machinesC,
		Id:     st.docID(pDoc.MachineID),
		Assert: notDeadDoc,
	}}

	if pDoc.SubnetID != "" {
		ops = append(ops, txn.Op{
			C:      subnetsC,
			Id:     st.docID(pDoc.SubnetID),
			Assert: notDeadDoc,
		})
	}
	return ops
}

// removePortsForUnitOps returns the ops needed to remove all opened
// ports for the given unit on its assigned machine.
func removePortsForUnitOps(st *State, unit *Unit) ([]txn.Op, error) {
	machineId, err := unit.AssignedMachineId()
	if err != nil {
		// No assigned machine, so there won't be any ports.
		return nil, nil
	}
	machine, err := st.Machine(machineId)
	if errors.IsNotFound(err) {
		// Machine is removed, so there won't be a ports doc for it.
		return nil, nil
	} else if err != nil {
		return nil, errors.Trace(err)
	}

	// Get opened machine ports for all units across all subnets.
	openedPorts, err := machine.OpenedPorts()
	if err != nil {
		return nil, errors.Trace(err)
	}

	var (
		ops         []txn.Op
		removedUnit = unit.Name()
	)

	for _, openedPort := range openedPorts {
		ports := openedPort.(*machineSubnetPorts)
		var keepPorts []portRangeDoc
		for _, doc := range ports.doc.Ports {
			if doc.UnitName == removedUnit {
				continue
			}
			keepPorts = append(keepPorts, doc)
		}
		if len(keepPorts) > 0 {
			assert := bson.D{{"txn-revno", ports.doc.TxnRevno}}
			ops = append(ops, setPortsDocOps(st, ports.doc, assert, keepPorts...)...)
		} else {
			// No other ports left, remove the doc.
			ops = append(ops, ports.removeOps()...)
		}
	}
	return ops, nil
}

// getOrCreateOpenedMachinePortsInSubnet attempts to retrieve a ports document
// and returns a newly created one if it does not exist.
func getOrCreateOpenedMachinePortsInSubnet(st *State, machineID, subnetID string) (*machineSubnetPorts, error) {
	openedPorts, closer := st.db().GetCollection(openedPortsC)
	defer closer()

	var doc portsDoc
	key := portsGlobalKey(machineID, subnetID)
	err := openedPorts.FindId(key).One(&doc)
	if err != nil {
		if err != mgo.ErrNotFound {
			return nil, errors.Annotatef(err, "cannot get ports for machine %q, subnet %q", machineID, subnetID)
		}
		doc.DocID = st.docID(key)
		doc.MachineID = machineID
		doc.SubnetID = subnetID
		doc.ModelUUID = st.ModelUUID()
		return &machineSubnetPorts{st, doc, true}, nil
	}

	return &machineSubnetPorts{st, doc, false}, nil
}

// getOpenedMachinePortsInAllSubnets returns a list of MachineSubnetPorts instances
// for each subnet the machine has opened ports in.
func getOpenedMachinePortsInAllSubnets(st *State, machineID string) ([]MachineSubnetPorts, error) {
	openedPorts, closer := st.db().GetCollection(openedPortsC)
	defer closer()

	docs := []portsDoc{}
	err := openedPorts.Find(bson.D{{"machine-id", machineID}}).All(&docs)
	if err != nil {
		return nil, errors.Trace(err)
	}
	results := make([]MachineSubnetPorts, len(docs))
	for i, doc := range docs {
		results[i] = &machineSubnetPorts{st: st, doc: doc}
	}
	return results, nil
}
