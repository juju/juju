package mstate

import "launchpad.net/mgo/bson"

// Machine represents the state of a machine.
type Machine struct {
	st *State
	id int
}

// machineDoc represents the internal state of a machine in MongoDB.
type machineDoc struct {
	Id         int `bson:"_id"`
	InstanceId string
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
		return "", err
	}
	return mdoc.InstanceId, nil
}

// SetInstanceId sets the provider specific machine id for this machine.
func (m *Machine) SetInstanceId(id string) error {
	return m.st.machines.Update(bson.D{{"_id", m.id}}, bson.D{{"instanceid", id}})
}
