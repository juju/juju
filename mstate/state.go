// The state package enables reading, observing, and changing
// the state stored in MongoDB of a whole environment
// managed by juju.
package mstate

import (
	"fmt"
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
	defer errorContextf(&err, "can't add a new machine")
	id, err := s.sequence("machine")
	if err != nil {
		return nil, err
	}
	err = s.machines.Insert(bson.D{{"_id", id}})
	if err != nil {
		return nil, err
	}
	return &Machine{st: s, id: id}, nil
}

// RemoveMachine removes the machine with the given id.
func (s *State) RemoveMachine(id int) error {
	err := s.machines.Remove(bson.D{{"_id", id}})
	if err != nil {
		return fmt.Errorf("can't remove machine %d", id)
	}
	return nil
}

// AllMachines returns all machines in the environment.
func (s *State) AllMachines() (machines []*Machine, err error) {
	mdocs := []machineDoc{}
	err = s.machines.Find(nil).Select(bson.D{{"_id", 1}}).All(&mdocs)
	if err != nil {
		return nil, fmt.Errorf("can't get all machines: %v", err)
	}
	for _, v := range mdocs {
		machines = append(machines, &Machine{st: s, id: v.Id})
	}
	return
}

// Machine returns the machine with the given id.
func (s *State) Machine(id int) (*Machine, error) {
	mdoc := &machineDoc{}
	err := s.machines.Find(bson.D{{"_id", id}}).One(mdoc)
	if err == mgo.NotFound {
		return nil, fmt.Errorf("machine %d not found", id)
	}
	if err != nil {
		return nil, fmt.Errorf("can't get machine %d: %v", id, err)
	}
	return &Machine{st: s, id: mdoc.Id}, nil
}
