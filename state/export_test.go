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

// ZkConn returns the ZooKeeper connection used by a state.
// It is defined in export_test.go so that tests can have access
// to this connection.
func ZkConn(st *State) *zookeeper.Conn {
	return st.zk
}
