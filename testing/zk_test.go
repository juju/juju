package testing_test

import (
	"fmt"
	. "launchpad.net/gocheck"
	zk "launchpad.net/gozk/zookeeper"
	"launchpad.net/juju-core/testing"
	stdtesting "testing"
)

type Z struct{}

var _ = Suite(Z{})

func TestT(t *stdtesting.T) {
	TestingT(t)
}

type testt func(string)

func (f testt) Fatalf(format string, args ...interface{}) {
	f(fmt.Sprintf(format, args...))
}

var allPerms = zk.WorldACL(zk.PERM_ALL)

func (Z) TestZkStartAndClean(c *C) {
	srv := testing.StartZkServer()
	defer srv.Destroy()

	addr, err := srv.Addr()
	c.Assert(err, IsNil)

	conn, event, err := zk.Dial(addr, 5e9)
	c.Assert(err, IsNil)
	defer conn.Close()
	e := <-event
	c.Assert(e.Ok(), Equals, true)

	_, err = conn.Create("/foo", "foo", 0, allPerms)
	c.Assert(err, IsNil)
	_, err = conn.Create("/foo/bar", "bar", 0, allPerms)
	c.Assert(err, IsNil)
	_, err = conn.Create("/foo/bletch", "bar", 0, allPerms)
	c.Assert(err, IsNil)

	testing.ZkRemoveTree(conn, "/fdsvfdsvfds")

	testing.ZkRemoveTree(conn, "/zookeeper")

	c.Assert(func() { testing.ZkRemoveTree(conn, "//dsafsa") }, PanicMatches, ".+")

	testing.ZkRemoveTree(conn, "/")

	stat, err := conn.Exists("/foo")
	c.Assert(err, IsNil)
	c.Assert(stat, IsNil)
}
