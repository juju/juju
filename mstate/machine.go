package mstate

import "launchpad.net/mgo/bson"

// Machine represents the state of a machine.
type Machine struct {
	st *State
	id bson.ObjectId
}

// machineDoc represents the internal state of a machine in MongoDB.
type machineDoc struct {
	Id         bson.ObjectId `bson:"_id"`
	InstanceId string
}

// Id returns the machine id.
func (m *Machine) Id() string {
	return m.id.Hex()
}

// InstanceId returns the provider specific machine id for this machine.
func (m *Machine) InstanceId() (iid string, err error) {
	mdoc := &machineDoc{}
	err = m.st.machines.Find(bson.D{{"_id", m.id}}).One(mdoc)
	if err != nil {
		return
	}
	iid = mdoc.InstanceId
	return
}

// SetInstanceId sets the provider specific machine id for this machine.
func (m *Machine) SetInstanceId(id string) error {
	return m.st.machines.Update(bson.D{{"_id", m.id}}, bson.D{{"instanceid", id}})
}
