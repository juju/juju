// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package addresser_test

import (
	"errors"

	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/api/addresser"
	"github.com/juju/juju/api/base"
	apitesting "github.com/juju/juju/api/base/testing"
	"github.com/juju/juju/api/watcher"
	"github.com/juju/juju/apiserver/params"
	apiservertesting "github.com/juju/juju/apiserver/testing"
	coretesting "github.com/juju/juju/testing"
)

type AddresserSuite struct {
	coretesting.BaseSuite
}

var _ = gc.Suite(&AddresserSuite{})

func (s *AddresserSuite) TestNewAPI(c *gc.C) {
	var called int
	apiCaller := clientErrorAPICaller(c, "CleanupIPAddresses", nil, &called)
	api := addresser.NewAPI(apiCaller)
	c.Check(api, gc.NotNil)
	c.Check(called, gc.Equals, 0)

	// Make a call so that an error will be returned.
	err := api.CleanupIPAddresses()
	c.Assert(err, gc.ErrorMatches, "client error!")
	c.Assert(called, gc.Equals, 1)
}

func (s *AddresserSuite) TestNewAPIWithNilCaller(c *gc.C) {
	panicFunc := func() { addresser.NewAPI(nil) }
	c.Assert(panicFunc, gc.PanicMatches, "caller is nil")
}

func (s *AddresserSuite) TestCanDeallocateAddressesSuccess(c *gc.C) {
	var called int
	expectedResult := params.BoolResult{
		Result: true,
	}
	apiCaller := successAPICaller(c, "CanDeallocateAddresses", nil, expectedResult, &called)
	api := addresser.NewAPI(apiCaller)

	ok, err := api.CanDeallocateAddresses()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(ok, jc.IsTrue)
	c.Assert(called, gc.Equals, 1)
}

func (s *AddresserSuite) TestCanDeallocateAddressesServerError(c *gc.C) {
	var called int
	expectedResult := params.BoolResult{
		Error: apiservertesting.ServerError("server boom!"),
	}
	apiCaller := successAPICaller(c, "CanDeallocateAddresses", nil, expectedResult, &called)
	api := addresser.NewAPI(apiCaller)

	ok, err := api.CanDeallocateAddresses()
	c.Assert(err, gc.ErrorMatches, "server boom!")
	c.Assert(ok, jc.IsFalse)
	c.Assert(called, gc.Equals, 1)
}

func (s *AddresserSuite) TestCleanupIPAddressesSuccess(c *gc.C) {
	var called int
	expectedResult := params.ErrorResult{}
	apiCaller := successAPICaller(c, "CleanupIPAddresses", nil, expectedResult, &called)
	api := addresser.NewAPI(apiCaller)

	err := api.CleanupIPAddresses()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(called, gc.Equals, 1)
}

func (s *AddresserSuite) TestCleanupIPAddressesServerError(c *gc.C) {
	var called int
	expectedResult := params.ErrorResult{
		Error: apiservertesting.ServerError("server boom!"),
	}
	apiCaller := successAPICaller(c, "CleanupIPAddresses", nil, expectedResult, &called)
	api := addresser.NewAPI(apiCaller)

	err := api.CleanupIPAddresses()
	c.Assert(err, gc.ErrorMatches, "server boom!")
	c.Assert(called, gc.Equals, 1)
}

func (s *AddresserSuite) TestWatchIPAddressesSuccess(c *gc.C) {
	var numFacadeCalls int
	var numWatcherCalls int
	expectedResult := params.EntityWatchResult{
		EntityWatcherId: "42",
		Changes: []string{
			"ipaddress-11111111-0000-0000-0000-000000000000",
			"ipaddress-22222222-0000-0000-0000-000000000000",
		},
	}
	watcherFunc := func(caller base.APICaller, result params.EntityWatchResult) watcher.EntityWatcher {
		numWatcherCalls++
		c.Check(caller, gc.NotNil)
		c.Check(result, jc.DeepEquals, expectedResult)
		return nil
	}
	s.PatchValue(addresser.NewEntityWatcher, watcherFunc)

	apiCaller := successAPICaller(c, "WatchIPAddresses", nil, expectedResult, &numFacadeCalls)
	api := addresser.NewAPI(apiCaller)

	w, err := api.WatchIPAddresses()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(numFacadeCalls, gc.Equals, 1)
	c.Assert(numWatcherCalls, gc.Equals, 1)
	c.Assert(w, gc.IsNil)
}

func (s *AddresserSuite) TestWatchIPAddressesClientError(c *gc.C) {
	var called int
	apiCaller := clientErrorAPICaller(c, "WatchIPAddresses", nil, &called)

	api := addresser.NewAPI(apiCaller)
	w, err := api.WatchIPAddresses()

	c.Assert(w, jc.ErrorIsNil)
	c.Assert(err, gc.ErrorMatches, "client error!")
	c.Assert(called, gc.Equals, 1)
}

func (s *AddresserSuite) TestWatchIPAddressesServerError(c *gc.C) {
	var called int
	expectedResult := params.EntityWatchResult{
		Error: apiservertesting.ServerError("server boom!"),
	}
	apiCaller := successAPICaller(c, "WatchIPAddresses", nil, expectedResult, &called)
	api := addresser.NewAPI(apiCaller)

	w, err := api.WatchIPAddresses()
	c.Assert(w, jc.ErrorIsNil)
	c.Assert(err, gc.ErrorMatches, "server boom!")
	c.Assert(called, gc.Equals, 1)
}

func successAPICaller(c *gc.C, method string, expectArgs, useResults interface{}, numCalls *int) base.APICaller {
	args := &apitesting.CheckArgs{
		Facade:        "Addresser",
		VersionIsZero: true,
		IdIsEmpty:     true,
		Method:        method,
		Args:          expectArgs,
		Results:       useResults,
	}
	return apitesting.CheckingAPICaller(c, args, numCalls, nil)
}

func clientErrorAPICaller(c *gc.C, method string, expectArgs interface{}, numCalls *int) base.APICaller {
	args := &apitesting.CheckArgs{
		Facade:        "Addresser",
		VersionIsZero: true,
		IdIsEmpty:     true,
		Method:        method,
		Args:          expectArgs,
	}
	return apitesting.CheckingAPICaller(c, args, numCalls, errors.New("client error!"))
}
