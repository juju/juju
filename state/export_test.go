package state

import (
	. "launchpad.net/gocheck"
	"launchpad.net/gozk/zookeeper"
)

func OpenAddr(c *C, addr string) (st *State, zk *zookeeper.Conn) {
	st, err := Open(&Info{
		Addrs: []string{addr},
	})
	c.Assert(err, IsNil)
	return st, st.zk
}
