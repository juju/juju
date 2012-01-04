// launchpad.net/juju/state
//
// Copyright (c) 2011 Canonical Ltd.

// The state package enables reading, observing, and changing
// the state stored in ZooKeeper of a whole environment
// managed by juju.
package state

import (
	"launchpad.net/gozk/zookeeper"
)

// State enables reading, observing, and changing the state 
// of a whole environment managed by juju.
type State struct {
	zk       *zookeeper.Conn
	topology *topology
}

// Open returns a new State representing the environment
// being accessed through the ZooKeeper connection.
func Open(zk *zookeeper.Conn) (*State, error) {
	t, err := readTopology(zk)
	if err != nil {
		return nil, err
	}
	return &State{zk, t}, nil
}

// Service returns a service state by name.
func (s *State) Service(serviceName string) (*Service, error) {
	id, err := s.topology.serviceIdByName(serviceName)
	if err != nil {
		return nil, err
	}
	return &Service{s.zk, id, serviceName}, nil
}

// Unit returns a unit by name.
func (s *State) Unit(unitName string) (*Unit, error) {
	serviceName, _, err := parseUnitName(unitName)
	if err != nil {
		return nil, err
	}
	service, err := s.Service(serviceName)
	if err != nil {
		return nil, err
	}
	return service.Unit(unitName)
}
