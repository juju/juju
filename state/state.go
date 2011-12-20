// launchpad.net/juju/state
//
// Copyright (c) 2011 Canonical Ltd.

package state

// --------------------
// IMPORT
// --------------------

import (
	"errors"
	"launchpad.net/gozk/zookeeper"
)

// --------------------
// CONST
// --------------------

// --------------------
// STATE
// --------------------

// State is the entry point to get access to the states
// of the parts of the managed environmens.
type State struct {
	topology *topology
}

// Open returns a new or already created instance of
// the state.
func Open(zkc *zookeeper.Conn) (*State, error) {
	t, err := retrieveTopology(zkc)

	if err != nil {
		return nil, errors.New("state: " + err.Error())
	}

	return &State{
		topology: t,
	}, nil
}

// Service returns a service state by name.
func (s *State) Service(serviceName string) (*Service, error) {
	// Search inside topology.
	result, err := s.topology.execute(func(tn topologyNodes) (interface{}, error) {
		spath, _, serr := tn.search(func(p []string, v interface{}) bool {
			if len(p) == 3 && p[0] == "services" && p[len(p)-1] == "name" && v == serviceName {
				return true
			}

			return false
		})

		if serr != nil {
			return "", serr
		}

		return spath[1], nil
	})

	// Check the result of the command.
	if err != nil {
		return nil, err
	}

	return newService(s.topology, result.(string), serviceName), nil
}

// EOF
