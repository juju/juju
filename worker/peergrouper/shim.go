// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package peergrouper

import (
	"net"
	"strconv"

	"labix.org/v2/mgo"

	"launchpad.net/juju-core/instance"
	"launchpad.net/juju-core/replicaset"
	"launchpad.net/juju-core/state"
)

// This file holds code that translates from State
// to the interface expected internally by the
// worker.

type stateShim struct {
	*state.State
	mongoPort int
}

func (s *stateShim) Machine(id string) (stateMachine, error) {
	m, err := s.State.Machine(id)
	if err != nil {
		return nil, err
	}
	return &machineShim{
		Machine:   m,
		mongoPort: s.mongoPort,
	}, nil
}

func (s *stateShim) MongoSession() mongoSession {
	return mongoSessionShim{s.State.MongoSession()}
}

func (m *machineShim) StateHostPort() string {
	privateAddr := instance.SelectInternalAddress(m.Addresses(), false)
	if privateAddr == "" {
		return ""
	}
	return net.JoinHostPort(privateAddr, strconv.Itoa(m.mongoPort))
}

type machineShim struct {
	*state.Machine
	mongoPort int
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
