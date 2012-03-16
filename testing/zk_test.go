package testing_test

import (
	"fmt"
	. "launchpad.net/gocheck"
	zk "launchpad.net/gozk/zookeeper"
	"launchpad.net/juju/go/testing"
	stdtesting "testing"
)

type S struct{}

var _ = Suite(S{})

func TestT(t *stdtesting.T) {
	TestingT(t)
}

type testt func(string)

func (f testt) Fatalf(format string, args ...interface{}) {
	f(fmt.Sprintf(format, args...))
}

var allPerms = zk.WorldACL(zk.PERM_ALL)

func (S) TestStartAndClean(c *C) {
	msg := ""
	t := testt(func(s string) {
		msg = s
	})

	srv := testing.StartZkServer(t)
	c.Assert(msg, Equals, "")
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

	testing.ZkRemoveTree(t, conn, "/fdsvfdsvfds")
	c.Assert(msg, Equals, "")

	testing.ZkRemoveTree(t, conn, "/zookeeper")
	c.Assert(msg, Equals, "")

	testing.ZkRemoveTree(t, conn, "//dsafsa")
	c.Assert(msg, Not(Equals), "")
	msg = ""

	testing.ZkRemoveTree(t, conn, "/foo")
	c.Assert(msg, Equals, "")

	stat, err := conn.Exists("/foo")
	c.Assert(err, IsNil)
	c.Assert(stat, IsNil)
}
