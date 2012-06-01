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

// ReadConfigNode exports readConfigNode for testing.
func ReadConfigNode(st *State, path string) (*ConfigNode, error) {
	return readConfigNode(st.zk, path)
}

// HasRelation tests if the topology contains a relation.
func HasRelation(st *State, relation *Relation) (bool, error) {
	t, err := readTopology(st.zk)
	if err != nil {
		return false, err
	}
	_, err = t.Relation(relation.key)
	if err != nil {
		return false, err
	}
	return true, nil
}

func Diff(a, b []string) []string { return diff(a, b) }
