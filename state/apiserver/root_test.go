// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package apiserver_test

import (
	"time"

	gc "launchpad.net/gocheck"

	"launchpad.net/juju-core/rpc/rpcreflect"
	"launchpad.net/juju-core/state/apiserver"
	"launchpad.net/juju-core/testing"
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
	action := func() {
		closedc <- time.Now()
	}
	timeout := apiserver.NewPingTimeout(action, 50*time.Millisecond)
	for i := 0; i < 2; i++ {
		time.Sleep(10 * time.Millisecond)
		timeout.Ping()
	}
	// Expect action to be executed about 50ms after last ping.
	broken := time.Now()
	var closed time.Time
	select {
	case closed = <-closedc:
	case <-time.After(testing.LongWait):
		c.Fatalf("action never executed")
	}
	closeDiff := closed.Sub(broken) / time.Millisecond
	c.Assert(50 <= closeDiff && closeDiff <= 100, gc.Equals, true)
}

func (r *rootSuite) TestPingTimeoutStopped(c *gc.C) {
	closedc := make(chan time.Time, 1)
	action := func() {
		closedc <- time.Now()
	}
	timeout := apiserver.NewPingTimeout(action, 20*time.Millisecond)
	timeout.Ping()
	timeout.Stop()
	// The action should never trigger
	select {
	case <-closedc:
		c.Fatalf("action triggered after Stop()")
	case <-time.After(testing.ShortWait):
	}
}
