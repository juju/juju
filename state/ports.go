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
	networkNamePart
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

// CheckConflicts determines if the two port ranges conflict.
func (a PortRange) CheckConflicts(b PortRange) error {
	if err := a.Validate(); err != nil {
		return err
	}
	if err := b.Validate(); err != nil {
		return err
	}

	// An exact port range match (including the associated unit name) is not
	// considered a conflict due to the fact that many charms issue commands
	// to open the same port multiple times.
	if a == b {
		return nil
	}
	if a.Protocol != b.Protocol {
		return nil
	}
	if a.ToPort >= b.FromPort && b.ToPort >= a.FromPort {
		return fmt.Errorf("port ranges %v (%s) and %v (%s) conflict", a, a.UnitName, b, b.UnitName)
	}
	return nil
}

// Strings returns the port range as a string.
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

// String returns p as a user-readable string.
func (p *Ports) String() string {
	// We do not check for errors here, as we assume
	// the values have been validated on construction.
	machineId, _ := p.MachineId()
	networkName, _ := p.NetworkName()
	return portsIdAsString(machineId, networkName)
}

func portsIdAsString(machineId, networkName string) string {
	return fmt.Sprintf("ports for machine %s, network %q", machineId, networkName)
}

// portsGlobalKey returns the global database key for the opened ports
// document for the given machine and network.
func portsGlobalKey(machineId string, networkName string) string {
	return fmt.Sprintf("m#%s#n#%s", machineId, networkName)
}

// Check if a port range can be opened, return the conflict error
// for more accurate reporting.
func (p *Ports) canOpenPorts(newPorts PortRange) error {
	for _, existingPorts := range p.doc.Ports {
		if err := existingPorts.CheckConflicts(newPorts); err != nil {
			return err
		}
	}
	return nil
}

func extractPortsIdPart(id string, part portIdPart) (string, error) {
	if part <= fullId || part > networkNamePart {
		return "", errors.Errorf("invalid ports document name part: %v", part)
	}
	if parts := portsIdRe.FindStringSubmatch(id); len(parts) == 3 {
		return parts[part], nil
	}
	return "", errors.Errorf("invalid ports document name: %v", id)
}

// MachineId returns the machine id associated with this ports
// document.
func (p *Ports) MachineId() (string, error) {
	return extractPortsIdPart(p.doc.Id, machineIdPart)
}

// NetworkName returns the network name associated with this ports
// document.
func (p *Ports) NetworkName() (string, error) {
	return extractPortsIdPart(p.doc.Id, networkNamePart)
}

// OpenPorts adds the specified port range to the list of ports
// maintained by this document.
func (p *Ports) OpenPorts(portRange PortRange) (err error) {
	if err = portRange.Validate(); err != nil {
		return errors.Trace(err)
	}
	ports := Ports{st: p.st, doc: p.doc, new: p.new}
	buildTxn := func(attempt int) ([]txn.Op, error) {
		var machineId string
		machineId, err = ports.MachineId()
		if err != nil {
			return nil, errors.Trace(err)
		}

		if attempt > 0 {
			if err = ports.Refresh(); errors.IsNotFound(err) {
				// No longer exists.
				if !ports.new {
					return nil, errors.Annotatef(err, "no open ports for machine %s", machineId)
				}
			} else if err != nil {
				return nil, errors.Trace(err)
			} else if ports.new {
				// Already created.
				ports.new = false
			}
		}

		if err = ports.canOpenPorts(portRange); err != nil {
			return nil, errors.Annotatef(err, "cannot open ports %v on machine %s", portRange, machineId)
		}

		if ports.new {
			// Create a new document.
			var networkName string
			networkName, err = ports.NetworkName()
			if err != nil {
				return nil, errors.Trace(err)
			}
			return addPortsDocOps(
				ports.st,
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
	if err = p.st.run(buildTxn); err != nil {
		return errors.Trace(err)
	}
	// Mark object as created.
	p.new = false
	p.doc.Ports = append(p.doc.Ports, portRange)
	return nil
}

// ClosePorts removes the specified port range from the list of ports
// maintained by this document.
func (p *Ports) ClosePorts(portRange PortRange) (err error) {
	var newPorts []PortRange

	ports := Ports{st: p.st, doc: p.doc, new: p.new}
	buildTxn := func(attempt int) ([]txn.Op, error) {
		if attempt > 0 {
			if err = ports.Refresh(); errors.IsNotFound(err) {
				// No longer exists.
				return nil, statetxn.ErrNoOperations
			} else if err != nil {
				return nil, errors.Trace(err)
			}
		}
		newPorts = newPorts[0:0]

		found := false
		for _, existingPortsDef := range ports.doc.Ports {
			if existingPortsDef == portRange {
				found = true
				continue
			}
			err = existingPortsDef.CheckConflicts(portRange)
			if existingPortsDef.UnitName == portRange.UnitName && err != nil {
				return nil, errors.Annotatef(err, "mismatched port ranges")
			}
			newPorts = append(newPorts, existingPortsDef)
		}
		if !found {
			return nil, statetxn.ErrNoOperations
		}
		ops := []txn.Op{{
			C:      unitsC,
			Id:     portRange.UnitName,
			Assert: notDeadDoc,
		}, {
			C:      openedPortsC,
			Id:     ports.Id(),
			Assert: bson.D{{"txn-revno", ports.doc.TxnRevno}},
			Update: bson.D{{"$set", bson.D{{"ports", newPorts}}}},
		}}
		return ops, nil
	}
	if err = p.st.run(buildTxn); err != nil {
		return errors.Trace(err)
	}
	p.doc.Ports = newPorts
	return nil
}

// migratePorts migrates old-style unit ports collection to the ports
// document.
//
// TODO(dimitern) Convert this to an upgrade step to run once.
func (p *Ports) migratePorts(u *Unit) error {
	ports := Ports{st: p.st, doc: p.doc, new: p.new}
	buildTxn := func(attempt int) ([]txn.Op, error) {
		machineId, err := ports.MachineId()
		if err != nil {
			return nil, errors.Trace(err)
		}

		if attempt > 0 {
			if err := ports.Refresh(); errors.IsNotFound(err) {
				// No longer exists.
				if !ports.new {
					return nil, errors.Annotatef(err, "no open ports for machine %s", machineId)
				}
			} else if err != nil {
				return nil, errors.Trace(err)
			} else if ports.new {
				// Already created.
				ports.new = false
			}
		}

		migratedPorts := make([]PortRange, len(u.doc.Ports))
		for i, port := range u.doc.Ports {
			portDef, err := NewPortRange(u.Name(), port.Number, port.Number, port.Protocol)
			if err != nil {
				return nil, errors.Annotatef(err, "cannot migrate port %v", port)
			}
			if err := ports.canOpenPorts(portDef); err != nil {
				return nil, errors.Annotatef(err, "cannot migrate port %v", port)
			}
			migratedPorts[i] = portDef
		}

		var ops []txn.Op

		if ports.new {
			// Create a new document.
			networkName, err := ports.NetworkName()
			if err != nil {
				return nil, errors.Trace(err)
			}
			ops = addPortsDocOps(
				ports.st,
				machineId,
				networkName,
				migratedPorts...)
		} else {
			// Updating an existing document.
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

		// TODO(domas) 2014-07-04 bug #1337813: Clear the old port
		// collection on the unit document once the firewaller no
		// longer depends on the unit ports list.
		return ops, nil
	}
	if err := p.st.run(buildTxn); err != nil {
		return errors.Trace(err)
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
	prts := &Ports{st: p.st, doc: p.doc}
	buildTxn := func(attempt int) ([]txn.Op, error) {
		if attempt > 0 {
			err := prts.Refresh()
			if errors.IsNotFound(err) {
				return nil, statetxn.ErrNoOperations
			} else if err != nil {
				return nil, errors.Trace(err)
			}
		}
		ops := prts.removeOps()
		return ops, nil
	}
	return p.st.run(buildTxn)
}

// removeOps returns the ops for removing the ports document from
// state.
func (p *Ports) removeOps() []txn.Op {
	return []txn.Op{{
		C:      openedPortsC,
		Id:     p.Id(),
		Remove: true,
	}}
}

// OpenedPorts returns this machine ports document for the given
// network.
func (m *Machine) OpenedPorts(networkName string) (*Ports, error) {
	key := portsGlobalKey(m.Id(), networkName)
	ports, err := m.st.Ports(key)
	if err != nil && !errors.IsNotFound(err) {
		return nil, errors.Trace(err)
	}
	return ports, nil
}

// AllPorts returns all opened ports for this machine (on all
// networks).
func (m *Machine) AllPorts() ([]*Ports, error) {
	openedPorts, closer := m.st.getCollection(openedPortsC)
	defer closer()

	idRegex := fmt.Sprintf("m#%s#n#.*", m.Id())
	docs := []portsDoc{}
	query := bson.M{"_id": bson.M{"$regex": idRegex}}
	err := openedPorts.Find(query).All(&docs)
	if err != nil {
		return nil, errors.Trace(err)
	}
	results := make([]*Ports, len(docs))
	for i, doc := range docs {
		results[i] = &Ports{st: m.st, doc: doc}
	}
	return results, nil
}

// Ports returns the opened ports document with the given global key.
func (st *State) Ports(key string) (*Ports, error) {
	openedPorts, closer := st.getCollection(openedPortsC)
	defer closer()

	machineId, err := extractPortsIdPart(key, machineIdPart)
	if err != nil {
		return nil, errors.Trace(err)
	}
	networkName, err := extractPortsIdPart(key, networkNamePart)
	if err != nil {
		return nil, errors.Trace(err)
	}
	portsId := portsIdAsString(machineId, networkName)

	var doc portsDoc
	err = openedPorts.FindId(key).One(&doc)
	if err == mgo.ErrNotFound {
		return nil, errors.NotFoundf(portsId)
	}
	if err != nil {
		return nil, errors.Annotatef(err, "cannot get %s", portsId)
	}

	return &Ports{st, doc, false}, nil
}

func addPortsDocOps(st *State, machineId, networkName string, ports ...PortRange) []txn.Op {
	key := portsGlobalKey(machineId, networkName)
	pdoc := &portsDoc{
		Id:    key,
		Ports: ports,
	}
	return []txn.Op{{
		C:      machinesC,
		Id:     machineId,
		Assert: notDeadDoc,
	}, {
		C:      openedPortsC,
		Id:     key,
		Assert: txn.DocMissing,
		Insert: pdoc,
	}}
}

// getPorts returns the ports document for the specified machine and
// network.
func getPorts(st *State, machineId, networkName string) (*Ports, error) {
	openedPorts, closer := st.getCollection(openedPortsC)
	defer closer()

	var doc portsDoc
	key := portsGlobalKey(machineId, networkName)
	stringId := portsIdAsString(machineId, networkName)
	err := openedPorts.FindId(key).One(&doc)
	if err == mgo.ErrNotFound {
		return nil, errors.NotFoundf(stringId)
	}
	if err != nil {
		return nil, errors.Annotatef(err, "cannot get %s", stringId)
	}

	return &Ports{st, doc, false}, nil
}

// getOrCreatePorts attempts to retrieve a ports document and returns
// a newly created one if it does not exist.
func getOrCreatePorts(st *State, machineId, networkName string) (*Ports, error) {
	ports, err := getPorts(st, machineId, networkName)
	if errors.IsNotFound(err) {
		key := portsGlobalKey(machineId, networkName)
		doc := portsDoc{Id: key}
		ports = &Ports{st, doc, true}
	} else if err != nil {
		return nil, errors.Trace(err)
	}
	return ports, nil
}
