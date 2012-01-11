// launchpad.net/juju/state
//
// Copyright (c) 2011-2012 Canonical Ltd.

// The state package enables reading, observing, and changing
// the state stored in ZooKeeper of a whole environment
// managed by juju.
package state

import (
	"launchpad.net/gozk/zookeeper"
)

// State represents the state of an environment
// managed by juju.
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
func (s *State) Service(name string) (*Service, error) {
	key, err := s.topology.serviceKey(name)
	if err != nil {
		return nil, err
	}
	return &Service{s.zk, key, name}, nil
}

// Unit returns a unit by name.
func (s *State) Unit(name string) (*Unit, error) {
	serviceName, _, err := parseUnitName(name)
	if err != nil {
		return nil, err
	}
	service, err := s.Service(serviceName)
	if err != nil {
		return nil, err
	}
	return service.Unit(name)
}
