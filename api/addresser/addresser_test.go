// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package addresser_test

import (
	"errors"

	gc "gopkg.in/check.v1"

	"github.com/juju/names"
	jc "github.com/juju/testing/checkers"

	"github.com/juju/juju/api/addresser"
	"github.com/juju/juju/api/base"
	apitesting "github.com/juju/juju/api/base/testing"
	"github.com/juju/juju/api/common"
	"github.com/juju/juju/api/watcher"
	"github.com/juju/juju/apiserver/params"
	apiservertesting "github.com/juju/juju/apiserver/testing"
	coretesting "github.com/juju/juju/testing"
)

type AddresserSuite struct {
	coretesting.BaseSuite
}

var _ = gc.Suite(&AddresserSuite{})

func (s *AddresserSuite) TestNewAPISuccess(c *gc.C) {
	var called int
	// IPAddress below uses common.Life for implementation.
	apiCaller := clientErrorAPICaller(c, "Life", nil, &called)
	api := addresser.NewAPI(apiCaller)
	c.Check(api, gc.NotNil)
	c.Check(called, gc.Equals, 0)

	// Make a call so that an error will be returned.
	addresses, err := api.IPAddresses(names.NewIPAddressTag("00000000-0000-0000-0000-000000000000"))
	c.Assert(err, gc.ErrorMatches, "client error!")
	c.Assert(addresses, gc.IsNil)
	c.Assert(called, gc.Equals, 1)
}

func (s *AddresserSuite) TestNewAPIWithNilCaller(c *gc.C) {
	panicFunc := func() { addresser.NewAPI(nil) }
	c.Assert(panicFunc, gc.PanicMatches, "caller is nil")
}

func (s *AddresserSuite) TestEnvironConfigSuccess(c *gc.C) {
	var called int
	expectedConfig := coretesting.EnvironConfig(c)
	expectedResults := params.EnvironConfigResult{
		Config: params.EnvironConfig(expectedConfig.AllAttrs()),
	}
	apiCaller := successAPICaller(c, "EnvironConfig", nil, expectedResults, &called)
	api := addresser.NewAPI(apiCaller)

	cfg, err := api.EnvironConfig()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(called, gc.Equals, 1)
	c.Assert(cfg, jc.DeepEquals, expectedConfig)
}

func (s *AddresserSuite) TestEnvironConfigClientError(c *gc.C) {
	var called int
	apiCaller := clientErrorAPICaller(c, "EnvironConfig", nil, &called)
	api := addresser.NewAPI(apiCaller)

	cfg, err := api.EnvironConfig()
	c.Assert(err, gc.ErrorMatches, "client error!")
	c.Assert(cfg, gc.IsNil)
	c.Assert(called, gc.Equals, 1)
}

func (s *AddresserSuite) TestEnvironConfigServerError(c *gc.C) {
	var called int
	expectResults := params.EnvironConfigResult{
		Config: params.EnvironConfig{"type": "foo"},
	}
	apiCaller := successAPICaller(c, "EnvironConfig", nil, expectResults, &called)
	api := addresser.NewAPI(apiCaller)

	cfg, err := api.EnvironConfig()
	c.Assert(err, gc.NotNil)
	c.Assert(called, gc.Equals, 1)
	c.Assert(cfg, gc.IsNil)
}

func (s *AddresserSuite) TestIPAddressesSuccess(c *gc.C) {
	tag1 := names.NewIPAddressTag("11111111-0000-0000-0000-000000000000")
	tag2 := names.NewIPAddressTag("22222222-0000-0000-0000-000000000000")
	tag3 := names.NewIPAddressTag("33333333-0000-0000-0000-000000000000")

	var called int
	args := params.Entities{
		Entities: []params.Entity{
			{Tag: tag1.String()},
			{Tag: tag2.String()},
			{Tag: tag3.String()},
		},
	}
	results := params.LifeResults{
		Results: []params.LifeResult{
			{Life: params.Alive},
			{Life: params.Dying},
			{Life: params.Dead},
		},
	}
	apiCaller := successAPICaller(c, "Life", args, results, &called)
	api := addresser.NewAPI(apiCaller)

	ipAddresses, err := api.IPAddresses(tag1, tag2, tag3)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(called, gc.Equals, 1)
	c.Assert(len(ipAddresses), gc.Equals, 3)
	for i, ipAddress := range ipAddresses {
		c.Check(ipAddress.Tag().String(), gc.Equals, args.Entities[i].Tag)
		c.Check(ipAddress.Life(), gc.Equals, results.Results[i].Life)
	}
}

func (s *AddresserSuite) TestIPAddressesClientError(c *gc.C) {
	var called int
	apiCaller := clientErrorAPICaller(c, "Life", nil, &called)
	api := addresser.NewAPI(apiCaller)

	ipAddresses, err := api.IPAddresses(names.NewIPAddressTag("00000000-0000-0000-0000-000000000000"))
	c.Assert(err, gc.ErrorMatches, "client error!")
	c.Assert(ipAddresses, gc.IsNil)
	c.Assert(called, gc.Equals, 1)
}

func (s *AddresserSuite) TestIPAddressesServerError(c *gc.C) {
	tag1 := names.NewIPAddressTag("11111111-0000-0000-0000-000000000000")
	tag2 := names.NewIPAddressTag("22222222-0000-0000-0000-000000000000")

	var called int
	args := params.Entities{
		Entities: []params.Entity{
			{Tag: tag1.String()},
			{Tag: tag2.String()},
		},
	}
	results := params.LifeResults{
		Results: []params.LifeResult{
			{params.Alive, nil},
			{"", apiservertesting.ServerError("server failure")},
		},
	}
	apiCaller := successAPICaller(c, "Life", args, results, &called)
	api := addresser.NewAPI(apiCaller)

	ipAddresses, err := api.IPAddresses(tag1, tag2)
	c.Assert(len(ipAddresses), gc.Equals, 2)
	c.Assert(ipAddresses[0].Tag().String(), gc.Equals, tag1.String())
	c.Assert(ipAddresses[1], gc.IsNil)
	c.Assert(err, gc.Equals, common.ErrPartialResults)
	c.Assert(called, gc.Equals, 1)
}

func (s *AddresserSuite) TestRemoveSuccess(c *gc.C) {
	tag1 := names.NewIPAddressTag("11111111-0000-0000-0000-000000000000")
	tag2 := names.NewIPAddressTag("22222222-0000-0000-0000-000000000000")
	tag3 := names.NewIPAddressTag("33333333-0000-0000-0000-000000000000")

	var called int
	args := params.Entities{
		Entities: []params.Entity{
			{Tag: tag1.String()},
			{Tag: tag2.String()},
			{Tag: tag3.String()},
		},
	}
	results := params.ErrorResults{
		Results: []params.ErrorResult{{nil}, {nil}, {nil}},
	}
	apiCaller := successAPICaller(c, "Remove", args, results, &called)
	api := addresser.NewAPI(apiCaller)

	ipAddress1 := addresser.NewIPAddress(api, tag1, params.Alive)
	ipAddress2 := addresser.NewIPAddress(api, tag2, params.Dying)
	ipAddress3 := addresser.NewIPAddress(api, tag3, params.Dead)

	err := api.Remove(ipAddress1, ipAddress2, ipAddress3)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(called, gc.Equals, 1)
}

func (s *AddresserSuite) TestRemoveClientError(c *gc.C) {
	var called int
	tag := names.NewIPAddressTag("00000000-0000-0000-0000-000000000000")
	apiCaller := clientErrorAPICaller(c, "Remove", nil, &called)
	api := addresser.NewAPI(apiCaller)

	ipAddress := addresser.NewIPAddress(api, tag, params.Alive)
	err := api.Remove(ipAddress)
	c.Assert(err, gc.ErrorMatches, "client error!")
	c.Assert(called, gc.Equals, 1)
}

func (s *AddresserSuite) TestRemoveServerError(c *gc.C) {
	var called int
	tag := names.NewIPAddressTag("00000000-0000-0000-0000-000000000000")
	args := params.Entities{
		Entities: []params.Entity{{Tag: tag.String()}},
	}
	results := params.ErrorResults{
		Results: []params.ErrorResult{{apiservertesting.ServerError("server boom!")}},
	}
	apiCaller := successAPICaller(c, "Remove", args, results, &called)
	api := addresser.NewAPI(apiCaller)

	ipAddress := addresser.NewIPAddress(api, tag, params.Alive)
	err := api.Remove(ipAddress)
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

	c.Assert(w, gc.IsNil)
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
	c.Assert(w, gc.IsNil)
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
