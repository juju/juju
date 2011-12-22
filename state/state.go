// launchpad.net/juju/state
//
// Copyright (c) 2011 Canonical Ltd.

// The state package monitor reads and monitors the
// state data stored in the passed ZooKeeper connection.
// The access to the topology is via the topology type,
// a concurrently working manager which is updated
// automatically using a gozk watch.
package state

import (
	"launchpad.net/gozk/zookeeper"
)

// State is the entry point to get access to the states
// of the parts of the managed environmens.
type State struct {
	topology *topology
}

// Open returns a new instance of the state.
func Open(zk *zookeeper.Conn) (*State, error) {
	t, err := newTopology(zk)

	if err != nil {
		return nil, err
	}

	return &State{t}, nil
}

// Service returns a service state by name.
func (s *State) Service(serviceName string) (*Service, error) {
	var svc *Service

	// Search inside topology.
	err := s.topology.execute(func(td *topologyData) error {
		for k, v := range td.Services {
			if v.Name == serviceName {
				svc = v

				svc.topology = s.topology
				svc.id = k

				return nil
			}
		}

		return ErrServiceNotFound
	})

	// Check the result of the command.
	if err != nil {
		return nil, err
	}

	return svc, nil
}

// EOF
