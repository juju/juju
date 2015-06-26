// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package peergrouper

import (
	"github.com/juju/replicaset"
	"gopkg.in/mgo.v2"

	"github.com/juju/juju/network"
	"github.com/juju/juju/state"
)

// This file holds code that translates from State
// to the interface expected internally by the
// worker.

type stateShim struct {
	*state.State
	mongoPort int
	apiPort   int
}

func (s *stateShim) Machine(id string) (stateMachine, error) {
	m, err := s.State.Machine(id)
	if err != nil {
		return nil, err
	}
	return &machineShim{
		Machine:   m,
		mongoPort: s.mongoPort,
		apiPort:   s.apiPort,
	}, nil
}

func (s *stateShim) MongoSession() mongoSession {
	return mongoSessionShim{s.State.MongoSession()}
}

func (m *machineShim) APIHostPorts() []network.HostPort {
	return network.AddressesWithPort(m.Addresses(), m.apiPort)
}

func (m *machineShim) MongoHostPorts() []network.HostPort {
	return network.AddressesWithPort(m.Addresses(), m.mongoPort)
}

type machineShim struct {
	*state.Machine
	mongoPort int
	apiPort   int
}

type mongoSessionShim struct {
	session *mgo.Session
}

func (s mongoSessionShim) CurrentStatus() (*replicaset.Status, error) {
	return replicaset.CurrentStatus(s.session)
}

func (s mongoSessionShim) CurrentMembers() ([]replicaset.Member, error) {
	return replicaset.CurrentMembers(s.session)
}

func (s mongoSessionShim) Set(members []replicaset.Member) error {
	return replicaset.Set(s.session, members)
}
