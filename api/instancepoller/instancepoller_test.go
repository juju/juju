// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package instancepoller_test

import (
	"errors"

	"github.com/juju/names"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/api/base"
	apitesting "github.com/juju/juju/api/base/testing"
	"github.com/juju/juju/api/instancepoller"
	"github.com/juju/juju/api/watcher"
	"github.com/juju/juju/apiserver/params"
	apiservertesting "github.com/juju/juju/apiserver/testing"
	coretesting "github.com/juju/juju/testing"
)

type InstancePollerSuite struct {
	coretesting.BaseSuite
}

var _ = gc.Suite(&InstancePollerSuite{})

func (s *InstancePollerSuite) TestNewAPI(c *gc.C) {
	var called int
	apiCaller := clientErrorAPICaller(c, "Life", nil, &called)
	api := instancepoller.NewAPI(apiCaller)
	c.Check(api, gc.NotNil)
	c.Check(called, gc.Equals, 0)

	// Nothing happens until we actually call something else.
	m, err := api.Machine(names.MachineTag{})
	c.Assert(err, gc.ErrorMatches, "client error!")
	c.Assert(m, gc.IsNil)
	c.Assert(called, gc.Equals, 1)
}

func (s *InstancePollerSuite) TestNewAPIWithNilCaller(c *gc.C) {
	panicFunc := func() { instancepoller.NewAPI(nil) }
	c.Assert(panicFunc, gc.PanicMatches, "caller is nil")
}

func (s *InstancePollerSuite) TestMachineCallsLife(c *gc.C) {
	// We have tested separately the Life method, here we just check
	// it's called internally.
	var called int
	expectedResults := params.LifeResults{
		Results: []params.LifeResult{{Life: "working"}},
	}
	apiCaller := successAPICaller(c, "Life", entitiesArgs, expectedResults, &called)
	api := instancepoller.NewAPI(apiCaller)
	m, err := api.Machine(names.NewMachineTag("42"))
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(called, gc.Equals, 1)
	c.Assert(m.Life(), gc.Equals, params.Life("working"))
	c.Assert(m.Id(), gc.Equals, "42")
}

func (s *InstancePollerSuite) TestWatchEnvironMachinesSuccess(c *gc.C) {
	// We're not testing the watcher logic here as it's already tested elsewhere.
	var numFacadeCalls int
	var numWatcherCalls int
	expectResult := params.StringsWatchResult{
		StringsWatcherId: "42",
		Changes:          []string{"foo", "bar"},
	}
	watcherFunc := func(caller base.APICaller, result params.StringsWatchResult) watcher.StringsWatcher {
		numWatcherCalls++
		c.Check(caller, gc.NotNil)
		c.Check(result, jc.DeepEquals, expectResult)
		return nil
	}
	s.PatchValue(instancepoller.NewStringsWatcher, watcherFunc)

	apiCaller := successAPICaller(c, "WatchEnvironMachines", nil, expectResult, &numFacadeCalls)

	api := instancepoller.NewAPI(apiCaller)
	w, err := api.WatchEnvironMachines()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(numFacadeCalls, gc.Equals, 1)
	c.Assert(numWatcherCalls, gc.Equals, 1)
	c.Assert(w, gc.IsNil)
}

func (s *InstancePollerSuite) TestWatchEnvironMachinesClientError(c *gc.C) {
	var called int
	apiCaller := clientErrorAPICaller(c, "WatchEnvironMachines", nil, &called)
	api := instancepoller.NewAPI(apiCaller)
	w, err := api.WatchEnvironMachines()
	c.Assert(err, gc.ErrorMatches, "client error!")
	c.Assert(w, gc.IsNil)
	c.Assert(called, gc.Equals, 1)
}

func (s *InstancePollerSuite) TestWatchEnvironMachinesServerError(c *gc.C) {
	var called int
	expectedResults := params.StringsWatchResult{
		Error: apiservertesting.ServerError("server boom!"),
	}
	apiCaller := successAPICaller(c, "WatchEnvironMachines", nil, expectedResults, &called)

	api := instancepoller.NewAPI(apiCaller)
	w, err := api.WatchEnvironMachines()
	c.Assert(err, gc.ErrorMatches, "server boom!")
	c.Assert(called, gc.Equals, 1)
	c.Assert(w, gc.IsNil)
}

func (s *InstancePollerSuite) TestWatchForEnvironConfigChangesClientError(c *gc.C) {
	// We're not testing the success case as we're not patching the
	// NewNotifyWatcher call the embedded EnvironWatcher is calling.
	var called int
	apiCaller := clientErrorAPICaller(c, "WatchForEnvironConfigChanges", nil, &called)

	api := instancepoller.NewAPI(apiCaller)
	w, err := api.WatchForEnvironConfigChanges()
	c.Assert(err, gc.ErrorMatches, "client error!")
	c.Assert(called, gc.Equals, 1)
	c.Assert(w, gc.IsNil)
}

func (s *InstancePollerSuite) TestEnvironConfigSuccess(c *gc.C) {
	var called int
	expectedConfig := coretesting.EnvironConfig(c)
	expectedResults := params.EnvironConfigResult{
		Config: params.EnvironConfig(expectedConfig.AllAttrs()),
	}
	apiCaller := successAPICaller(c, "EnvironConfig", nil, expectedResults, &called)

	api := instancepoller.NewAPI(apiCaller)
	cfg, err := api.EnvironConfig()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(called, gc.Equals, 1)
	c.Assert(cfg, jc.DeepEquals, expectedConfig)
}

func (s *InstancePollerSuite) TestEnvironConfigClientError(c *gc.C) {
	var called int
	apiCaller := clientErrorAPICaller(c, "EnvironConfig", nil, &called)
	api := instancepoller.NewAPI(apiCaller)
	cfg, err := api.EnvironConfig()
	c.Assert(err, gc.ErrorMatches, "client error!")
	c.Assert(cfg, gc.IsNil)
	c.Assert(called, gc.Equals, 1)
}

func (s *InstancePollerSuite) TestEnvironConfigServerError(c *gc.C) {
	var called int
	expectResults := params.EnvironConfigResult{
		Config: params.EnvironConfig{"type": "foo"},
	}
	apiCaller := successAPICaller(c, "EnvironConfig", nil, expectResults, &called)

	api := instancepoller.NewAPI(apiCaller)
	cfg, err := api.EnvironConfig()
	c.Assert(err, gc.NotNil) // the actual error doesn't matter
	c.Assert(called, gc.Equals, 1)
	c.Assert(cfg, gc.IsNil)
}

func clientErrorAPICaller(c *gc.C, method string, expectArgs interface{}, numCalls *int) base.APICaller {
	args := &apitesting.CheckArgs{
		Facade:        "InstancePoller",
		VersionIsZero: true,
		IdIsEmpty:     true,
		Method:        method,
		Args:          expectArgs,
	}
	return apitesting.CheckingAPICaller(c, args, numCalls, errors.New("client error!"))
}

func successAPICaller(c *gc.C, method string, expectArgs, useResults interface{}, numCalls *int) base.APICaller {
	args := &apitesting.CheckArgs{
		Facade:        "InstancePoller",
		VersionIsZero: true,
		IdIsEmpty:     true,
		Method:        method,
		Args:          expectArgs,
		Results:       useResults,
	}
	return apitesting.CheckingAPICaller(c, args, numCalls, nil)
}
