package mstate

import (
	"fmt"
	"labix.org/v2/mgo/bson"
)

// unitDoc represents the internal state of a unit in MongoDB.
type unitDoc struct {
	Name    string `bson:"_id"`
	Service string
	UnitSet string
}

// ServiceName returns the service name.
func (u *unitDoc) ServiceName() string {
	return u.Service
}

// String returns the unit as string.
func (u *unitDoc) String() string {
	return u.Name
}

func (u *unitDoc) uSet() string {
	return u.UnitSet
}

// unitSet represents the internal MongoDB state of a principal unit and
// its subsidiaries.
type unitSet struct {
	Principal string `bson:"_id"`
	MachineId int
}

// Unit represents the state of a service unit.
type Unit struct {
	st *State
	*unitDoc
	*unitSet
}

func newUnit(s *State, udoc *unitDoc) *Unit {
	uset := &unitSet{Principal: udoc.UnitSet}
	return &Unit{
		st:      s,
		unitDoc: udoc,
		unitSet: uset,
	}
}

// Name returns the unit name.
func (u *Unit) Name() string {
	return u.unitDoc.Name
}

// IsPrincipal returns whether the unit is deployed in its own container,
// and can therefore have subordinate services deployed alongside it.
func (u *Unit) IsPrincipal() bool {
	return u.unitDoc.Name == u.unitSet.Principal
}

// AssignedMachineId returns the id of the assigned machine.
func (u *Unit) AssignedMachineId() (id int, err error) {
	us := &unitSet{}
	err = u.st.unitSets.Find(bson.D{{"_id", u.uSet()}}).One(us)
	if err != nil {
		return 0, fmt.Errorf("can't get machine id of unit %q: %v", u, err)
	}
	return us.MachineId, nil
}

// AssignToMachine assigns this unit to a given machine.
func (u *Unit) AssignToMachine(m *Machine) (err error) {
	change := bson.D{{"$set", bson.D{{"machineid", m.Id()}}}}
	err = u.st.unitSets.Update(bson.D{{"_id", u.uSet()}}, change)
	if err != nil {
		return fmt.Errorf("can't assign unit %q to machine %s: %v", u, m, err)
	}
	return nil
}
