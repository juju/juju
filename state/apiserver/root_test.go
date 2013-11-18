// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package apiserver_test

import (
	"time"

	gc "launchpad.net/gocheck"

	"launchpad.net/juju-core/rpc/rpcreflect"
	"launchpad.net/juju-core/state/apiserver"
)

type rootSuite struct{}

var _ = gc.Suite(&rootSuite{})

var allowedDiscardedMethods = []string{
	"AuthClient",
	"AuthEnvironManager",
	"AuthMachineAgent",
	"AuthOwner",
	"AuthUnitAgent",
	"GetAuthEntity",
	"GetAuthTag",
}

func (*rootSuite) TestDiscardedAPIMethods(c *gc.C) {
	t := rpcreflect.TypeOf(apiserver.RootType)
	// We must have some root-level methods.
	c.Assert(t.MethodNames(), gc.Not(gc.HasLen), 0)
	c.Assert(t.DiscardedMethods(), gc.DeepEquals, allowedDiscardedMethods)

	for _, name := range t.MethodNames() {
		m, err := t.Method(name)
		c.Assert(err, gc.IsNil)
		// We must have some methods on every object returned
		// by a root-level method.
		c.Assert(m.ObjType.MethodNames(), gc.Not(gc.HasLen), 0)
		// We don't allow any methods that don't implement
		// an RPC entry point.
		c.Assert(m.ObjType.DiscardedMethods(), gc.HasLen, 0)
	}
}

func (r *rootSuite) TestPingTimeout(c *gc.C) {
	closedc := make(chan time.Time, 1)
	action := func() error {
		closedc <- time.Now()
		return nil
	}
	timeout := apiserver.NewPingTimeout("test", action, 50*time.Millisecond)
	for i := 0; i < 10; i++ {
		time.Sleep(10 * time.Millisecond)
		timeout.Ping()
	}
	// Expect killer.killed to be set about 50ms after last ping.
	broken := time.Now()
	time.Sleep(100 * time.Millisecond)
	closed := <-closedc
	closeDiff := closed.Sub(broken).Nanoseconds() / 1000000
	c.Assert(closeDiff, gc.Equals, int64(50))
	c.Assert(closeDiff >= 50 && closeDiff <= 60, gc.Equals, true)
}
