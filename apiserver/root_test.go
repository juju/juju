// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package apiserver_test

import (
	"context"
	"fmt"
	"reflect"
	"sync"
	"time"

	"github.com/juju/clock/testclock"
	"github.com/juju/names/v6"
	"github.com/juju/tc"
	jc "github.com/juju/testing/checkers"

	"github.com/juju/juju/apiserver"
	"github.com/juju/juju/apiserver/facade"
	"github.com/juju/juju/core/pinger"
	"github.com/juju/juju/internal/rpcreflect"
	"github.com/juju/juju/internal/testing"
)

type pingSuite struct {
	testing.BaseSuite
}

var _ = tc.Suite(&pingSuite{})

func (r *pingSuite) TestPingTimeout(c *tc.C) {
	triggered := make(chan struct{})
	action := func() {
		close(triggered)
	}
	clock := testclock.NewClock(time.Now())
	timeout := pinger.NewPinger(action, clock, 50*time.Millisecond)
	for i := 0; i < 2; i++ {
		waitAlarm(c, clock)
		clock.Advance(10 * time.Millisecond)
		timeout.Ping()
	}

	waitAlarm(c, clock)
	clock.Advance(49 * time.Millisecond)
	select {
	case <-triggered:
		c.Fatalf("action triggered early")
	case <-time.After(testing.ShortWait):
	}

	clock.Advance(time.Millisecond)
	select {
	case <-triggered:
	case <-time.After(testing.LongWait):
		c.Fatalf("action never triggered")
	}
}

func (r *pingSuite) TestPingTimeoutStopped(c *tc.C) {
	triggered := make(chan struct{})
	action := func() {
		close(triggered)
	}
	clock := testclock.NewClock(time.Now())
	timeout := pinger.NewPinger(action, clock, 20*time.Millisecond)

	waitAlarm(c, clock)
	timeout.Kill()
	err := timeout.Wait()
	c.Assert(err, jc.ErrorIsNil)
	clock.Advance(time.Hour)

	// The action should never trigger
	select {
	case <-triggered:
		c.Fatalf("action triggered after Stop()")
	case <-time.After(testing.ShortWait):
	}
}

func waitAlarm(c *tc.C, clock *testclock.Clock) {
	select {
	case <-time.After(testing.LongWait):
		c.Fatalf("alarm never set")
	case <-clock.Alarms():
	}
}

type errRootSuite struct {
	testing.BaseSuite
}

var _ = tc.Suite(&errRootSuite{})

func (s *errRootSuite) TestErrorRoot(c *tc.C) {
	origErr := fmt.Errorf("my custom error")
	errRoot := apiserver.NewErrRoot(origErr)
	st, err := errRoot.FindMethod("", 0, "")
	c.Check(st, tc.IsNil)
	c.Check(err, tc.Equals, origErr)
}

type testingType struct{}

func (testingType) Exposed() error {
	return fmt.Errorf("Exposed was bogus")
}

type badType struct{}

func (badType) Exposed() error {
	return fmt.Errorf("badType.Exposed was not to be exposed")
}

type rootSuite struct {
	testing.BaseSuite
}

var _ = tc.Suite(&rootSuite{})

func (r *rootSuite) TestFindMethodUnknownFacade(c *tc.C) {
	root := apiserver.TestingAPIRoot(new(facade.Registry))
	caller, err := root.FindMethod("unknown-testing-facade", 0, "Method")
	c.Check(caller, tc.IsNil)
	c.Check(err, tc.FitsTypeOf, (*rpcreflect.CallNotImplementedError)(nil))
	c.Check(err, tc.ErrorMatches, `unknown facade type "unknown-testing-facade"`)
}

func (r *rootSuite) TestFindMethodUnknownVersion(c *tc.C) {
	registry := new(facade.Registry)
	myGoodFacade := func(context.Context, facade.ModelContext) (facade.Facade, error) {
		return &testingType{}, nil
	}
	registry.MustRegister("my-testing-facade", 0, myGoodFacade, reflect.TypeOf((*testingType)(nil)).Elem())
	srvRoot := apiserver.TestingAPIRoot(registry)
	caller, err := srvRoot.FindMethod("my-testing-facade", 1, "Exposed")
	c.Check(caller, tc.IsNil)
	c.Check(err, tc.FitsTypeOf, (*rpcreflect.CallNotImplementedError)(nil))
	c.Check(err, tc.ErrorMatches, `unknown version 1 for facade type "my-testing-facade"`)
}

func (r *rootSuite) TestFindMethodEnsuresTypeMatch(c *tc.C) {
	myBadFacade := func(context.Context, facade.ModelContext) (facade.Facade, error) {
		return &badType{}, nil
	}
	myGoodFacade := func(context.Context, facade.ModelContext) (facade.Facade, error) {
		return &testingType{}, nil
	}
	myErrFacade := func(_ context.Context, context facade.ModelContext) (facade.Facade, error) {
		return nil, fmt.Errorf("you shall not pass")
	}
	expectedType := reflect.TypeOf((*testingType)(nil))

	registry := new(facade.Registry)
	registry.Register("my-testing-facade", 0, myBadFacade, expectedType)
	registry.Register("my-testing-facade", 1, myGoodFacade, expectedType)
	registry.Register("my-testing-facade", 2, myErrFacade, expectedType)
	srvRoot := apiserver.TestingAPIRoot(registry)

	// Now, myGoodFacade returns the right type, so calling it should work
	// fine
	caller, err := srvRoot.FindMethod("my-testing-facade", 1, "Exposed")
	c.Assert(err, jc.ErrorIsNil)
	_, err = caller.Call(context.Background(), "", reflect.Value{})
	c.Check(err, tc.ErrorMatches, "Exposed was bogus")

	// However, myBadFacade returns the wrong type, so trying to access it
	// should create an error
	caller, err = srvRoot.FindMethod("my-testing-facade", 0, "Exposed")
	c.Assert(err, jc.ErrorIsNil)
	_, err = caller.Call(context.Background(), "", reflect.Value{})
	c.Check(err, tc.ErrorMatches,
		`internal error, my-testing-facade\(0\) claimed to return \*apiserver_test.testingType but returned \*apiserver_test.badType`)

	// myErrFacade had the permissions change, so calling it returns an
	// error, but that shouldn't trigger the type checking code.
	caller, err = srvRoot.FindMethod("my-testing-facade", 2, "Exposed")
	c.Assert(err, jc.ErrorIsNil)
	res, err := caller.Call(context.Background(), "", reflect.Value{})
	c.Check(err, tc.ErrorMatches, `you shall not pass`)
	c.Check(res.IsValid(), jc.IsFalse)
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

func assertCallResult(c *tc.C, caller rpcreflect.MethodCaller, id string, expected string) {
	v, err := caller.Call(context.Background(), id, reflect.Value{})
	c.Assert(err, jc.ErrorIsNil)
	c.Check(v.Interface(), tc.Equals, stringVar{expected})
}

func (r *rootSuite) TestFindMethodCachesFacades(c *tc.C) {
	registry := new(facade.Registry)
	var count int64
	newCounter := func(context.Context, facade.ModelContext) (facade.Facade, error) {
		count += 1
		return &countingType{count: count, id: ""}, nil
	}
	facadeType := reflect.TypeOf((*countingType)(nil))
	registry.MustRegister("my-counting-facade", 0, newCounter, facadeType)
	registry.MustRegister("my-counting-facade", 1, newCounter, facadeType)
	srvRoot := apiserver.TestingAPIRoot(registry)

	// The first time we call FindMethod, it should lookup a facade, and
	// request a new object.
	caller, err := srvRoot.FindMethod("my-counting-facade", 0, "Count")
	c.Assert(err, jc.ErrorIsNil)
	assertCallResult(c, caller, "", "1")
	// The second time we ask for a method on the same facade, it should
	// reuse that object, rather than creating another instance
	caller, err = srvRoot.FindMethod("my-counting-facade", 0, "AltCount")
	c.Assert(err, jc.ErrorIsNil)
	assertCallResult(c, caller, "", "ALT-1")
	// But when we ask for a different version, we should get a new
	// instance
	caller, err = srvRoot.FindMethod("my-counting-facade", 1, "Count")
	c.Assert(err, jc.ErrorIsNil)
	assertCallResult(c, caller, "", "2")
	// But it, too, should be cached
	caller, err = srvRoot.FindMethod("my-counting-facade", 1, "AltCount")
	c.Assert(err, jc.ErrorIsNil)
	assertCallResult(c, caller, "", "ALT-2")

	c.Check(count, tc.Equals, int64(2))
}

func (r *rootSuite) TestFindMethodForMultiModelCachesFacades(c *tc.C) {
	registry := new(facade.Registry)
	var count int64
	newCounter := func(context.Context, facade.MultiModelContext) (facade.Facade, error) {
		count += 1
		return &countingType{count: count, id: ""}, nil
	}
	facadeType := reflect.TypeOf((*countingType)(nil))
	registry.MustRegisterForMultiModel("my-counting-facade", 0, newCounter, facadeType)
	registry.MustRegisterForMultiModel("my-counting-facade", 1, newCounter, facadeType)
	srvRoot := apiserver.TestingAPIRoot(registry)

	// The first time we call FindMethod, it should lookup a facade, and
	// request a new object.
	caller, err := srvRoot.FindMethod("my-counting-facade", 0, "Count")
	c.Assert(err, jc.ErrorIsNil)
	assertCallResult(c, caller, "", "1")
	// The second time we ask for a method on the same facade, it should
	// reuse that object, rather than creating another instance
	caller, err = srvRoot.FindMethod("my-counting-facade", 0, "AltCount")
	c.Assert(err, jc.ErrorIsNil)
	assertCallResult(c, caller, "", "ALT-1")
	// But when we ask for a different version, we should get a new
	// instance
	caller, err = srvRoot.FindMethod("my-counting-facade", 1, "Count")
	c.Assert(err, jc.ErrorIsNil)
	assertCallResult(c, caller, "", "2")
	// But it, too, should be cached
	caller, err = srvRoot.FindMethod("my-counting-facade", 1, "AltCount")
	c.Assert(err, jc.ErrorIsNil)
	assertCallResult(c, caller, "", "ALT-2")

	c.Check(count, tc.Equals, int64(2))
}

func (r *rootSuite) TestFindMethodCachesFacadesWithId(c *tc.C) {
	var count int64
	// like newCounter, but also tracks the "id" that was requested for
	// this counter
	newIdCounter := func(_ context.Context, context facade.ModelContext) (facade.Facade, error) {
		count += 1
		return &countingType{count: count, id: context.ID()}, nil
	}
	registry := new(facade.Registry)
	reflectType := reflect.TypeOf((*countingType)(nil))
	registry.Register("my-counting-facade", 0, newIdCounter, reflectType)
	srvRoot := apiserver.TestingAPIRoot(registry)

	// The first time we call FindMethod, it should lookup a facade, and
	// request a new object.
	caller, err := srvRoot.FindMethod("my-counting-facade", 0, "Count")
	c.Assert(err, jc.ErrorIsNil)
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
	c.Assert(err, jc.ErrorIsNil)
	assertCallResult(c, caller, "orig-id", "ALT-orig-id1")
	assertCallResult(c, caller, "alt-id", "ALT-alt-id2")
	assertCallResult(c, caller, "third-id", "ALT-third-id3")
}

func (r *rootSuite) TestFindMethodCacheRaceSafe(c *tc.C) {
	var count int64
	newIdCounter := func(_ context.Context, context facade.ModelContext) (facade.Facade, error) {
		count += 1
		return &countingType{count: count, id: context.ID()}, nil
	}
	reflectType := reflect.TypeOf((*countingType)(nil))
	registry := new(facade.Registry)
	registry.Register("my-counting-facade", 0, newIdCounter, reflectType)
	srvRoot := apiserver.TestingAPIRoot(registry)

	caller, err := srvRoot.FindMethod("my-counting-facade", 0, "Count")
	c.Assert(err, jc.ErrorIsNil)
	// This is designed to trigger the race detector
	var wg sync.WaitGroup
	wg.Add(4)
	go func() { caller.Call(context.Background(), "first", reflect.Value{}); wg.Done() }()
	go func() { caller.Call(context.Background(), "second", reflect.Value{}); wg.Done() }()
	go func() { caller.Call(context.Background(), "first", reflect.Value{}); wg.Done() }()
	go func() { caller.Call(context.Background(), "second", reflect.Value{}); wg.Done() }()
	wg.Wait()
	// Once we're done, we should have only instantiated 2 different
	// objects. If we pass a different Id, we should be at 3 total count.
	assertCallResult(c, caller, "third", "third3")
}

type smallInterface interface {
	OneMethod() stringVar
}

type firstImpl struct {
}

func (*firstImpl) OneMethod() stringVar {
	return stringVar{"first"}
}

type secondImpl struct {
}

func (*secondImpl) AMethod() stringVar {
	return stringVar{"A"}
}

func (*secondImpl) ZMethod() stringVar {
	return stringVar{"Z"}
}

func (*secondImpl) OneMethod() stringVar {
	return stringVar{"second"}
}

func (r *rootSuite) TestFindMethodHandlesInterfaceTypes(c *tc.C) {
	registry := new(facade.Registry)
	registry.MustRegister("my-interface-facade", 0, func(_ context.Context, _ facade.ModelContext) (facade.Facade, error) {
		return &firstImpl{}, nil
	}, reflect.TypeOf((*smallInterface)(nil)).Elem())
	registry.MustRegister("my-interface-facade", 1, func(_ context.Context, _ facade.ModelContext) (facade.Facade, error) {
		return &secondImpl{}, nil
	}, reflect.TypeOf((*smallInterface)(nil)).Elem())
	srvRoot := apiserver.TestingAPIRoot(registry)

	caller, err := srvRoot.FindMethod("my-interface-facade", 0, "OneMethod")
	c.Assert(err, jc.ErrorIsNil)
	assertCallResult(c, caller, "", "first")
	caller2, err := srvRoot.FindMethod("my-interface-facade", 1, "OneMethod")
	c.Assert(err, jc.ErrorIsNil)
	assertCallResult(c, caller2, "", "second")
	// We should *not* be able to see AMethod or ZMethod
	caller, err = srvRoot.FindMethod("my-interface-facade", 1, "AMethod")
	c.Check(err, tc.FitsTypeOf, (*rpcreflect.CallNotImplementedError)(nil))
	c.Check(err, tc.ErrorMatches,
		`unknown method "AMethod" at version 1 for facade type "my-interface-facade"`)
	c.Check(caller, tc.IsNil)
	caller, err = srvRoot.FindMethod("my-interface-facade", 1, "ZMethod")
	c.Check(err, tc.FitsTypeOf, (*rpcreflect.CallNotImplementedError)(nil))
	c.Check(err, tc.ErrorMatches,
		`unknown method "ZMethod" at version 1 for facade type "my-interface-facade"`)
	c.Check(caller, tc.IsNil)
}

func (r *rootSuite) TestDescribeFacades(c *tc.C) {
	facades := apiserver.DescribeFacades(apiserver.AllFacades())
	c.Check(facades, tc.Not(tc.HasLen), 0)
	// As a sanity check, we should see that we have a Client v0 available
	asMap := make(map[string][]int, len(facades))
	for _, facade := range facades {
		asMap[facade.Name] = facade.Versions
	}
	clientVersions := asMap["Client"]
	c.Assert(len(clientVersions), jc.GreaterThan, 0)
	c.Check(clientVersions[0], tc.Equals, clientFacadeVersion)
}

type stubStateEntity struct{ tag names.Tag }

func (e *stubStateEntity) Tag() names.Tag { return e.tag }

func (r *rootSuite) TestAuthOwner(c *tc.C) {

	tag, err := names.ParseUnitTag("unit-postgresql-0")
	if err != nil {
		c.Errorf("error parsing unit tag for test: %s", err)
	}

	entity := &stubStateEntity{tag}

	apiHandler := apiserver.APIHandlerWithEntity(entity)
	authorized := apiHandler.AuthOwner(tag)

	c.Check(authorized, jc.IsTrue)

	incorrectTag, err := names.ParseUnitTag("unit-mysql-0")
	if err != nil {
		c.Errorf("error parsing unit tag for test: %s", err)
	}

	authorized = apiHandler.AuthOwner(incorrectTag)

	c.Check(authorized, jc.IsFalse)
}
