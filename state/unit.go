package state

import (
	"errors"
	"fmt"
	"labix.org/v2/mgo/txn"
	"launchpad.net/juju-core/charm"
	"launchpad.net/juju-core/log"
	"launchpad.net/juju-core/state/presence"
	"launchpad.net/juju-core/trivial"
	"sort"
	"strings"
	"time"
)

// ResolvedMode describes the way state transition errors 
// are resolved. 
type ResolvedMode int

const (
	ResolvedNone ResolvedMode = iota
	ResolvedRetryHooks
	ResolvedNoHooks
	nResolvedModes
)

// AssignmentPolicy controls what machine a unit will be assigned to.
type AssignmentPolicy string

const (
	// AssignLocal indicates that all service units should be assigned 
	// to machine 0.
	AssignLocal AssignmentPolicy = "local"
	// AssignUnused indicates that every service unit should be assigned
	// to a dedicated machine, and that new machines should be launched
	// if required.
	AssignUnused AssignmentPolicy = "unused"
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

// Port identifies a network port number for a particular protocol.
type Port struct {
	Protocol string
	Number   int
}

func (p Port) String() string {
	return fmt.Sprintf("%s:%d", p.Protocol, p.Number)
}

// UnitSettings holds information about a service unit's settings within a
// relation.
type UnitSettings struct {
	Version  int64
	Settings map[string]interface{}
}

// unitDoc represents the internal state of a unit in MongoDB.
type unitDoc struct {
	Name           string `bson:"_id"`
	Service        string
	CharmURL       *charm.URL
	Principal      string
	Subordinates   []string
	PublicAddress  string
	PrivateAddress string
	MachineId      *int
	Resolved       ResolvedMode
	Tools          *Tools `bson:",omitempty"`
	Ports          []Port
	Life           Life
	TxnRevno       int64 `bson:"txn-revno"`
}

// Unit represents the state of a service unit.
type Unit struct {
	st  *State
	doc unitDoc
}

func newUnit(st *State, udoc *unitDoc) *Unit {
	return &Unit{
		st:  st,
		doc: *udoc,
	}
}

// ServiceName returns the service name.
func (u *Unit) ServiceName() string {
	return u.doc.Service
}

// String returns the unit as string.
func (u *Unit) String() string {
	return u.doc.Name
}

// Name returns the unit name.
func (u *Unit) Name() string {
	return u.doc.Name
}

// globalKey returns the global database key for the unit.
func (u *Unit) globalKey() string {
	return "u#" + u.doc.Name
}

// Life returns whether the unit is Alive, Dying or Dead.
func (u *Unit) Life() Life {
	return u.doc.Life
}

// AgentTools returns the tools that the agent is currently running.
func (u *Unit) AgentTools() (*Tools, error) {
	if u.doc.Tools == nil {
		return &Tools{}, nil
	}
	tools := *u.doc.Tools
	return &tools, nil
}

// SetAgentTools sets the tools that the agent is currently running.
func (u *Unit) SetAgentTools(t *Tools) (err error) {
	defer trivial.ErrorContextf(&err, "cannot set agent tools for unit %q", u)
	if t.Series == "" || t.Arch == "" {
		return fmt.Errorf("empty series or arch")
	}
	ops := []txn.Op{{
		C:      u.st.units.Name,
		Id:     u.doc.Name,
		Assert: notDead,
		Update: D{{"$set", D{{"tools", t}}}},
	}}
	if err := u.st.runner.Run(ops, "", nil); err != nil {
		return onAbort(err, errNotAlive)
	}
	tools := *t
	u.doc.Tools = &tools
	return nil
}

// EnsureDying sets the unit lifecycle to Dying if it is Alive.
// It does nothing otherwise.
func (u *Unit) EnsureDying() error {
	err := ensureDying(u.st, u.st.units, u.doc.Name, "unit")
	if err != nil {
		return err
	}
	u.doc.Life = Dying
	return nil
}

// EnsureDead sets the unit lifecycle to Dead if it is Alive or Dying.
// It does nothing otherwise.
func (u *Unit) EnsureDead() error {
	err := ensureDead(u.st, u.st.units, u.doc.Name, "unit", nil, "")
	if err != nil {
		return err
	}
	u.doc.Life = Dead
	return nil
}

// Resolved returns the resolved mode for the unit.
func (u *Unit) Resolved() (mode ResolvedMode, err error) {
	return u.doc.Resolved, nil
}

// IsPrincipal returns whether the unit is deployed in its own container,
// and can therefore have subordinate services deployed alongside it.
func (u *Unit) IsPrincipal() bool {
	return u.doc.Principal == ""
}

// PublicAddress returns the public address of the unit.
func (u *Unit) PublicAddress() (string, error) {
	if u.doc.PublicAddress == "" {
		return "", fmt.Errorf("public address of unit %q not found", u)
	}
	return u.doc.PublicAddress, nil
}

// PrivateAddress returns the public address of the unit.
func (u *Unit) PrivateAddress() (string, error) {
	if u.doc.PrivateAddress == "" {
		return "", fmt.Errorf("private address of unit %q not found", u)
	}
	return u.doc.PrivateAddress, nil
}

func (u *Unit) Refresh() error {
	err := u.st.units.FindId(u.doc.Name).One(&u.doc)
	if err != nil {
		return fmt.Errorf("cannot refresh unit %q: %v", u, err)
	}
	return nil
}

// Status returns the status of the unit's agent.
func (u *Unit) Status() (s UnitStatus, info string, err error) {
	config, err := u.Config()
	if err != nil {
		return "", "", fmt.Errorf("cannot read status of unit %q: %v", u, err)
	}
	raw, found := config.Get("status")
	if !found {
		return UnitPending, "", nil
	}
	s = UnitStatus(raw.(string))
	switch s {
	case UnitError:
		// We always expect an info if status is 'error'.
		raw, found = config.Get("status-info")
		if !found {
			panic("no status-info found for unit error")
		}
		return s, raw.(string), nil
	case UnitStopped:
		return UnitStopped, "", nil
	}
	alive, err := u.AgentAlive()
	if err != nil {
		return "", "", err
	}
	if !alive {
		s = UnitDown
	}
	return s, "", nil
}

// SetStatus sets the status of the unit.
func (u *Unit) SetStatus(status UnitStatus, info string) error {
	if status == UnitPending {
		panic("unit status must not be set to pending")
	}
	config, err := u.Config()
	if err != nil {
		return err
	}
	config.Set("status", status)
	config.Set("status-info", info)
	_, err = config.Write()
	if err != nil {
		return fmt.Errorf("cannot set status of unit %q: %v", u, err)
	}
	return nil
}

// OpenPort sets the policy of the port with protocol and number to be opened.
func (u *Unit) OpenPort(protocol string, number int) (err error) {
	port := Port{Protocol: protocol, Number: number}
	defer trivial.ErrorContextf(&err, "cannot open port %v for unit %q", port, u)
	ops := []txn.Op{{
		C:      u.st.units.Name,
		Id:     u.doc.Name,
		Assert: notDead,
		Update: D{{"$addToSet", D{{"ports", port}}}},
	}}
	err = u.st.runner.Run(ops, "", nil)
	if err != nil {
		return onAbort(err, errNotAlive)
	}
	found := false
	for _, p := range u.doc.Ports {
		if p == port {
			break
		}
	}
	if !found {
		u.doc.Ports = append(u.doc.Ports, port)
	}
	return nil
}

// ClosePort sets the policy of the port with protocol and number to be closed.
func (u *Unit) ClosePort(protocol string, number int) (err error) {
	port := Port{Protocol: protocol, Number: number}
	defer trivial.ErrorContextf(&err, "cannot close port %v for unit %q", port, u)
	ops := []txn.Op{{
		C:      u.st.units.Name,
		Id:     u.doc.Name,
		Assert: notDead,
		Update: D{{"$pull", D{{"ports", port}}}},
	}}
	err = u.st.runner.Run(ops, "", nil)
	if err != nil {
		return onAbort(err, errNotAlive)
	}
	newPorts := make([]Port, 0, len(u.doc.Ports))
	for _, p := range u.doc.Ports {
		if p != port {
			newPorts = append(newPorts, p)
		}
	}
	u.doc.Ports = newPorts
	return nil
}

// OpenedPorts returns a slice containing the open ports of the unit.
func (u *Unit) OpenedPorts() []Port {
	ports := append([]Port{}, u.doc.Ports...)
	SortPorts(ports)
	return ports
}

// Charm returns the charm this unit is currently using.
func (u *Unit) Charm() (ch *Charm, err error) {
	if u.doc.CharmURL == nil {
		return nil, fmt.Errorf("charm URL of unit %q not found", u)
	}
	return u.st.Charm(u.doc.CharmURL)
}

// SetCharm marks the unit as currently using the supplied charm.
func (u *Unit) SetCharm(ch *Charm) (err error) {
	ops := []txn.Op{{
		C:      u.st.units.Name,
		Id:     u.doc.Name,
		Assert: notDead,
		Update: D{{"$set", D{{"charmurl", ch.URL()}}}},
	}}
	if err := u.st.runner.Run(ops, "", nil); err != nil {
		return fmt.Errorf("cannot set charm for unit %q: %v", u, onAbort(err, errNotAlive))
	}
	u.doc.CharmURL = ch.URL()
	return nil
}

// AgentAlive returns whether the respective remote agent is alive.
func (u *Unit) AgentAlive() (bool, error) {
	return u.st.pwatcher.Alive(u.globalKey())
}

// PathKey returns a name identifying the unit that can be used as a
// file name.  The returned key will be different from other
// PathKeys returned by any other entities from the same state.
func (u *Unit) PathKey() string {
	return "unit-" + strings.Replace(u.Name(), "/", "-", -1)
}

// WaitAgentAlive blocks until the respective agent is alive.
func (u *Unit) WaitAgentAlive(timeout time.Duration) (err error) {
	defer trivial.ErrorContextf(&err, "waiting for agent of unit %q", u)
	ch := make(chan presence.Change)
	u.st.pwatcher.Watch(u.globalKey(), ch)
	defer u.st.pwatcher.Unwatch(u.globalKey(), ch)
	for i := 0; i < 2; i++ {
		select {
		case change := <-ch:
			if change.Alive {
				return nil
			}
		case <-time.After(timeout):
			return fmt.Errorf("still not alive after timeout")
		case <-u.st.pwatcher.Dead():
			return u.st.pwatcher.Err()
		}
	}
	panic(fmt.Sprintf("presence reported dead status twice in a row for unit %q", u))
}

// SetAgentAlive signals that the agent for unit u is alive. 
// It returns the started pinger.
func (u *Unit) SetAgentAlive() (*presence.Pinger, error) {
	p := presence.NewPinger(u.st.presence, u.globalKey())
	err := p.Start()
	if err != nil {
		return nil, err
	}
	return p, nil
}

// AssignedMachineId returns the id of the assigned machine.
func (u *Unit) AssignedMachineId() (id int, err error) {
	defer trivial.ErrorContextf(&err, "cannot get machine id of unit %q", u)
	if u.IsPrincipal() {
		if u.doc.MachineId == nil {
			return 0, errors.New("unit not assigned to machine")
		}
		return *u.doc.MachineId, nil
	}
	pudoc := unitDoc{}
	err = u.st.units.Find(D{{"_id", u.doc.Principal}}).One(&pudoc)
	if err != nil {
		return 0, err
	}
	if pudoc.MachineId == nil {
		return 0, errors.New("unit not assigned to machine")
	}
	return *pudoc.MachineId, nil
}

var (
	machineDeadErr     = errors.New("machine is dead")
	unitDeadErr        = errors.New("unit is dead")
	alreadyAssignedErr = errors.New("unit is already assigned to a machine")
	inUseErr           = errors.New("machine is not unused")
)

// assignToMachine is the internal version of AssignToMachine,
// also used by AssignToUnusedMachine. It returns specific errors
// in some cases:
// - machineDeadErr when the machine is not alive.
// - unitDeadErr when the unit is not alive.
// - alreadyAssignedErr when the unit has already been assigned
// - inUseErr when the machine already has a unit assigned (if unused is true)
func (u *Unit) assignToMachine(m *Machine, unused bool) (err error) {
	if u.doc.MachineId != nil {
		if *u.doc.MachineId != m.Id() {
			return alreadyAssignedErr
		}
		return nil
	}
	if u.doc.Principal != "" {
		return fmt.Errorf("unit is a subordinate")
	}
	assert := append(isAlive, D{
		{"$or", []D{
			D{{"machineid", nil}},
			D{{"machineid", m.Id()}},
		}},
	}...)
	massert := isAlive
	if unused {
		massert = append(massert, D{{"principals", D{{"$size", 0}}}}...)
	}
	ops := []txn.Op{{
		C:      u.st.units.Name,
		Id:     u.doc.Name,
		Assert: assert,
		Update: D{{"$set", D{{"machineid", m.doc.Id}}}},
	}, {
		C:      u.st.machines.Name,
		Id:     m.doc.Id,
		Assert: massert,
		Update: D{{"$addToSet", D{{"principals", u.doc.Name}}}},
	}}
	err = u.st.runner.Run(ops, "", nil)
	if err == nil {
		u.doc.MachineId = &m.doc.Id
		return nil
	}
	if err != txn.ErrAborted {
		return err
	}
	u0, err := u.st.Unit(u.Name())
	if err != nil {
		return err
	}
	m0, err := u.st.Machine(m.Id())
	if err != nil {
		return err
	}
	switch {
	case u0.Life() != Alive:
		return unitDeadErr
	case m0.Life() != Alive:
		return machineDeadErr
	case u0.doc.MachineId != nil || !unused:
		return alreadyAssignedErr
	}
	return inUseErr
}

// AssignToMachine assigns this unit to a given machine.
func (u *Unit) AssignToMachine(m *Machine) error {
	err := u.assignToMachine(m, false)
	if err != nil {
		return fmt.Errorf("cannot assign unit %q to machine %v: %v", u, m, err)
	}
	return nil
}

var noUnusedMachines = errors.New("all machines in use")

// AssignToUnusedMachine assigns u to a machine without other units.
// If there are no unused machines besides machine 0, an error is returned.
func (u *Unit) AssignToUnusedMachine() (m *Machine, err error) {
	// Select all machines with no principals except the bootstrap machine.
	sel := D{{"principals", D{{"$size", 0}}}, {"_id", D{{"$ne", 0}}}}
	// TODO use Batch(1). See https://bugs.launchpad.net/mgo/+bug/1053509
	iter := u.st.machines.Find(sel).Batch(2).Prefetch(0).Iter()
	var mdoc machineDoc
	for iter.Next(&mdoc) {
		m := newMachine(u.st, &mdoc)

		err := u.assignToMachine(m, true)
		if err == nil {
			return m, nil
		}
		if err != inUseErr && err != machineDeadErr {
			return nil, fmt.Errorf("cannot assign unit %q to unused machine: %v", u, err)
		}
	}
	return nil, noUnusedMachines
}

// UnassignFromMachine removes the assignment between this unit and the
// machine it's assigned to.
func (u *Unit) UnassignFromMachine() (err error) {
	// TODO check local machine id and add an assert that the
	// machine id is as expected.
	ops := []txn.Op{{
		C:      u.st.units.Name,
		Id:     u.doc.Name,
		Assert: txn.DocExists,
		Update: D{{"$set", D{{"machineid", nil}}}},
	}}
	if u.doc.MachineId != nil {
		ops = append(ops, txn.Op{
			C:      u.st.machines.Name,
			Id:     u.doc.MachineId,
			Assert: txn.DocExists,
			Update: D{{"$pull", D{{"principals", u.doc.Name}}}},
		})
	}
	err = u.st.runner.Run(ops, "", nil)
	if err != nil {
		return fmt.Errorf("cannot unassign unit %q from machine: %v", u, onAbort(err, &NotFoundError{"machine"}))
	}
	u.doc.MachineId = nil
	return nil
}

// SetPublicAddress sets the public address of the unit.
func (u *Unit) SetPublicAddress(address string) (err error) {
	ops := []txn.Op{{
		C:      u.st.units.Name,
		Id:     u.doc.Name,
		Assert: txn.DocExists,
		Update: D{{"$set", D{{"publicaddress", address}}}},
	}}
	if err := u.st.runner.Run(ops, "", nil); err != nil {
		return fmt.Errorf("cannot set public address of unit %q: %v", u, onAbort(err, &NotFoundError{"machine"}))
	}
	u.doc.PublicAddress = address
	return nil
}

// SetPrivateAddress sets the private address of the unit.
func (u *Unit) SetPrivateAddress(address string) error {
	ops := []txn.Op{{
		C:      u.st.units.Name,
		Id:     u.doc.Name,
		Assert: txn.DocExists,
		Update: D{{"$set", D{{"privateaddress", address}}}},
	}}
	log.Printf("UNIT %s: RUNNING ops: %#v", u, ops)
	err := u.st.runner.Run(ops, "", nil)
	if err != nil {
		return fmt.Errorf("cannot set private address of unit %q: %v", u, &NotFoundError{"unit"})
	}
	log.Printf("UNIT %s: RAN ops", u)
	u.doc.PrivateAddress = address
	if err := u.Refresh(); err != nil {
		return err
	}
	log.Printf("UNIT %s: private address really is %s %d", u, u.doc.PrivateAddress, u.doc.TxnRevno)
	return nil
}

// SetResolved marks the unit as having had any previous state transition
// problems resolved, and informs the unit that it may attempt to
// reestablish normal workflow. The resolved mode parameter informs
// whether to attempt to reexecute previous failed hooks or to continue
// as if they had succeeded before.
func (u *Unit) SetResolved(mode ResolvedMode) (err error) {
	defer trivial.ErrorContextf(&err, "cannot set resolved mode for unit %q", u)
	if !(0 <= mode && mode < nResolvedModes) {
		return fmt.Errorf("invalid error resolution mode: %v", mode)
	}
	assert := append(isAlive, D{{"resolved", ResolvedNone}}...)
	ops := []txn.Op{{
		C:      u.st.units.Name,
		Id:     u.doc.Name,
		Assert: assert,
		Update: D{{"$set", D{{"resolved", mode}}}},
	}}
	if err := u.st.runner.Run(ops, "", nil); err != nil {
		if err == txn.ErrAborted {
			// Find which assertion failed so we can give a
			// more specific error.
			u1, err := u.st.Unit(u.Name())
			if err != nil {
				return err
			}
			if u1.Life() != Alive {
				return errNotAlive
			}
			return fmt.Errorf("already resolved")
		}
		return err
	}
	u.doc.Resolved = mode
	return nil
}

// ClearResolved removes any resolved setting on the unit.
func (u *Unit) ClearResolved() error {
	ops := []txn.Op{{
		C:      u.st.units.Name,
		Id:     u.doc.Name,
		Assert: txn.DocExists,
		Update: D{{"$set", D{{"resolved", ResolvedNone}}}},
	}}
	err := u.st.runner.Run(ops, "", nil)
	if err != nil {
		return fmt.Errorf("cannot clear resolved mode for unit %q: %v", u, &NotFoundError{"unit"})
	}
	u.doc.Resolved = ResolvedNone
	return nil
}

// Config returns the configuration node for the unit.
func (u *Unit) Config() (config *ConfigNode, err error) {
	config, err = readConfigNode(u.st, u.globalKey())
	if err != nil {
		return nil, fmt.Errorf("cannot get configuration of unit %q: %v", u, err)
	}
	return config, nil
}

type portSlice []Port

func (p portSlice) Len() int      { return len(p) }
func (p portSlice) Swap(i, j int) { p[i], p[j] = p[j], p[i] }
func (p portSlice) Less(i, j int) bool {
	p1 := p[i]
	p2 := p[j]
	if p1.Protocol != p2.Protocol {
		return p1.Protocol < p2.Protocol
	}
	return p1.Number < p2.Number
}

// SortPorts sorts the given ports, first by protocol,
// then by number.
func SortPorts(ports []Port) {
	sort.Sort(portSlice(ports))
}
