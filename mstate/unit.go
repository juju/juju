package mstate

import (
	"errors"
	"fmt"
	"labix.org/v2/mgo/bson"
)

// Unit represents the state of a service unit.
type Unit struct {
	svc         *Service
	st          *State
	name        string
	serviceName string
	principal   string
}

// type unitDoc represents the internal state of a unit in MongoDB.
type unitDoc struct {
	Name        string `bson:"_id"`
	ServiceName string
	Principal   string `bson:",omitempty"`
	MachineId   *int
}

func newUnit(svc *Service, udoc *unitDoc) *Unit {
	return &Unit{
		svc:         svc,
		st:          svc.st,
		name:        udoc.Name,
		serviceName: udoc.ServiceName,
		principal:   udoc.Principal,
	}
}

// ServiceName returns the service name.
func (u *Unit) ServiceName() string {
	return u.serviceName
}

// Name returns the unit name.
func (u *Unit) Name() string {
	return u.name
}

// String returns the unit as string.
func (u *Unit) String() string {
	return u.Name()
}

// IsPrincipal returns whether the unit is deployed in its own container,
// and can therefore have subordinate services deployed alongside it.
func (u *Unit) IsPrincipal() bool {
	return u.principal == ""
}

// AssignedMachineId returns the id of the assigned machine.
func (u *Unit) AssignedMachineId() (int, error) {
	udoc, err := u.st.unitDoc(u.name)
	if err != nil {
		return 0, fmt.Errorf("can't get machine id of unit %q: %v", u, err)
	}
	if udoc.MachineId == nil {
		return 0, errors.New("unit not assigned to machine")
	}
	return *udoc.MachineId, nil
}

// AssignToMachine assigns this unit to a given machine.
func (u *Unit) AssignToMachine(m *Machine) (err error) {
	defer errorContextf(&err, "can't assign unit %q to machine %s", u, m)
	doc := bson.D{
		{"_id", u.name},
		{"$or", []bson.M{bson.M{"machineid": nil}, bson.M{"machineid": m.id}}},
	}
	change := bson.D{{"$set", bson.D{{"machineid", m.id}}}}
	err = u.st.units.Update(doc, change)
	if err != nil {
		return err
	}
	if u.principal != "" {
		return nil
	}
	// subordinate units must also be assigned to machine.
	subDocs := []unitDoc{}
	err = u.st.units.Find(bson.D{{"principal", u.name}}).All(&subDocs)
	if err != nil {
		return err
	}
	for _, doc := range subDocs {
		sunit := newUnit(u.svc, &doc)
		err = sunit.AssignToMachine(m)
		if err != nil {
			return err
		}
	}
	return nil
}
