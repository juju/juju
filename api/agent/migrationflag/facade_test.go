// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package migrationflag_test

import (
	"errors"
	"testing"

	"github.com/juju/tc"

	"github.com/juju/juju/api/agent/migrationflag"
	"github.com/juju/juju/api/base"
	basetesting "github.com/juju/juju/api/base/testing"
	"github.com/juju/juju/core/migration"
	"github.com/juju/juju/core/watcher"
	"github.com/juju/juju/internal/testhelpers"
	"github.com/juju/juju/rpc/params"
)

type FacadeSuite struct {
	testhelpers.IsolationSuite
}

func TestFacadeSuite(t *testing.T) {
	tc.Run(t, &FacadeSuite{})
}

func (*FacadeSuite) TestPhaseCallError(c *tc.C) {
	stub := &testhelpers.Stub{}
	apiCaller := apiCaller(c, stub, func(interface{}) error {
		return errors.New("bork")
	})
	facade := migrationflag.NewFacade(apiCaller, nil)

	phase, err := facade.Phase(c.Context(), someUUID)
	c.Check(err, tc.ErrorMatches, "bork")
	c.Check(phase, tc.Equals, migration.UNKNOWN)
	checkCalls(c, stub, "Phase")
}

func (*FacadeSuite) TestPhaseNoResults(c *tc.C) {
	stub := &testhelpers.Stub{}
	apiCaller := apiCaller(c, stub, func(interface{}) error {
		return nil
	})
	facade := migrationflag.NewFacade(apiCaller, nil)

	phase, err := facade.Phase(c.Context(), someUUID)
	c.Check(err, tc.ErrorMatches, "expected 1 result, got 0")
	c.Check(phase, tc.Equals, migration.UNKNOWN)
	checkCalls(c, stub, "Phase")
}

func (*FacadeSuite) TestPhaseExtraResults(c *tc.C) {
	stub := &testhelpers.Stub{}
	apiCaller := apiCaller(c, stub, func(response interface{}) error {
		outPtr, ok := response.(*params.PhaseResults)
		c.Assert(ok, tc.IsTrue)
		outPtr.Results = []params.PhaseResult{
			{Phase: "ABORT"},
			{Phase: "DONE"},
		}
		return nil
	})
	facade := migrationflag.NewFacade(apiCaller, nil)

	phase, err := facade.Phase(c.Context(), someUUID)
	c.Check(err, tc.ErrorMatches, "expected 1 result, got 2")
	c.Check(phase, tc.Equals, migration.UNKNOWN)
	checkCalls(c, stub, "Phase")
}

func (*FacadeSuite) TestPhaseError(c *tc.C) {
	stub := &testhelpers.Stub{}
	apiCaller := apiCaller(c, stub, func(response interface{}) error {
		outPtr, ok := response.(*params.PhaseResults)
		c.Assert(ok, tc.IsTrue)
		outPtr.Results = []params.PhaseResult{
			{Error: &params.Error{Message: "mneh"}},
		}
		return nil
	})
	facade := migrationflag.NewFacade(apiCaller, nil)

	phase, err := facade.Phase(c.Context(), someUUID)
	c.Check(err, tc.ErrorMatches, "mneh")
	c.Check(phase, tc.Equals, migration.UNKNOWN)
	checkCalls(c, stub, "Phase")
}

func (*FacadeSuite) TestPhaseInvalid(c *tc.C) {
	stub := &testhelpers.Stub{}
	apiCaller := apiCaller(c, stub, func(response interface{}) error {
		outPtr, ok := response.(*params.PhaseResults)
		c.Assert(ok, tc.IsTrue)
		outPtr.Results = []params.PhaseResult{{Phase: "COLLABORATE"}}
		return nil
	})
	facade := migrationflag.NewFacade(apiCaller, nil)

	phase, err := facade.Phase(c.Context(), someUUID)
	c.Check(err, tc.ErrorMatches, `unknown phase "COLLABORATE"`)
	c.Check(phase, tc.Equals, migration.UNKNOWN)
	checkCalls(c, stub, "Phase")
}

func (*FacadeSuite) TestPhaseSuccess(c *tc.C) {
	stub := &testhelpers.Stub{}
	apiCaller := apiCaller(c, stub, func(response interface{}) error {
		outPtr, ok := response.(*params.PhaseResults)
		c.Assert(ok, tc.IsTrue)
		outPtr.Results = []params.PhaseResult{{Phase: "ABORT"}}
		return nil
	})
	facade := migrationflag.NewFacade(apiCaller, nil)

	phase, err := facade.Phase(c.Context(), someUUID)
	c.Check(err, tc.ErrorIsNil)
	c.Check(phase, tc.Equals, migration.ABORT)
	checkCalls(c, stub, "Phase")
}

func (*FacadeSuite) TestWatchCallError(c *tc.C) {
	stub := &testhelpers.Stub{}
	apiCaller := apiCaller(c, stub, func(interface{}) error {
		return errors.New("bork")
	})
	facade := migrationflag.NewFacade(apiCaller, nil)

	watch, err := facade.Watch(c.Context(), someUUID)
	c.Check(err, tc.ErrorMatches, "bork")
	c.Check(watch, tc.IsNil)
	checkCalls(c, stub, "Watch")
}

func (*FacadeSuite) TestWatchNoResults(c *tc.C) {
	stub := &testhelpers.Stub{}
	apiCaller := apiCaller(c, stub, func(interface{}) error {
		return nil
	})
	facade := migrationflag.NewFacade(apiCaller, nil)

	watch, err := facade.Watch(c.Context(), someUUID)
	c.Check(err, tc.ErrorMatches, "expected 1 result, got 0")
	c.Check(watch, tc.IsNil)
	checkCalls(c, stub, "Watch")
}

func (*FacadeSuite) TestWatchExtraResults(c *tc.C) {
	stub := &testhelpers.Stub{}
	apiCaller := apiCaller(c, stub, func(response interface{}) error {
		outPtr, ok := response.(*params.NotifyWatchResults)
		c.Assert(ok, tc.IsTrue)
		outPtr.Results = []params.NotifyWatchResult{
			{NotifyWatcherId: "123"},
			{NotifyWatcherId: "456"},
		}
		return nil
	})
	facade := migrationflag.NewFacade(apiCaller, nil)

	watch, err := facade.Watch(c.Context(), someUUID)
	c.Check(err, tc.ErrorMatches, "expected 1 result, got 2")
	c.Check(watch, tc.IsNil)
	checkCalls(c, stub, "Watch")
}

func (*FacadeSuite) TestWatchError(c *tc.C) {
	stub := &testhelpers.Stub{}
	apiCaller := apiCaller(c, stub, func(response interface{}) error {
		outPtr, ok := response.(*params.NotifyWatchResults)
		c.Assert(ok, tc.IsTrue)
		outPtr.Results = []params.NotifyWatchResult{
			{Error: &params.Error{Message: "snfl"}},
		}
		return nil
	})
	facade := migrationflag.NewFacade(apiCaller, nil)

	watch, err := facade.Watch(c.Context(), someUUID)
	c.Check(err, tc.ErrorMatches, "snfl")
	c.Check(watch, tc.IsNil)
	checkCalls(c, stub, "Watch")
}

func (*FacadeSuite) TestWatchSuccess(c *tc.C) {
	stub := &testhelpers.Stub{}
	apiCaller := apiCaller(c, stub, func(response interface{}) error {
		outPtr, ok := response.(*params.NotifyWatchResults)
		c.Assert(ok, tc.IsTrue)
		outPtr.Results = []params.NotifyWatchResult{
			{NotifyWatcherId: "789"},
		}
		return nil
	})
	expectWatch := &struct{ watcher.NotifyWatcher }{}
	newWatcher := func(gotCaller base.APICaller, result params.NotifyWatchResult) watcher.NotifyWatcher {
		c.Check(gotCaller, tc.NotNil) // uncomparable
		c.Check(result, tc.DeepEquals, params.NotifyWatchResult{
			NotifyWatcherId: "789",
		})
		return expectWatch
	}
	facade := migrationflag.NewFacade(apiCaller, newWatcher)

	watch, err := facade.Watch(c.Context(), someUUID)
	c.Check(err, tc.ErrorIsNil)
	c.Check(watch, tc.Equals, expectWatch)
	checkCalls(c, stub, "Watch")
}

func apiCaller(c *tc.C, stub *testhelpers.Stub, set func(interface{}) error) base.APICaller {
	return basetesting.APICallerFunc(func(
		objType string, version int,
		id, request string,
		args, response interface{},
	) error {
		c.Check(objType, tc.Equals, "MigrationFlag")
		c.Check(version, tc.Equals, 0)
		c.Check(id, tc.Equals, "")
		stub.AddCall(request, args)
		return set(response)
	})
}

func checkCalls(c *tc.C, stub *testhelpers.Stub, names ...string) {
	stub.CheckCallNames(c, names...)
	for _, call := range stub.Calls() {
		c.Check(call.Args, tc.DeepEquals, []interface{}{
			params.Entities{
				[]params.Entity{{"model-some-uuid"}},
			},
		})
	}
}

const someUUID = "some-uuid"
