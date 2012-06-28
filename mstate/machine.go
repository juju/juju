package mstate

import (
	"fmt"
	"labix.org/v2/mgo/bson"
	"strconv"
)

// Machine represents the state of a machine.
type Machine struct {
	st *State
	id int
}

// machineDoc represents the internal state of a machine in MongoDB.
type machineDoc struct {
	Id         int `bson:"_id"`
	InstanceId string
	UnitSet    string
}

// Id returns the machine id.
func (m *Machine) Id() int {
	return m.id
}

// InstanceId returns the provider specific machine id for this machine.
func (m *Machine) InstanceId() (string, error) {
	mdoc := &machineDoc{}
	err := m.st.machines.Find(bson.D{{"_id", m.id}}).One(mdoc)
	if err != nil {
		return "", fmt.Errorf("can't get instance id of machine %s: %v", m, err)
	}
	return mdoc.InstanceId, nil
}

// Units returns all the units that have been assigned to the machine.
func (m *Machine) Units() (units []*Unit, err error) {
	mdoc := &machineDoc{}
	err = m.st.machines.Find(bson.D{{"_id", m.id}}).One(mdoc)
	if err != nil {
		return nil, fmt.Errorf("can't get instance id of machine %s: %v", m, err)
	}
	sel := bson.D{{"unitset", mdoc.UnitSet}}
	udocs := []unitDoc{}
	err = m.st.units.Find(sel).All(&udocs)
	if err != nil {
		return nil, fmt.Errorf("can't get units assigned to machine %s: %v", m, err)
	}
	for _, udoc := range udocs {
		units = append(units, newUnit(m.st, &udoc))
	}
	return units, nil
}

// SetInstanceId sets the provider specific machine id for this machine.
func (m *Machine) SetInstanceId(id string) error {
	change := bson.D{{"$set", bson.D{{"instanceid", id}}}}
	err := m.st.machines.Update(bson.D{{"_id", m.id}}, change)
	if err != nil {
		return fmt.Errorf("can't set instance id of machine %s: %v", m, err)
	}
	return nil
}

// String returns a unique description of this machine.
func (m *Machine) String() string {
	return strconv.Itoa(m.Id())
}
