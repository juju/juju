// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package migrationflag_test

import (
	"errors"

	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/api/base"
	basetesting "github.com/juju/juju/api/base/testing"
	"github.com/juju/juju/api/migrationflag"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/core/migration"
	"github.com/juju/juju/watcher"
)

type FacadeSuite struct {
	testing.IsolationSuite
}

var _ = gc.Suite(&FacadeSuite{})

func (*FacadeSuite) TestPhaseCallError(c *gc.C) {
	stub := &testing.Stub{}
	apiCaller := apiCaller(c, stub, func(interface{}) error {
		return errors.New("bork")
	})
	facade := migrationflag.NewFacade(apiCaller, nil)

	phase, err := facade.Phase(someUUID)
	c.Check(err, gc.ErrorMatches, "bork")
	c.Check(phase, gc.Equals, migration.UNKNOWN)
	checkCalls(c, stub, "Phase")
}

func (*FacadeSuite) TestPhaseNoResults(c *gc.C) {
	stub := &testing.Stub{}
	apiCaller := apiCaller(c, stub, func(interface{}) error {
		return nil
	})
	facade := migrationflag.NewFacade(apiCaller, nil)

	phase, err := facade.Phase(someUUID)
	c.Check(err, gc.ErrorMatches, "expected 1 result, got 0")
	c.Check(phase, gc.Equals, migration.UNKNOWN)
	checkCalls(c, stub, "Phase")
}

func (*FacadeSuite) TestPhaseExtraResults(c *gc.C) {
	stub := &testing.Stub{}
	apiCaller := apiCaller(c, stub, func(response interface{}) error {
		outPtr, ok := response.(*params.PhaseResults)
		c.Assert(ok, jc.IsTrue)
		outPtr.Results = []params.PhaseResult{
			{Phase: "ABORT"},
			{Phase: "DONE"},
		}
		return nil
	})
	facade := migrationflag.NewFacade(apiCaller, nil)

	phase, err := facade.Phase(someUUID)
	c.Check(err, gc.ErrorMatches, "expected 1 result, got 2")
	c.Check(phase, gc.Equals, migration.UNKNOWN)
	checkCalls(c, stub, "Phase")
}

func (*FacadeSuite) TestPhaseError(c *gc.C) {
	stub := &testing.Stub{}
	apiCaller := apiCaller(c, stub, func(response interface{}) error {
		outPtr, ok := response.(*params.PhaseResults)
		c.Assert(ok, jc.IsTrue)
		outPtr.Results = []params.PhaseResult{
			{Error: &params.Error{Message: "mneh"}},
		}
		return nil
	})
	facade := migrationflag.NewFacade(apiCaller, nil)

	phase, err := facade.Phase(someUUID)
	c.Check(err, gc.ErrorMatches, "mneh")
	c.Check(phase, gc.Equals, migration.UNKNOWN)
	checkCalls(c, stub, "Phase")
}

func (*FacadeSuite) TestPhaseInvalid(c *gc.C) {
	stub := &testing.Stub{}
	apiCaller := apiCaller(c, stub, func(response interface{}) error {
		outPtr, ok := response.(*params.PhaseResults)
		c.Assert(ok, jc.IsTrue)
		outPtr.Results = []params.PhaseResult{{Phase: "COLLABORATE"}}
		return nil
	})
	facade := migrationflag.NewFacade(apiCaller, nil)

	phase, err := facade.Phase(someUUID)
	c.Check(err, gc.ErrorMatches, `unknown phase "COLLABORATE"`)
	c.Check(phase, gc.Equals, migration.UNKNOWN)
	checkCalls(c, stub, "Phase")
}

func (*FacadeSuite) TestPhaseSuccess(c *gc.C) {
	stub := &testing.Stub{}
	apiCaller := apiCaller(c, stub, func(response interface{}) error {
		outPtr, ok := response.(*params.PhaseResults)
		c.Assert(ok, jc.IsTrue)
		outPtr.Results = []params.PhaseResult{{Phase: "ABORT"}}
		return nil
	})
	facade := migrationflag.NewFacade(apiCaller, nil)

	phase, err := facade.Phase(someUUID)
	c.Check(err, jc.ErrorIsNil)
	c.Check(phase, gc.Equals, migration.ABORT)
	checkCalls(c, stub, "Phase")
}

func (*FacadeSuite) TestWatchCallError(c *gc.C) {
	stub := &testing.Stub{}
	apiCaller := apiCaller(c, stub, func(interface{}) error {
		return errors.New("bork")
	})
	facade := migrationflag.NewFacade(apiCaller, nil)

	watch, err := facade.Watch(someUUID)
	c.Check(err, gc.ErrorMatches, "bork")
	c.Check(watch, gc.IsNil)
	checkCalls(c, stub, "Watch")
}

func (*FacadeSuite) TestWatchNoResults(c *gc.C) {
	stub := &testing.Stub{}
	apiCaller := apiCaller(c, stub, func(interface{}) error {
		return nil
	})
	facade := migrationflag.NewFacade(apiCaller, nil)

	watch, err := facade.Watch(someUUID)
	c.Check(err, gc.ErrorMatches, "expected 1 result, got 0")
	c.Check(watch, gc.IsNil)
	checkCalls(c, stub, "Watch")
}

func (*FacadeSuite) TestWatchExtraResults(c *gc.C) {
	stub := &testing.Stub{}
	apiCaller := apiCaller(c, stub, func(response interface{}) error {
		outPtr, ok := response.(*params.NotifyWatchResults)
		c.Assert(ok, jc.IsTrue)
		outPtr.Results = []params.NotifyWatchResult{
			{NotifyWatcherId: "123"},
			{NotifyWatcherId: "456"},
		}
		return nil
	})
	facade := migrationflag.NewFacade(apiCaller, nil)

	watch, err := facade.Watch(someUUID)
	c.Check(err, gc.ErrorMatches, "expected 1 result, got 2")
	c.Check(watch, gc.IsNil)
	checkCalls(c, stub, "Watch")
}

func (*FacadeSuite) TestWatchError(c *gc.C) {
	stub := &testing.Stub{}
	apiCaller := apiCaller(c, stub, func(response interface{}) error {
		outPtr, ok := response.(*params.NotifyWatchResults)
		c.Assert(ok, jc.IsTrue)
		outPtr.Results = []params.NotifyWatchResult{
			{Error: &params.Error{Message: "snfl"}},
		}
		return nil
	})
	facade := migrationflag.NewFacade(apiCaller, nil)

	watch, err := facade.Watch(someUUID)
	c.Check(err, gc.ErrorMatches, "snfl")
	c.Check(watch, gc.IsNil)
	checkCalls(c, stub, "Watch")
}

func (*FacadeSuite) TestWatchSuccess(c *gc.C) {
	stub := &testing.Stub{}
	apiCaller := apiCaller(c, stub, func(response interface{}) error {
		outPtr, ok := response.(*params.NotifyWatchResults)
		c.Assert(ok, jc.IsTrue)
		outPtr.Results = []params.NotifyWatchResult{
			{NotifyWatcherId: "789"},
		}
		return nil
	})
	expectWatch := &struct{ watcher.NotifyWatcher }{}
	newWatcher := func(gotCaller base.APICaller, result params.NotifyWatchResult) watcher.NotifyWatcher {
		c.Check(gotCaller, gc.NotNil) // uncomparable
		c.Check(result, jc.DeepEquals, params.NotifyWatchResult{
			NotifyWatcherId: "789",
		})
		return expectWatch
	}
	facade := migrationflag.NewFacade(apiCaller, newWatcher)

	watch, err := facade.Watch(someUUID)
	c.Check(err, jc.ErrorIsNil)
	c.Check(watch, gc.Equals, expectWatch)
	checkCalls(c, stub, "Watch")
}

func apiCaller(c *gc.C, stub *testing.Stub, set func(interface{}) error) base.APICaller {
	return basetesting.APICallerFunc(func(
		objType string, version int,
		id, request string,
		args, response interface{},
	) error {
		c.Check(objType, gc.Equals, "MigrationFlag")
		c.Check(version, gc.Equals, 0)
		c.Check(id, gc.Equals, "")
		stub.AddCall(request, args)
		return set(response)
	})
}

func checkCalls(c *gc.C, stub *testing.Stub, names ...string) {
	stub.CheckCallNames(c, names...)
	for _, call := range stub.Calls() {
		c.Check(call.Args, jc.DeepEquals, []interface{}{
			params.Entities{
				[]params.Entity{{"model-some-uuid"}},
			},
		})
	}
}

const someUUID = "some-uuid"
