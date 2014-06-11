// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package apiserver_test

import (
	"fmt"
	"reflect"
	"time"

	jc "github.com/juju/testing/checkers"
	gc "launchpad.net/gocheck"

	"github.com/juju/juju/rpc/rpcreflect"
	"github.com/juju/juju/state/apiserver"
	"github.com/juju/juju/testing"
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

type errRootSuite struct {
	testing.BaseSuite
}

var _ = gc.Suite(&errRootSuite{})

func (s *errRootSuite) TestErrorRoot(c *gc.C) {
	origErr := fmt.Errorf("my custom error")
	errRoot := apiserver.NewErrRoot(origErr)
	st, err := errRoot.Admin("")
	c.Check(st, gc.IsNil)
	c.Check(err, gc.Equals, origErr)
}

func (s *errRootSuite) TestErrorRootViaRPC(c *gc.C) {
	origErr := fmt.Errorf("my custom error")
	errRoot := apiserver.NewErrRoot(origErr)
	val := rpcreflect.ValueOf(reflect.ValueOf(errRoot))
	caller, err := val.FindMethod("Admin", 0, "Login")
	c.Assert(err, gc.IsNil)
	resp, err := caller.Call("", reflect.Value{})
	c.Check(err, gc.Equals, origErr)
	c.Check(resp.IsValid(), jc.IsFalse)
}
