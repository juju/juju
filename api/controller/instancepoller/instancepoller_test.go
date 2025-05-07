// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package instancepoller_test

import (
	"context"
	"errors"

	"github.com/juju/names/v6"
	"github.com/juju/tc"
	jc "github.com/juju/testing/checkers"

	"github.com/juju/juju/api/base"
	apitesting "github.com/juju/juju/api/base/testing"
	"github.com/juju/juju/api/controller/instancepoller"
	apiservertesting "github.com/juju/juju/apiserver/testing"
	"github.com/juju/juju/core/life"
	"github.com/juju/juju/core/watcher"
	coretesting "github.com/juju/juju/internal/testing"
	"github.com/juju/juju/rpc/params"
)

type InstancePollerSuite struct {
	coretesting.BaseSuite
}

var _ = tc.Suite(&InstancePollerSuite{})

func (s *InstancePollerSuite) TestNewAPI(c *tc.C) {
	apiCaller := clientErrorAPICaller(c, "Life", nil)
	api := instancepoller.NewAPI(apiCaller)
	c.Check(api, tc.NotNil)
	c.Check(apiCaller.CallCount, tc.Equals, 0)

	// Nothing happens until we actually call something else.
	m, err := api.Machine(context.Background(), names.MachineTag{})
	c.Assert(err, tc.ErrorMatches, "client error!")
	c.Assert(m, tc.IsNil)
	c.Check(apiCaller.CallCount, tc.Equals, 1)
}

func (s *InstancePollerSuite) TestNewAPIWithNilCaller(c *tc.C) {
	panicFunc := func() { instancepoller.NewAPI(nil) }
	c.Assert(panicFunc, tc.PanicMatches, "caller is nil")
}

func (s *InstancePollerSuite) TestMachineCallsLife(c *tc.C) {
	// We have tested separately the Life method, here we just check
	// it's called internally.
	expectedResults := params.LifeResults{
		Results: []params.LifeResult{{Life: "working"}},
	}
	apiCaller := successAPICaller(c, "Life", entitiesArgs, expectedResults)
	api := instancepoller.NewAPI(apiCaller)
	m, err := api.Machine(context.Background(), names.NewMachineTag("42"))
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(apiCaller.CallCount, tc.Equals, 1)
	c.Assert(m.Life(), tc.Equals, life.Value("working"))
	c.Assert(m.Id(), tc.Equals, "42")
}

func (s *InstancePollerSuite) TestWatchModelMachineStartTimesSuccess(c *tc.C) {
	// We're not testing the watcher logic here as it's already tested elsewhere.
	var numWatcherCalls int
	expectResult := params.StringsWatchResult{
		StringsWatcherId: "42",
		Changes:          []string{"foo", "bar"},
	}
	watcherFunc := func(caller base.APICaller, result params.StringsWatchResult) watcher.StringsWatcher {
		numWatcherCalls++
		c.Check(caller, tc.NotNil)
		c.Check(result, jc.DeepEquals, expectResult)
		return nil
	}
	s.PatchValue(instancepoller.NewStringsWatcher, watcherFunc)

	apiCaller := successAPICaller(c, "WatchModelMachineStartTimes", nil, expectResult)

	api := instancepoller.NewAPI(apiCaller)
	w, err := api.WatchModelMachines(context.Background())
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(apiCaller.CallCount, tc.Equals, 1)
	c.Assert(numWatcherCalls, tc.Equals, 1)
	c.Assert(w, tc.IsNil)
}

func (s *InstancePollerSuite) TestWatchModelMachineStartTimesClientError(c *tc.C) {
	apiCaller := clientErrorAPICaller(c, "WatchModelMachineStartTimes", nil)
	api := instancepoller.NewAPI(apiCaller)
	w, err := api.WatchModelMachines(context.Background())
	c.Assert(err, tc.ErrorMatches, "client error!")
	c.Assert(w, tc.IsNil)
	c.Assert(apiCaller.CallCount, tc.Equals, 1)
}

func (s *InstancePollerSuite) TestWatchModelMachineStartTimesServerError(c *tc.C) {
	expectedResults := params.StringsWatchResult{
		Error: apiservertesting.ServerError("server boom!"),
	}
	apiCaller := successAPICaller(c, "WatchModelMachineStartTimes", nil, expectedResults)

	api := instancepoller.NewAPI(apiCaller)
	w, err := api.WatchModelMachines(context.Background())
	c.Assert(err, tc.ErrorMatches, "server boom!")
	c.Assert(apiCaller.CallCount, tc.Equals, 1)
	c.Assert(w, tc.IsNil)
}

func clientErrorAPICaller(c *tc.C, method string, expectArgs interface{}) *apitesting.CallChecker {
	return apitesting.APICallChecker(c, apitesting.APICall{
		Facade:        "InstancePoller",
		VersionIsZero: true,
		IdIsEmpty:     true,
		Method:        method,
		Args:          expectArgs,
		Error:         errors.New("client error!"),
	})
}

func successAPICaller(c *tc.C, method string, expectArgs, useResults interface{}) *apitesting.CallChecker {
	return apitesting.APICallChecker(c, apitesting.APICall{
		Facade:        "InstancePoller",
		VersionIsZero: true,
		IdIsEmpty:     true,
		Method:        method,
		Args:          expectArgs,
		Results:       useResults,
	})
}
