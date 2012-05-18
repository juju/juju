// launchpad.net/juju/state
//
// Copyright (c) 2011-2012 Canonical Ltd.
package state

import (
	"launchpad.net/gozk/zookeeper"
)

// ZkConn returns the ZooKeeper connection used by a state.
// It is defined in export_test.go so that tests can have access
// to this connection.
func ZkConn(st *State) *zookeeper.Conn {
	return st.zk
}

// NewMachine constructs *Machine's for tests.
func NewMachine(st *State, key string) *Machine {
	return &Machine{
		st:  st,
		key: key,
	}
}

func Except(a, b []string) []string { return except(a, b) }
