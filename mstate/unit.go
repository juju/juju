package mstate

import (
	"errors"
	"fmt"
	"labix.org/v2/mgo/bson"
)

// ResolvedMode describes the way state transition errors 
// are resolved. 
type ResolvedMode int

const (
	ResolvedNone       ResolvedMode = 0
	ResolvedRetryHooks ResolvedMode = 1000
	ResolvedNoHooks    ResolvedMode = 1001
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

// unitDoc represents the internal state of a unit in MongoDB.
type unitDoc struct {
	Name      string `bson:"_id"`
	Service   string
	Principal string
	MachineId *int
	Life      Life
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

// IsPrincipal returns whether the unit is deployed in its own container,
// and can therefore have subordinate services deployed alongside it.
func (u *Unit) IsPrincipal() bool {
	return u.doc.Principal == ""
}

func (u *Unit) Refresh() error {
	doc := unitDoc{}
	err := u.st.units.FindId(u.doc.Name).One(&doc)
	if err != nil {
		return fmt.Errorf("cannot refresh unit %v: %v", u, err)
	}
	u.doc = doc
	return nil
}

// AssignedMachineId returns the id of the assigned machine.
func (u *Unit) AssignedMachineId() (id int, err error) {
	defer errorContextf(&err, "cannot get machine id of unit %q", u)
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
	change := bson.D{{"$set", bson.D{{"machineid", m.Id()}}}}
	sel := bson.D{
		{"_id", u.doc.Name},
		{"$or", []bson.D{
			bson.D{{"machineid", nil}},
			bson.D{{"machineid", m.Id()}},
		}},
	}
	err = u.st.units.Update(sel, change)
	if err != nil {
		return fmt.Errorf("cannot assign unit %q to machine %s: %v", u, m, err)
	}
	u.doc.MachineId = &m.doc.Id
	return nil
}

// UnassignFromMachine removes the assignment between this unit and the
// machine it's assigned to.
func (u *Unit) UnassignFromMachine() (err error) {
	change := bson.D{{"$set", bson.D{{"machineid", nil}}}}
	sel := bson.D{{"_id", u.doc.Name}}
	err = u.st.units.Update(sel, change)
	if err != nil {
		return fmt.Errorf("cannot unassign unit %q from machine: %v", u, err)
	}
	return nil
}
