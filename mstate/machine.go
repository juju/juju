package mstate

import (
	"fmt"
	"labix.org/v2/mgo/bson"
	"launchpad.net/juju-core/trivial"
	"strconv"
)

// Machine represents the state of a machine.
type Machine struct {
	st  *State
	doc machineDoc
}

// machineDoc represents the internal state of a machine in MongoDB.
type machineDoc struct {
	Id         int `bson:"_id"`
	InstanceId string
	Life       Life
}

func newMachine(st *State, doc *machineDoc) *Machine {
	return &Machine{st: st, doc: *doc}
}

// Id returns the machine id.
func (m *Machine) Id() int {
	return m.doc.Id
}

// Life returns whether the machine is Alive, Dying or Dead.
func (m *Machine) Life() Life {
	return m.doc.Life
}

// Kill sets the machine lifecycle to Dying if it is Alive.
// It does nothing otherwise.
func (m *Machine) Kill() error {
	err := ensureLife(m.doc.Id, m.st.machines, "machine", Dying)
	if err != nil {
		return err
	}
	m.doc.Life = Dying
	return nil
}

// Die sets the machine lifecycle to Dead if it is Alive or Dying.
// It does nothing otherwise.
func (m *Machine) Die() error {
	err := ensureLife(m.doc.Id, m.st.machines, "machine", Dead)
	if err != nil {
		return err
	}
	m.doc.Life = Dead
	return nil
}

func (m *Machine) Refresh() error {
	doc := machineDoc{}
	err := m.st.machines.FindId(m.doc.Id).One(&doc)
	if err != nil {
		return fmt.Errorf("cannot refresh machine %v: %v", m, err)
	}
	m.doc = doc
	return nil
}

// InstanceId returns the provider specific machine id for this machine.
func (m *Machine) InstanceId() (string, error) {
	if m.doc.InstanceId == "" {
		msg := fmt.Sprintf("instance id for machine %d is not set", m.Id())
		return "", &NotFoundError{msg}
	}
	return m.doc.InstanceId, nil
}

// Units returns all the units that have been assigned to the machine.
func (m *Machine) Units() (units []*Unit, err error) {
	defer trivial.ErrorContextf(&err, "cannot get units assigned to machine %s", m)
	pudocs := []unitDoc{}
	err = m.st.units.Find(bson.D{{"machineid", m.doc.Id}}).All(&pudocs)
	if err != nil {
		return nil, err
	}
	for _, pudoc := range pudocs {
		units = append(units, newUnit(m.st, &pudoc))
		docs := []unitDoc{}
		sel := bson.D{{"principal", pudoc.Name}, {"life", Alive}}
		err = m.st.units.Find(sel).All(&docs)
		if err != nil {
			return nil, err
		}
		for _, doc := range docs {
			units = append(units, newUnit(m.st, &doc))
		}
	}
	return units, nil
}

// SetInstanceId sets the provider specific machine id for this machine.
func (m *Machine) SetInstanceId(id string) error {
	change := bson.D{{"$set", bson.D{{"instanceid", id}}}}
	err := m.st.machines.Update(bson.D{{"_id", m.doc.Id}}, change)
	if err != nil {
		return fmt.Errorf("cannot set instance id of machine %s: %v", m, err)
	}
	m.doc.InstanceId = id
	return nil
}

// String returns a unique description of this machine.
func (m *Machine) String() string {
	return strconv.Itoa(m.doc.Id)
}
