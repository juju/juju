// The state package enables reading, observing, and changing
// the state stored in MongoDB of a whole environment
// managed by juju.
package mstate

import (
	"launchpad.net/mgo"
	"launchpad.net/mgo/bson"
)

// State represents the state of an environment
// managed by juju.
type State struct {
	db       *mgo.Database
	machines *mgo.Collection
}

// AddMachine creates a new machine state.
func (s *State) AddMachine() (m *Machine, err error) {
	id := bson.NewObjectId()
	err = s.machines.Insert(bson.D{{"_id", id}})
	if err != nil {
		return
	}
	m = &Machine{st: s, id: id}
	return
}

// RemoveMachine removes the machine with the given id.
func (s *State) RemoveMachine(id string) (err error) {
	return s.machines.Remove(bson.D{{"_id", bson.ObjectIdHex(id)}})
}

// AllMachines returns all machines in the environment.
func (s *State) AllMachines() (machines []*Machine, err error) {
	mdocs := []machineDoc{}
	err = s.machines.Find(nil).Select(bson.D{{"_id", 1}}).All(&mdocs)
	if err != nil {
		return
	}
	for _, v := range mdocs {
		machines = append(machines, &Machine{st: s, id: v.Id})
	}
	return
}

// Machine returns the machine with the given id.
func (s *State) Machine(id string) (m *Machine, err error) {
	mdoc := &machineDoc{}
	err = s.machines.Find(bson.D{{"_id", bson.ObjectIdHex(id)}}).One(mdoc)
	if err != nil {
		return
	}
	m = &Machine{st: s, id: mdoc.Id}
	return
}
