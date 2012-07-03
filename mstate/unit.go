package mstate

import (
	"errors"
	"fmt"
	"labix.org/v2/mgo/bson"
)

// unitDoc represents the internal state of a unit in MongoDB.
type unitDoc struct {
	Name      string `bson:"_id"`
	Service   string
	Principal string
	MachineId *int
}

// ServiceName returns the service name.
func (u *unitDoc) ServiceName() string {
	return u.Service
}

// String returns the unit as string.
func (u *unitDoc) String() string {
	return u.Name
}

// Unit represents the state of a service unit.
type Unit struct {
	st *State
	unitDoc
}

func newUnit(s *State, udoc *unitDoc) *Unit {
	return &Unit{
		st:      s,
		unitDoc: *udoc,
	}
}

// Name returns the unit name.
func (u *Unit) Name() string {
	return u.unitDoc.Name
}

// IsPrincipal returns whether the unit is deployed in its own container,
// and can therefore have subordinate services deployed alongside it.
func (u *Unit) IsPrincipal() bool {
	return u.unitDoc.Name == u.unitDoc.Principal
}

// AssignedMachineId returns the id of the assigned machine.
func (u *Unit) AssignedMachineId() (id int, err error) {
	defer errorContextf(&err, "can't get machine id of unit %q", u)
	if u.IsPrincipal() {
		if u.unitDoc.MachineId == nil {
			return 0, errors.New("unit not assigned to machine")
		}
		return *u.unitDoc.MachineId, nil
	}
	pudoc := unitDoc{}
	err = u.st.units.Find(bson.D{{"_id", u.unitDoc.Principal}}).One(&pudoc)
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
	sel := bson.D{{"_id", u.unitDoc.Principal}, {"machineid", nil}}
	err = u.st.units.Update(sel, change)
	if err != nil {
		return fmt.Errorf("can't assign unit %q to machine %s: %v", u, m, err)
	}
	return nil
}
