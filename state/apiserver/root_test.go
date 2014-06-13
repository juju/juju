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
	"github.com/juju/juju/state"
	"github.com/juju/juju/state/apiserver"
	"github.com/juju/juju/state/apiserver/common"
	"github.com/juju/juju/testing"
)

type rootSuite struct {
	testing.BaseSuite
}

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

type testingType struct{}

func (testingType) Exposed() error {
	return fmt.Errorf("Exposed was bogus")
}

type badType struct{}

func (badType) Exposed() error {
	return fmt.Errorf("badType.Exposed was not to be exposed")
}

func (r *rootSuite) TestFindMethodUnknownFacade(c *gc.C) {
	srvRoot := apiserver.TestingSrvRoot(nil)
	caller, err := srvRoot.FindMethod("unknown-testing-facade", 0, "Method")
	c.Check(caller, gc.IsNil)
	c.Check(err, gc.FitsTypeOf, (*rpcreflect.CallNotImplementedError)(nil))
	c.Check(err, gc.ErrorMatches, "fo")
}

func (r *rootSuite) TestFindMethodEnsuresTypeMatch(c *gc.C) {
	srvRoot := apiserver.TestingSrvRoot(nil)
	defer common.Facades.Discard("my-testing-facade", 0)
	defer common.Facades.Discard("my-testing-facade", 1)
	myBadFacade := func(
		*state.State, *common.Resources, common.Authorizer, string,
	) (
		interface{}, error,
	) {
		return &badType{}, nil
	}
	myGoodFacade := func(
		*state.State, *common.Resources, common.Authorizer, string,
	) (
		interface{}, error,
	) {
		return &testingType{}, nil
	}
	expectedType := reflect.TypeOf((*testingType)(nil))
	common.RegisterFacade("my-testing-facade", 0, myBadFacade, expectedType)
	common.RegisterFacade("my-testing-facade", 1, myGoodFacade, expectedType)
	// Now, myGoodFacade returns the right type, so calling it should work
	// fine
	caller, err := srvRoot.FindMethod("my-testing-facade", 1, "Exposed")
	c.Assert(err, gc.IsNil)
	_, err = caller.Call("", reflect.Value{})
	c.Check(err, gc.ErrorMatches, "Exposed was bogus")
	// However, myBadFacade returns the wrong type, so trying to access it
	// should create an error
	caller, err = srvRoot.FindMethod("my-testing-facade", 0, "Exposed")
	c.Assert(err, gc.IsNil)
	_, err = caller.Call("", reflect.Value{})
	c.Check(err, gc.ErrorMatches,
		`internal error, my-testing-facade\(0\) claimed to return \*apiserver_test.testingType but returned \*apiserver_test.badType`)
}

type stringVar struct {
	Val string
}

type countingType struct {
	count int64
	id    string
}

func (ct *countingType) Count() stringVar {
	return stringVar{fmt.Sprintf("%s%d", ct.id, ct.count)}
}

func (ct *countingType) AltCount() stringVar {
	return stringVar{fmt.Sprintf("ALT-%s%d", ct.id, ct.count)}
}

func assertCallResult(c *gc.C, caller rpcreflect.MethodCaller, id string, expected string) {
	v, err := caller.Call(id, reflect.Value{})
	c.Assert(err, gc.IsNil)
	c.Check(v.Interface(), gc.Equals, stringVar{expected})
}

func (r *rootSuite) TestFindMethodCachesFacades(c *gc.C) {
	srvRoot := apiserver.TestingSrvRoot(nil)
	defer common.Facades.Discard("my-counting-facade", 0)
	defer common.Facades.Discard("my-counting-facade", 1)
	var count int64
	newCounter := func(
		*state.State, *common.Resources, common.Authorizer,
	) (
		*countingType, error,
	) {
		count += 1
		return &countingType{count: count, id: ""}, nil
	}
	common.RegisterStandardFacade("my-counting-facade", 0, newCounter)
	common.RegisterStandardFacade("my-counting-facade", 1, newCounter)
	// The first time we call FindMethod, it should lookup a facade, and
	// request a new object.
	caller, err := srvRoot.FindMethod("my-counting-facade", 0, "Count")
	c.Assert(err, gc.IsNil)
	assertCallResult(c, caller, "", "1")
	// The second time we ask for a method on the same facade, it should
	// reuse that object, rather than creating another instance
	caller, err = srvRoot.FindMethod("my-counting-facade", 0, "AltCount")
	c.Assert(err, gc.IsNil)
	assertCallResult(c, caller, "", "ALT-1")
	// But when we ask for a different version, we should get a new
	// instance
	caller, err = srvRoot.FindMethod("my-counting-facade", 1, "Count")
	c.Assert(err, gc.IsNil)
	assertCallResult(c, caller, "", "2")
	// But it, too, should be cached
	caller, err = srvRoot.FindMethod("my-counting-facade", 1, "AltCount")
	c.Assert(err, gc.IsNil)
	assertCallResult(c, caller, "", "ALT-2")
}

func (r *rootSuite) TestFindMethodCachesFacadesWithId(c *gc.C) {
	srvRoot := apiserver.TestingSrvRoot(nil)
	defer common.Facades.Discard("my-counting-facade", 0)
	var count int64
	// like newCounter, but also tracks the "id" that was requested for
	// this counter
	newIdCounter := func(
		_ *state.State, _ *common.Resources, _ common.Authorizer, id string,
	) (interface{}, error) {
		count += 1
		return &countingType{count: count, id: id}, nil
	}
	reflectType := reflect.TypeOf((*countingType)(nil))
	common.RegisterFacade("my-counting-facade", 0, newIdCounter, reflectType)
	// The first time we call FindMethod, it should lookup a facade, and
	// request a new object.
	caller, err := srvRoot.FindMethod("my-counting-facade", 0, "Count")
	c.Assert(err, gc.IsNil)
	assertCallResult(c, caller, "orig-id", "orig-id1")
	// However, if we place another call for a different Id, it should grab
	// a new object
	assertCallResult(c, caller, "alt-id", "alt-id2")
	// Asking for the original object gives us the cached value
	assertCallResult(c, caller, "orig-id", "orig-id1")
	// Asking for the original object gives us the cached value
	assertCallResult(c, caller, "alt-id", "alt-id2")
	// We get the same results asking for the other method
	caller, err = srvRoot.FindMethod("my-counting-facade", 0, "AltCount")
	c.Assert(err, gc.IsNil)
	assertCallResult(c, caller, "orig-id", "ALT-orig-id1")
	assertCallResult(c, caller, "alt-id", "ALT-alt-id2")
	assertCallResult(c, caller, "third-id", "ALT-third-id3")
}
