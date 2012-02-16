package state

import (
	. "launchpad.net/gocheck"
	"launchpad.net/gozk/zookeeper"
)

// OpenAddr connects to the single server at the given address
// and returns its State and the State's zookeeper connection.
// It is defined in export_test.go so that tests can have access to
// the underlying zookeeper connection as well as the State.
func OpenAddr(c *C, addr string) (st *State, zk *zookeeper.Conn) {
	st, err := Open(&Info{
		Addrs: []string{addr},
	})
	c.Assert(err, IsNil)
	return st, st.zk
}

// ZkRemoveTree deletes everything without checking for errors.
func ZkRemoveAll(conn *zookeeper.Conn) {
	children, _, err := conn.Children("/")
	if err == nil {
		for _, child := range children {
			zkRemoveTree(conn, "/"+child)
		}
	}
}
