// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package addresser_test

import (
	"errors"

	gc "gopkg.in/check.v1"

	"github.com/juju/juju/api/addresser"
	"github.com/juju/juju/api/base"
	apitesting "github.com/juju/juju/api/base/testing"
	"github.com/juju/juju/api/watcher"
	"github.com/juju/juju/apiserver/params"
	apiservertesting "github.com/juju/juju/apiserver/testing"
	coretesting "github.com/juju/juju/testing"
	jc "github.com/juju/testing/checkers"
)

type AddresserSuite struct {
	coretesting.BaseSuite
}

var _ = gc.Suite(&AddresserSuite{})

func (s *AddresserSuite) TestNewAPISuccess(c *gc.C) {
	var called int
	apiCaller := clientErrorAPICaller(c, "IPAddresses", nil, &called)
	api := addresser.NewAPI(apiCaller)
	c.Check(api, gc.NotNil)
	c.Check(called, gc.Equals, 0)

	// Make a call so that an error will be returned.
	addresses, err := api.IPAddresses("")
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
}

func (s *AddresserSuite) TestIPAddressesClientError(c *gc.C) {
}

func (s *AddresserSuite) TestIPAddressesServerError(c *gc.C) {
}

func (s *AddresserSuite) TestRemoveSuccess(c *gc.C) {
}

func (s *AddresserSuite) TestRemoveClientError(c *gc.C) {
}

func (s *AddresserSuite) TestRemoveServerError(c *gc.C) {
}

func (s *AddresserSuite) TestWatchIPAddressesSuccess(c *gc.C) {
}

func (s *AddresserSuite) TestWatchIPAddressesClientError(c *gc.C) {
}

func (s *AddresserSuite) TestWatchIPAddressesServerError(c *gc.C) {
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
