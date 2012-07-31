package state

import (
	"launchpad.net/gozk/zookeeper"
)

// ZkConn returns the ZooKeeper connection used by a state.
// It is required by state_test.ConnSuite.
func ZkConn(st *State) *zookeeper.Conn {
	return st.zk
}

// NewMachine constructs *Machine's for tests.
func NewMachine(st *State, key string) *Machine {
	return newMachine(st, key)
}

// ReadConfigNode exports readConfigNode for testing.
func ReadConfigNode(st *State, path string) (*ConfigNode, error) {
	return readConfigNode(st.zk, path)
}

func Diff(a, b []string) []string { return diff(a, b) }
