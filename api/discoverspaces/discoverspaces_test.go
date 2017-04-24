// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package discoverspaces_test

import (
	"errors"

	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	apitesting "github.com/juju/juju/api/base/testing"
	"github.com/juju/juju/api/discoverspaces"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/environs/config"
	coretesting "github.com/juju/juju/testing"
)

type DiscoverSpacesSuite struct {
	coretesting.BaseSuite
}

var _ = gc.Suite(&DiscoverSpacesSuite{})

func (s *DiscoverSpacesSuite) TestNewAPI(c *gc.C) {
	apiCaller := clientErrorAPICaller(c, "ListSpaces", nil)
	api := discoverspaces.NewAPI(apiCaller)
	c.Check(api, gc.NotNil)
	c.Check(apiCaller.CallCount, gc.Equals, 0)

	// Make a call so that an error will be returned.
	_, err := api.ListSpaces()
	c.Assert(err, gc.ErrorMatches, "client error!")
	c.Assert(apiCaller.CallCount, gc.Equals, 1)
}

func clientErrorAPICaller(c *gc.C, method string, expectArgs interface{}) *apitesting.CallChecker {
	return apitesting.APICallChecker(c, apitesting.APICall{
		Facade:        "DiscoverSpaces",
		VersionIsZero: true,
		IdIsEmpty:     true,
		Method:        method,
		Args:          expectArgs,
		Error:         errors.New("client error!"),
	})
}

func successAPICaller(c *gc.C, method string, expectArgs, useResults interface{}) *apitesting.CallChecker {
	return apitesting.APICallChecker(c, apitesting.APICall{
		Facade:        "DiscoverSpaces",
		VersionIsZero: true,
		IdIsEmpty:     true,
		Method:        method,
		Args:          expectArgs,
		Results:       useResults,
	})
}

func (s *DiscoverSpacesSuite) TestNewAPIWithNilCaller(c *gc.C) {
	panicFunc := func() { discoverspaces.NewAPI(nil) }
	c.Assert(panicFunc, gc.PanicMatches, "caller is nil")
}

func (s *DiscoverSpacesSuite) TestListSpaces(c *gc.C) {
	expectedResult := params.DiscoverSpacesResults{
		Results: []params.ProviderSpace{{Name: "foobar"}},
	}
	apiCaller := successAPICaller(c, "ListSpaces", nil, expectedResult)
	api := discoverspaces.NewAPI(apiCaller)

	result, err := api.ListSpaces()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result, jc.DeepEquals, expectedResult)
	c.Assert(apiCaller.CallCount, gc.Equals, 1)
}

func (s *DiscoverSpacesSuite) TestAddSubnets(c *gc.C) {
	expectedResult := params.ErrorResults{
		Results: []params.ErrorResult{{}},
	}
	expectedArgs := params.AddSubnetsParams{
		Subnets: []params.AddSubnetParams{{SubnetTag: "foo"}},
	}
	apiCaller := successAPICaller(c, "AddSubnets", expectedArgs, expectedResult)
	api := discoverspaces.NewAPI(apiCaller)

	result, err := api.AddSubnets(expectedArgs)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result, jc.DeepEquals, expectedResult)
	c.Assert(apiCaller.CallCount, gc.Equals, 1)
}

func (s *DiscoverSpacesSuite) TestCreateSpaces(c *gc.C) {
	expectedResult := params.ErrorResults{
		Results: []params.ErrorResult{{}},
	}
	expectedArgs := params.CreateSpacesParams{
		Spaces: []params.CreateSpaceParams{{SpaceTag: "foo"}},
	}
	apiCaller := successAPICaller(c, "CreateSpaces", expectedArgs, expectedResult)
	api := discoverspaces.NewAPI(apiCaller)

	result, err := api.CreateSpaces(expectedArgs)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result, jc.DeepEquals, expectedResult)
	c.Assert(apiCaller.CallCount, gc.Equals, 1)
}

func (s *DiscoverSpacesSuite) TestModelConfig(c *gc.C) {
	cfg, err := config.New(config.UseDefaults, coretesting.FakeConfig())
	c.Assert(err, jc.ErrorIsNil)
	expectedResult := params.ModelConfigResult{
		Config: cfg.AllAttrs(),
	}
	apiCaller := successAPICaller(c, "ModelConfig", nil, expectedResult)
	api := discoverspaces.NewAPI(apiCaller)

	result, err := api.ModelConfig()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result, jc.DeepEquals, cfg)
	c.Assert(apiCaller.CallCount, gc.Equals, 1)
}
