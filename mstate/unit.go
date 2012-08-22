package mstate

import (
	"errors"
	"fmt"
	"labix.org/v2/mgo"
	"labix.org/v2/mgo/bson"
	"labix.org/v2/mgo/txn"
	"launchpad.net/juju-core/trivial"
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
	sel := bson.D{{"_id", u.doc.Principal}, {"life", Alive}}
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
	sel := bson.D{
		{"_id", u.doc.Name},
		{"$or", []bson.D{
			bson.D{{"machineid", nil}},
			bson.D{{"machineid", m.Id()}},
		}},
		{"life", Alive},
	}
	op := []txn.Operation{{
		Collection: u.st.units.Name,
		DocId:      u.doc.Name,
		Assert:     sel,
		Change:     bson.D{{"$set", bson.D{{"machineid", m.Id()}}}},
	}}
	err = u.st.runner.Run(op, "", nil)
	if err != nil {
		return fmt.Errorf("cannot assign unit %q to machine %s: %v", u, m, err)
	}
	u.doc.MachineId = &m.doc.Id
	return nil
}

// UnassignFromMachine removes the assignment between this unit and the
// machine it's assigned to.
func (u *Unit) UnassignFromMachine() (err error) {
	op := []txn.Operation{{
		Collection: u.st.units.Name,
		DocId:      u.doc.Name,
		Assert:     txn.DocExists,
		Change:     bson.D{{"$set", bson.D{{"machineid", nil}}}},
	}}
	err = u.st.runner.Run(op, "", nil)
	if err != nil {
		return fmt.Errorf("cannot unassign unit %q from machine: %v", u, err)
	}
	u.doc.MachineId = nil
	return nil
}

// SetPublicAddress sets the public address of the unit.
func (u *Unit) SetPublicAddress(address string) error {
	op := []txn.Operation{{
		Collection: u.st.units.Name,
		DocId:      u.doc.Name,
		Assert:     txn.DocExists,
		Change:     bson.D{{"$set", bson.D{{"publicaddress", address}}}},
	}}
	err := u.st.runner.Run(op, "", nil)
	if err != nil {
		return fmt.Errorf("cannot set public address of unit %q: %v", u, err)
	}
	u.doc.PublicAddress = address
	return nil
}

// SetPrivateAddress sets the public address of the unit.
func (u *Unit) SetPrivateAddress(address string) error {
	change := bson.D{{"$set", bson.D{{"privateaddress", address}}}}
	sel := bson.D{{"_id", u.doc.Name}}
	err := u.st.units.Update(sel, change)
	if err != nil {
		return fmt.Errorf("cannot set private address of unit %q: %v", u, err)
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
	change := bson.D{{"$set", bson.D{{"resolved", mode}}}}
	sel := bson.D{
		{"_id", u.doc.Name},
		{"resolved", ResolvedNone},
	}
	err = u.st.units.Update(sel, change)
	if err == mgo.ErrNotFound {
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
	change := bson.D{{"$set", bson.D{{"resolved", ResolvedNone}}}}
	sel := bson.D{{"_id", u.doc.Name}}
	err := u.st.units.Update(sel, change)
	if err != nil {
		return fmt.Errorf("cannot clear resolved mode for unit %q: %v", u, err)
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
	change := bson.D{{"$set", bson.D{{"needsupgrade", nu}}}}
	sel := bson.D{
		{"_id", u.doc.Name},
		{"$or", []bson.D{
			bson.D{{"needsupgrade", nil}},
			bson.D{{"needsupgrade", nu}},
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
	change := bson.D{{"$set", bson.D{{"needsupgrade", nil}}}}
	sel := bson.D{{"_id", u.doc.Name}}
	err := u.st.units.Update(sel, change)
	if err != nil {
		return fmt.Errorf("upgrade notification for unit %q cannot be reset: %v", u, err)
	}
	u.doc.NeedsUpgrade = nil
	return nil
}
