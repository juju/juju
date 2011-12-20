// launchpad.net/juju/state
//
// Copyright (c) 2011 Canonical Ltd.

/*
 The state package monitor reads and monitors the
 state data stored in the passed ZooKeeper connection.
 The access to the topology is via the topology type,
 a concurrently working manager which is updated
 automatically using a gozk watch.
*/
package state

// EOF
