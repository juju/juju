// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/juju/errors"
	"github.com/juju/loggo"
	"github.com/juju/names"
	statetxn "github.com/juju/txn"
	"gopkg.in/mgo.v2"
	"gopkg.in/mgo.v2/bson"
	"gopkg.in/mgo.v2/txn"

	"github.com/juju/juju/network"
)

var portLogger = loggo.GetLogger("juju.state.ports")

// A regular expression for parsing ports document id into
// corresponding machine and network ids.
var portsIdRe = regexp.MustCompile(fmt.Sprintf("m#(?P<machine>%s)#n#(?P<network>%s)$", names.MachineSnippet, names.NetworkSnippet))

type portIdPart int

const (
	fullId portIdPart = iota
	machineIdPart
	networkIdPart
)

// PortRange represents a single range of ports opened
// by one unit.
type PortRange struct {
	UnitName string
	FromPort int
	ToPort   int
	Protocol string
}

// NewPortRange create a new port range.
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

// Validate validates a port range and returns an error if it is invalid.
func (p PortRange) Validate() error {
	proto := strings.ToLower(p.Protocol)
	if proto != "tcp" && proto != "udp" {
		return errors.Errorf("invalid protocol %q", proto)
	}
	if !names.IsValidUnit(p.UnitName) {
		return errors.Errorf("invalid unit %q", p.UnitName)
	}
	if p.FromPort > p.ToPort {
		return errors.Errorf("invalid port range %d-%d", p.FromPort, p.ToPort)
	}
	return nil
}

// ConflictsWith determines if the two port ranges conflict.
func (a PortRange) ConflictsWith(b PortRange) bool {
	if a.Protocol != b.Protocol {
		return false
	}
	switch {
	case a.FromPort >= b.FromPort && a.FromPort <= b.ToPort:
		return true
	case a.ToPort >= b.FromPort && a.ToPort <= b.ToPort:
		return true
	case a.FromPort <= b.FromPort && a.ToPort >= b.ToPort:
		return true
	}

	return false
}

func (p PortRange) String() string {
	return fmt.Sprintf("%d-%d/%s", p.FromPort, p.ToPort, strings.ToLower(p.Protocol))
}

// portsDoc represents the state of ports opened on machines for networks
type portsDoc struct {
	Id       string `bson:"_id"`
	Ports    []PortRange
	TxnRevno int64 `bson:"txn-revno"`
}

// Ports represents the state of ports on a machine.
type Ports struct {
	st  *State
	doc portsDoc
	// indicator for documents not in state yet
	new bool
}

// Id returns the id of the ports document.
func (p *Ports) Id() string {
	return p.doc.Id
}

// Check if a port range can be opened.
func (p *Ports) canOpenPorts(newPorts PortRange) bool {
	for _, existingPorts := range p.doc.Ports {
		if existingPorts.ConflictsWith(newPorts) {
			return false
		}
	}
	return true
}

func extractPortIdPart(id string, part portIdPart) (string, error) {
	if part < 0 || part > 2 {
		return "", fmt.Errorf("invalid ports document name part: %v", part)
	}
	if parts := portsIdRe.FindStringSubmatch(id); len(parts) == 3 {
		return parts[part], nil
	}
	return "", fmt.Errorf("invalid ports document name: %v", id)
}

// PortsMachineId returns the machine id encoded in the specified ports document id.
func PortsMachineId(pDocId string) (string, error) {
	return extractPortIdPart(pDocId, machineIdPart)
}

// PortsNetworkId returns the network id encoded in the specified ports document id.
func PortsNetworkId(pDocId string) (string, error) {
	return extractPortIdPart(pDocId, networkIdPart)
}

// MachineId returns the machine id associated with this port document.
func (p *Ports) MachineId() (string, error) {
	return PortsMachineId(p.doc.Id)
}

// NetworkName returns the network name associated with this port document.
func (p *Ports) NetworkName() (string, error) {
	return PortsNetworkId(p.doc.Id)
}

// OpenPorts adds the specified port range to the ports maintained by this document.
func (p *Ports) OpenPorts(portRange PortRange) (err error) {
	defer func() {
		if err == nil {
			p.doc.Ports = append(p.doc.Ports, portRange)
		}
	}()

	if err := portRange.Validate(); err != nil {
		return err
	}
	ports := Ports{st: p.st, doc: p.doc, new: p.new}
	buildTxn := func(attempt int) ([]txn.Op, error) {
		machineId, err := ports.MachineId()
		if err != nil {
			return nil, err
		}

		if attempt > 0 {
			if err := ports.Refresh(); errors.IsNotFound(err) {
				// the ports document no longer exists
				if !ports.new {
					return nil, fmt.Errorf("ports document not found for machine %v", machineId)
				}
			} else if err != nil {
				return nil, err
			} else if ports.new {
				// the ports document was created by somebody else
				ports.new = false
			}
		}

		if !ports.canOpenPorts(portRange) {
			return nil, fmt.Errorf("cannot open ports %v on machine %v due to conflict", portRange, machineId)
		}

		// a new ports document being created
		if ports.new {
			networkName, err := ports.NetworkName()
			if err != nil {
				return nil, err
			}
			return addPortsDocOps(ports.st,
				machineId,
				networkName,
				portRange), nil
		}
		ops := []txn.Op{{
			C:      unitsC,
			Id:     portRange.UnitName,
			Assert: notDeadDoc,
		}, {
			C:      machinesC,
			Id:     machineId,
			Assert: notDeadDoc,
		}, {
			C:      openedPortsC,
			Id:     ports.Id(),
			Assert: bson.D{{"txn-revno", ports.doc.TxnRevno}},
			Update: bson.D{{"$addToSet", bson.D{{"ports", portRange}}}},
		}}
		return ops, nil
	}
	// Run the transaction using the state transaction runner.
	err = p.st.run(buildTxn)
	if err != nil {
		return err
	}
	// Mark object as created.
	p.new = false
	return nil
}

// ClosePorts removes the specified port range from this document.
func (p *Ports) ClosePorts(portRange PortRange) (err error) {
	var newPorts []PortRange
	defer func() {
		if err == nil {
			p.doc.Ports = newPorts
		}
	}()
	ports := Ports{st: p.st, doc: p.doc, new: p.new}
	buildTxn := func(attempt int) ([]txn.Op, error) {
		if attempt > 0 {
			if err := ports.Refresh(); errors.IsNotFound(err) {
				return nil, statetxn.ErrNoOperations
			} else if err != nil {
				return nil, err
			}
		}
		newPorts = newPorts[0:0]
		found := false
		for _, existingPortsDef := range ports.doc.Ports {
			if existingPortsDef == portRange {
				found = true
				continue
			} else if existingPortsDef.UnitName == portRange.UnitName && existingPortsDef.ConflictsWith(portRange) {
				return nil, fmt.Errorf("mismatched port ranges %v and %v", existingPortsDef, portRange)
			}
			newPorts = append(newPorts, existingPortsDef)
		}
		if !found {
			return nil, statetxn.ErrNoOperations
		}
		ops := []txn.Op{{
			C:      openedPortsC,
			Id:     ports.Id(),
			Assert: bson.D{{"txn-revno", ports.doc.TxnRevno}},
			Update: bson.D{{"$set", bson.D{{"ports", newPorts}}}},
		}}
		return ops, nil
	}
	return p.st.run(buildTxn)
}

// migratePorts migrates old-style unit ports collection to the ports document.
func (p *Ports) migratePorts(u *Unit) error {
	ports := Ports{st: p.st, doc: p.doc, new: p.new}
	buildTxn := func(attempt int) ([]txn.Op, error) {
		machineId, err := ports.MachineId()
		if err != nil {
			return nil, err
		}

		if attempt > 0 {
			if err := ports.Refresh(); errors.IsNotFound(err) {
				// the ports document no longer exists
				if !ports.new {
					return nil, fmt.Errorf("ports document not found for machine %v", machineId)
				}
			} else if err != nil {
				return nil, err
			} else if ports.new {
				// the ports document was created by somebody else
				ports.new = false
			}
		}

		migratedPorts := make([]PortRange, len(u.doc.Ports))
		for i, port := range u.doc.Ports {
			portDef, err := NewPortRange(u.Name(), port.Number, port.Number, port.Protocol)
			if err != nil {
				return nil, fmt.Errorf("cannot migrate port %v: %v", port, err)
			}
			if !ports.canOpenPorts(portDef) {
				return nil, fmt.Errorf("cannot migrate port %v due to conflict", port)
			}
			migratedPorts[i] = portDef
		}

		var ops []txn.Op

		// a new ports document being created
		if ports.new {
			networkName, err := ports.NetworkName()
			if err != nil {
				return nil, err
			}
			ops = addPortsDocOps(ports.st, machineId, networkName, migratedPorts...)
		} else {

			// updating existing ports document

			ops = append(ops, txn.Op{
				C:      machinesC,
				Id:     machineId,
				Assert: isAliveDoc,
			})

			for _, portDef := range migratedPorts {
				ops = append(ops, txn.Op{
					C:      openedPortsC,
					Id:     ports.Id(),
					Update: bson.D{{"$addToSet", bson.D{{"ports", portDef}}}},
				})
			}
		}
		ops = append(ops, txn.Op{
			C:      unitsC,
			Id:     u.Name(),
			Update: bson.D{{"$unset", bson.D{{"ports", nil}}}},
		})

		return ops, nil
	}
	err := p.st.run(buildTxn)
	if err != nil {
		return err
	}
	p.new = false
	return nil
}

// PortsForUnit returns the ports associated with specified unit
// that are maintained on this document (i.e. are open on this unit's
// assigned machine).
func (p *Ports) PortsForUnit(unit string) []PortRange {
	ports := []PortRange{}
	for _, port := range p.doc.Ports {
		if port.UnitName == unit {
			ports = append(ports, port)
		}
	}
	return ports
}

// Refresh refreshes the port document from state.
func (p *Ports) Refresh() error {
	openedPorts, closer := p.st.getCollection(openedPortsC)
	defer closer()

	err := openedPorts.FindId(p.Id()).One(&p.doc)
	if err == mgo.ErrNotFound {
		return errors.NotFoundf("ports document %v", p.Id())
	} else if err != nil {
		return fmt.Errorf("cannot refresh ports %v: %v", p.Id(), err)
	}
	return nil
}

// AllPortRanges returns a map from network.PortRange to unit tag
func (p *Ports) AllPortRanges() map[network.PortRange]names.UnitTag {
	result := make(map[network.PortRange]names.UnitTag)
	for _, portRange := range p.doc.Ports {
		rawRange := network.PortRange{
			FromPort: portRange.FromPort,
			ToPort:   portRange.ToPort,
			Protocol: portRange.Protocol,
		}
		if names.IsValidUnit(portRange.UnitName) {
			result[rawRange] = names.NewUnitTag(portRange.UnitName)
		} else {
			portLogger.Errorf("invalid unit name %q encountered in ports document %q", portRange.UnitName, p.Id())
		}
	}
	return result
}

// Remove removes the ports document from state.
func (p *Ports) Remove() error {
	prts := &Ports{st: p.st, doc: p.doc}
	buildTxn := func(attempt int) ([]txn.Op, error) {
		if attempt > 0 {
			err := prts.Refresh()
			if errors.IsNotFound(err) {
				return nil, statetxn.ErrNoOperations
			} else if err != nil {
				return nil, err
			}
		}
		ops := prts.removeOps()
		return ops, nil
	}
	return p.st.run(buildTxn)
}

// removeOps returns the ops for removing the ports document from mongo.
func (p *Ports) removeOps() []txn.Op {
	return []txn.Op{{
		C:      openedPortsC,
		Id:     p.Id(),
		Remove: true,
	}}
}

// OpenedPorts returns ports documents associated with specified machine.
func (m *Machine) OpenedPorts(networkId string) (*Ports, error) {
	pDocId := portsDocId(m.Id(), networkId)
	ports, err := m.st.Ports(pDocId)
	if err != nil && !errors.IsNotFound(err) {
		return nil, err
	}
	return ports, nil
}

// AllPorts returns all ports documents associated with specified machine.
func (m *Machine) AllPorts() ([]*Ports, error) {
	openedPorts, closer := m.st.getCollection(openedPortsC)
	defer closer()

	idRegex := fmt.Sprintf("m#%s#n#.*", m.Id())
	docs := []portsDoc{}
	err := openedPorts.Find(bson.M{"_id": bson.M{"$regex": idRegex}}).All(&docs)
	if err != nil {
		return nil, err
	}
	results := make([]*Ports, len(docs))
	for i, doc := range docs {
		results[i] = &Ports{st: m.st, doc: doc}
	}
	return results, nil
}

// Ports returns the ports document with the specified id
func (st *State) Ports(id string) (*Ports, error) {
	openedPorts, closer := st.getCollection(openedPortsC)
	defer closer()

	var doc portsDoc
	err := openedPorts.FindId(id).One(&doc)
	if err == mgo.ErrNotFound {
		return nil, errors.NotFoundf("ports document %v", id)
	}
	if err != nil {
		return nil, fmt.Errorf("cannot retrieve ports document %v: %v",
			id, err)
	}

	return &Ports{st, doc, false}, nil
}

// portsDocId generates the id of a ports document given the machine id and network name.
func portsDocId(machineId string, networkName string) string {
	return fmt.Sprintf("m#%s#n#%s", machineId, networkName)
}

func addPortsDocOps(st *State,
	machineId string,
	networkName string,
	ports ...PortRange) []txn.Op {

	id := portsDocId(machineId, networkName)

	ops := []txn.Op{{
		C:      machinesC,
		Id:     machineId,
		Assert: notDeadDoc,
	}, {
		C:      openedPortsC,
		Id:     id,
		Assert: txn.DocMissing,
		Insert: portsDoc{Id: id, Ports: ports},
	}}
	return ops
}

// getPorts returns the ports document for the specified
// machine and network.
func getPorts(st *State,
	machineId string,
	networkName string) (*Ports, error) {
	openedPorts, closer := st.getCollection(openedPortsC)
	defer closer()

	var doc portsDoc
	id := portsDocId(machineId, networkName)
	err := openedPorts.FindId(id).One(&doc)
	if err == mgo.ErrNotFound {
		return nil, errors.NotFoundf("ports document for machine %v", machineId)
	}
	if err != nil {
		return nil, fmt.Errorf("cannot retrieve ports document for machine %v: %v",
			machineId, err)
	}

	return &Ports{st, doc, false}, nil
}

// getOrCreatePorts attempts to retrieve a ports document
// and returns a newly created one if it does not exist.
func getOrCreatePorts(st *State, machineId string, networkName string) (*Ports, error) {
	ports, err := getPorts(st, machineId, networkName)
	if errors.IsNotFound(err) {
		doc := portsDoc{Id: portsDocId(machineId, networkName)}
		ports = &Ports{st, doc, true}
	} else if err != nil {
		return nil, err
	}
	return ports, nil
}
