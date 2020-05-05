// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"fmt"
	"regexp"
	"strings"

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

// PortRange represents a single range of ports opened
// by one unit.
type PortRange struct {
	UnitName string
	FromPort int
	ToPort   int
	Protocol string
}

// NewPortRange create a new port range and validate it.
func NewPortRange(unitName string, fromPort, toPort int, protocol string) (PortRange, error) {
	p := PortRange{
		UnitName: unitName,
		FromPort: fromPort,
		ToPort:   toPort,
		Protocol: strings.ToLower(protocol),
	}
	if err := p.Validate(); err != nil {
		return PortRange{}, err
	}
	return p, nil
}

// Validate checks if the port range is valid.
func (p PortRange) Validate() error {
	proto := strings.ToLower(p.Protocol)
	if proto != "tcp" && proto != "udp" && proto != "icmp" {
		return errors.Errorf("invalid protocol %q", proto)
	}
	if !names.IsValidUnit(p.UnitName) {
		return errors.Errorf("invalid unit %q", p.UnitName)
	}
	if proto == "icmp" {
		if p.FromPort == p.ToPort && p.FromPort == -1 {
			return nil
		}
		return errors.Errorf(`protocol "icmp" doesn't support any ports; got "%v"`, p.FromPort)
	}
	if p.FromPort > p.ToPort {
		return errors.Errorf("invalid port range %d-%d", p.FromPort, p.ToPort)
	}
	if p.FromPort <= 0 || p.FromPort > 65535 ||
		p.ToPort <= 0 || p.ToPort > 65535 {
		return errors.Errorf("port range bounds must be between 1 and 65535, got %d-%d", p.FromPort, p.ToPort)
	}
	return nil
}

// Length returns the number of ports in the range.
// If the range is not valid, it returns 0.
func (a PortRange) Length() int {
	if err := a.Validate(); err != nil {
		// Invalid range (from > to or something equally bad)
		return 0
	}
	return (a.ToPort - a.FromPort) + 1
}

// Sanitize returns a copy of the port range, which is guaranteed to
// have FromPort >= ToPort and both FromPort and ToPort fit into the
// valid range from 1 to 65535, inclusive.
func (a PortRange) SanitizeBounds() PortRange {
	b := a
	if a.Protocol == "icmp" {
		return b
	}
	if b.FromPort > b.ToPort {
		b.FromPort, b.ToPort = b.ToPort, b.FromPort
	}
	for _, bound := range []*int{&b.FromPort, &b.ToPort} {
		switch {
		case *bound <= 0:
			*bound = 1
		case *bound > 65535:
			*bound = 65535
		}
	}
	return b
}

// CheckConflicts determines if the two port ranges conflict.
func (prA PortRange) CheckConflicts(prB PortRange) error {
	if err := prA.Validate(); err != nil {
		return err
	}
	if err := prB.Validate(); err != nil {
		return err
	}

	// An exact port range match (including the associated unit name) is not
	// considered a conflict due to the fact that many charms issue commands
	// to open the same port multiple times.
	if prA == prB {
		return nil
	}
	if prA.Protocol != prB.Protocol {
		return nil
	}
	if prA.ToPort >= prB.FromPort && prB.ToPort >= prA.FromPort {
		return errors.Errorf("port ranges %v and %v conflict", prA, prB)
	}
	return nil
}

// Strings returns the port range as a string.
func (p PortRange) String() string {
	proto := strings.ToLower(p.Protocol)
	if proto == "icmp" {
		return fmt.Sprintf("%s (%q)", proto, p.UnitName)
	}
	return fmt.Sprintf("%d-%d/%s (%q)", p.FromPort, p.ToPort, proto, p.UnitName)
}

// portsDoc represents the state of ports opened on machines for networks
type portsDoc struct {
	DocID     string      `bson:"_id"`
	ModelUUID string      `bson:"model-uuid"`
	MachineID string      `bson:"machine-id"`
	SubnetID  string      `bson:"subnet-id"`
	Ports     []PortRange `bson:"ports"`
	TxnRevno  int64       `bson:"txn-revno"`
}

// Ports represents the state of ports on a machine.
type Ports struct {
	st  *State
	doc portsDoc
	// areNew is true for documents not in state yet.
	areNew bool
}

// String returns p as a user-readable string.
func (p *Ports) String() string {
	return fmt.Sprintf("ports for machine %q, subnet %q", p.doc.MachineID, p.doc.SubnetID)
}

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

// MachineID returns the machine ID associated with this ports document.
func (p *Ports) MachineID() string {
	return p.doc.MachineID
}

// SubnetID returns the subnet ID associated with this ports document.
func (p *Ports) SubnetID() string {
	return p.doc.SubnetID
}

// OpenPorts adds the specified port range to the list of ports
// maintained by this document.
func (p *Ports) OpenPorts(portRange PortRange) error {
	modelOp, err := p.OpenClosePortsOperation([]PortRange{portRange}, nil)
	if err != nil {
		return err
	}

	return p.st.ApplyOperation(modelOp)
}

// ClosePorts removes the specified port range from the list of ports
// maintained by this document.
func (p *Ports) ClosePorts(portRange PortRange) error {
	modelOp, err := p.OpenClosePortsOperation(nil, []PortRange{portRange})
	if err != nil {
		return err
	}

	return p.st.ApplyOperation(modelOp)
}

// OpenClosePortsOperation creates a ModelOperation that opens and closes the
// requested set of PortRange arguments. Each of the PortRange arguments can be
// left empty to indicate that they should not be ignored.
func (p *Ports) OpenClosePortsOperation(openPortRanges, closePortRanges []PortRange) (ModelOperation, error) {
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
		openPortRanges:  openPortRanges,
		closePortRanges: closePortRanges,
	}, nil
}

// PortsForUnit returns the ports associated with specified unitName that are
// maintained on this document (i.e. are open on this unit's assigned machine).
func (p *Ports) PortsForUnit(unitName string) []PortRange {
	ports := []PortRange{}
	for _, port := range p.doc.Ports {
		if port.UnitName == unitName {
			ports = append(ports, port)
		}
	}
	return ports
}

// Refresh refreshes the port document from state.
func (p *Ports) Refresh() error {
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

// AllPortRanges returns a map with network.PortRange as keys and unit
// names as values.
func (p *Ports) AllPortRanges() map[network.PortRange]string {
	result := make(map[network.PortRange]string)
	for _, portRange := range p.doc.Ports {
		rawRange := network.PortRange{
			FromPort: portRange.FromPort,
			ToPort:   portRange.ToPort,
			Protocol: portRange.Protocol,
		}
		result[rawRange] = portRange.UnitName
	}
	return result
}

// Remove removes the ports document from state.
func (p *Ports) Remove() error {
	ports := &Ports{st: p.st, doc: p.doc}
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

// OpenedPorts returns this machine ports document for the given subnetID.
func (m *Machine) OpenedPorts(subnetID string) (*Ports, error) {
	ports, err := getPorts(m.st, m.Id(), subnetID)
	if err != nil && !errors.IsNotFound(err) {
		return nil, errors.Trace(err)
	}
	return ports, nil
}

// AllPorts returns all opened ports for this machine (on all
// networks).
func (m *Model) AllPorts() ([]*Ports, error) {
	openedPorts, closer := m.st.db().GetCollection(openedPortsC)
	defer closer()

	docs := []portsDoc{}
	err := openedPorts.Find(nil).All(&docs)
	if err != nil {
		return nil, errors.Trace(err)
	}
	results := make([]*Ports, len(docs))
	for i, doc := range docs {
		results[i] = &Ports{st: m.st, doc: doc}
	}
	return results, nil
}

// AllPorts returns all opened ports for this machine (on all
// networks).
func (m *Machine) AllPorts() ([]*Ports, error) {
	openedPorts, closer := m.st.db().GetCollection(openedPortsC)
	defer closer()

	docs := []portsDoc{}
	err := openedPorts.Find(bson.D{{"machine-id", m.Id()}}).All(&docs)
	if err != nil {
		return nil, errors.Trace(err)
	}
	results := make([]*Ports, len(docs))
	for i, doc := range docs {
		results[i] = &Ports{st: m.st, doc: doc}
	}
	return results, nil
}

// addPortsDocOps returns the ops for adding a number of port ranges
// to a new ports document. portsAssert allows specifying an assert
// statement for on the openedPorts collection op.
var addPortsDocOps = addPortsDocOpsFunc

func addPortsDocOpsFunc(st *State, pDoc *portsDoc, portsAssert interface{}, ports ...PortRange) []txn.Op {
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

// setPortsDocOps returns the ops for setting given port ranges to an
// existing ports document. portsAssert allows specifying an assert
// statement on the openedPorts collection op.
var setPortsDocOps = setPortsDocOpsFunc

func setPortsDocOpsFunc(st *State, pDoc portsDoc, portsAssert interface{}, ports ...PortRange) []txn.Op {
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
func (p *Ports) removeOps() []txn.Op {
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
	allPorts, err := machine.AllPorts()
	if err != nil {
		return nil, errors.Trace(err)
	}
	var ops []txn.Op
	for _, ports := range allPorts {
		allRanges := ports.AllPortRanges()
		var keepPorts []PortRange
		for portRange, unitName := range allRanges {
			if unitName != unit.Name() {
				unitRange := PortRange{
					UnitName: unitName,
					FromPort: portRange.FromPort,
					ToPort:   portRange.ToPort,
					Protocol: portRange.Protocol,
				}
				keepPorts = append(keepPorts, unitRange)
			}
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

// getPorts returns the ports document for the specified machine and subnet.
func getPorts(st *State, machineID, subnetID string) (*Ports, error) {
	openedPorts, closer := st.db().GetCollection(openedPortsC)
	defer closer()

	var doc portsDoc
	key := portsGlobalKey(machineID, subnetID)
	err := openedPorts.FindId(key).One(&doc)
	if err != nil {
		doc.MachineID = machineID
		doc.SubnetID = subnetID
		p := Ports{st, doc, false}
		if err == mgo.ErrNotFound {
			return nil, errors.NotFoundf(p.String())
		}
		return nil, errors.Annotatef(err, "cannot get %s", p.String())
	}

	return &Ports{st, doc, false}, nil
}

// getOrCreatePorts attempts to retrieve a ports document and returns a newly
// created one if it does not exist.
func getOrCreatePorts(st *State, machineID, subnetID string) (*Ports, error) {
	ports, err := getPorts(st, machineID, subnetID)
	if errors.IsNotFound(err) {
		key := portsGlobalKey(machineID, subnetID)
		doc := portsDoc{
			DocID:     st.docID(key),
			MachineID: machineID,
			SubnetID:  subnetID,
			ModelUUID: st.ModelUUID(),
		}
		ports = &Ports{st, doc, true}
	} else if err != nil {
		return nil, errors.Trace(err)
	}
	return ports, nil
}
