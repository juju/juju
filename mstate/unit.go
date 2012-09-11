package mstate

import (
	"errors"
	"fmt"
	"labix.org/v2/mgo"
	"labix.org/v2/mgo/txn"
	"launchpad.net/juju-core/mstate/presence"
	"launchpad.net/juju-core/trivial"
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

// NeedsUpgrade describes if a unit needs an
// upgrade and if this is forced.
type NeedsUpgrade struct {
	Upgrade bool
	Force   bool
}

// Port identifies a network port number for a particular protocol.
type Port struct {
	Protocol string `yaml:"proto"`
	Number   int    `yaml:"port"`
}

// UnitSettings holds information about a service unit's settings within a
// relation.
type UnitSettings struct {
	Version  int
	Settings map[string]interface{}
}

// unitDoc represents the internal state of a unit in MongoDB.
type unitDoc struct {
	Name           string `bson:"_id"`
	Service        string
	Principal      string
	PublicAddress  string
	PrivateAddress string
	MachineId      *int
	Resolved       ResolvedMode
	NeedsUpgrade   *NeedsUpgrade
	Life           Life
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

// Kill sets the unit lifecycle to Dying if it is Alive.
// It does nothing otherwise.
func (u *Unit) Kill() error {
	err := ensureLife(u.st, u.st.units, u.doc.Name, Dying, "unit")
	if err != nil {
		return err
	}
	u.doc.Life = Dying
	return nil
}

// Die sets the unit lifecycle to Dead if it is Alive or Dying.
// It does nothing otherwise.
func (u *Unit) Die() error {
	err := ensureLife(u.st, u.st.units, u.doc.Name, Dead, "unit")
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

// AgentAlive returns whether the respective remote agent is alive.
func (u *Unit) AgentAlive() (bool, error) {
	return u.st.pwatcher.Alive(u.globalKey())
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
		case <-u.st.pwatcher.Dying():
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
	sel := D{{"_id", u.doc.Principal}, {"life", Alive}}
	err = u.st.units.Find(sel).One(&pudoc)
	if err != nil {
		return 0, err
	}
	if pudoc.MachineId == nil {
		return 0, errors.New("unit not assigned to machine")
	}
	return *pudoc.MachineId, nil
}

// AssignToMachine assigns this unit to a given machine.
func (u *Unit) AssignToMachine(m *Machine) (err error) {
	defer trivial.ErrorContextf(&err, "cannot assign unit %q to machine %s", u, m)
	assert := D{
		{"$or", []D{
			D{{"machineid", nil}},
			D{{"machineid", m.Id()}},
		}},
		{"life", Alive},
	}
	ops := []txn.Op{{
		C:      u.st.units.Name,
		Id:     u.doc.Name,
		Assert: assert,
		Update: D{{"$set", D{{"machineid", m.Id()}}}},
	}, {
		C:      u.st.machines.Name,
		Id:     m.Id(),
		Assert: D{{"life", Alive}},
	}}
	err = u.st.runner.Run(ops, "", nil)
	if err == nil {
		u.doc.MachineId = &m.doc.Id
		return nil
	}
	err = u.Refresh()
	if err != nil {
		return err
	}
	err = m.Refresh()
	if err != nil {
		return err
	}
	switch {
	case u.doc.MachineId != nil && *u.doc.MachineId != m.Id():
		return fmt.Errorf("already assigned to machine %s", *u.doc.MachineId)
	case u.doc.Life != Alive:
		return fmt.Errorf("unit is %v", u.doc.Life)
	case m.doc.Life != Alive:
		return fmt.Errorf("machine is %v", m.doc.Life)
	default:
		panic("unreachable")
	}
	panic("unreachable")
}

// UnassignFromMachine removes the assignment between this unit and the
// machine it's assigned to.
func (u *Unit) UnassignFromMachine() (err error) {
	ops := []txn.Op{{
		C:      u.st.units.Name,
		Id:     u.doc.Name,
		Assert: txn.DocExists,
		Update: D{{"$set", D{{"machineid", nil}}}},
	}}
	err = u.st.runner.Run(ops, "", nil)
	if err != nil {
		return fmt.Errorf("cannot unassign unit %q from machine: %v", u, deadOnAbort(err))
	}
	u.doc.MachineId = nil
	return nil
}

// SetPublicAddress sets the public address of the unit.
func (u *Unit) SetPublicAddress(address string) error {
	ops := []txn.Op{{
		C:      u.st.units.Name,
		Id:     u.doc.Name,
		Assert: txn.DocExists,
		Update: D{{"$set", D{{"publicaddress", address}}}},
	}}
	err := u.st.runner.Run(ops, "", nil)
	if err != nil {
		return fmt.Errorf("cannot set public address of unit %q: %v", u, deadOnAbort(err))
	}
	u.doc.PublicAddress = address
	return nil
}

// SetPrivateAddress sets the public address of the unit.
func (u *Unit) SetPrivateAddress(address string) error {
	ops := []txn.Op{{
		C:      u.st.units.Name,
		Id:     u.doc.Name,
		Assert: txn.DocExists,
		Update: D{{"$set", D{{"privateaddress", address}}}},
	}}
	err := u.st.runner.Run(ops, "", nil)
	if err != nil {
		return fmt.Errorf("cannot set private address of unit %q: %v", u, deadOnAbort(err))
	}
	u.doc.PrivateAddress = address
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
	ops := []txn.Op{{
		C:      u.st.units.Name,
		Id:     u.doc.Name,
		Assert: D{{"resolved", ResolvedNone}},
		Update: D{{"$set", D{{"resolved", mode}}}},
	}}
	err = u.st.runner.Run(ops, "", nil)
	if err == txn.ErrAborted {
		return errors.New("flag already set")
	}
	if err != nil {
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
		return fmt.Errorf("cannot clear resolved mode for unit %q: %v", u, deadOnAbort(err))
	}
	u.doc.Resolved = ResolvedNone
	return nil
}

// NeedsUpgrade returns whether the unit needs an upgrade 
// and if it does, if this is forced.
func (u *Unit) NeedsUpgrade() (*NeedsUpgrade, error) {
	if u.doc.NeedsUpgrade == nil {
		return &NeedsUpgrade{Upgrade: false, Force: false}, nil
	}
	return u.doc.NeedsUpgrade, nil
}

// SetNeedsUpgrade informs the unit that it should perform 
// a regular or forced upgrade.
func (u *Unit) SetNeedsUpgrade(force bool) (err error) {
	defer trivial.ErrorContextf(&err, "cannot inform unit %q about upgrade", u)
	nu := &NeedsUpgrade{Upgrade: true, Force: force}
	change := D{{"$set", D{{"needsupgrade", nu}}}}
	sel := D{
		{"_id", u.doc.Name},
		{"$or", []D{
			D{{"needsupgrade", nil}},
			D{{"needsupgrade", nu}},
		}},
	}
	err = u.st.units.Update(sel, change)
	if err == mgo.ErrNotFound {
		return errors.New("upgrade already enabled")
	}
	if err != nil {
		return err
	}
	u.doc.NeedsUpgrade = nu
	return nil
}

// ClearNeedsUpgrade resets the upgrade notification. It is typically
// done by the unit agent before beginning the upgrade.
func (u *Unit) ClearNeedsUpgrade() error {
	change := D{{"$set", D{{"needsupgrade", nil}}}}
	sel := D{{"_id", u.doc.Name}}
	err := u.st.units.Update(sel, change)
	if err != nil {
		return fmt.Errorf("upgrade notification for unit %q cannot be reset: %v", u, err)
	}
	u.doc.NeedsUpgrade = nil
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
