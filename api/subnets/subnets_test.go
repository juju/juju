// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package subnets_test

import (
	"errors"

	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
	"gopkg.in/juju/names.v3"

	apitesting "github.com/juju/juju/api/base/testing"
	"github.com/juju/juju/api/subnets"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/core/network"
	coretesting "github.com/juju/juju/testing"
)

// SubnetsSuite tests the client side subnets API
type SubnetsSuite struct {
	coretesting.BaseSuite

	apiCaller *apitesting.CallChecker
	api       *subnets.API
}

var _ = gc.Suite(&SubnetsSuite{})

func (s *SubnetsSuite) prepareAPICall(c *gc.C, args apitesting.APICall) {
	s.apiCaller = apitesting.APICallChecker(c, args)
	best := &apitesting.BestVersionCaller{
		BestVersion:   3,
		APICallerFunc: s.apiCaller.APICallerFunc,
	}
	s.api = subnets.NewAPI(best)
	c.Check(s.api, gc.NotNil)
	c.Check(s.apiCaller.CallCount, gc.Equals, 0)
}

// TestNewAPISuccess checks that a new subnets API is created when passed a non-nil caller
func (s *SubnetsSuite) TestNewAPISuccess(c *gc.C) {
	apiCaller := apitesting.APICallChecker(c)
	api := subnets.NewAPI(apiCaller)
	c.Check(api, gc.NotNil)
	c.Check(apiCaller.CallCount, gc.Equals, 0)
}

// TestNewAPIWithNilCaller checks that a new subnets API is not created when passed a nil caller
func (s *SubnetsSuite) TestNewAPIWithNilCaller(c *gc.C) {
	panicFunc := func() { subnets.NewAPI(nil) }
	c.Assert(panicFunc, gc.PanicMatches, "caller is nil")
}

func makeAddSubnetsArgs(cidr, providerId, space string, zones []string) apitesting.APICall {
	spaceTag := names.NewSpaceTag(space).String()
	if providerId != "" {
		cidr = ""
	}

	expectArgs := params.AddSubnetsParams{
		Subnets: []params.AddSubnetParams{{
			SpaceTag:         spaceTag,
			CIDR:             cidr,
			SubnetProviderId: providerId,
			Zones:            zones,
		}}}

	expectResults := params.ErrorResults{
		Results: []params.ErrorResult{{}},
	}

	args := apitesting.APICall{
		Facade:  "Subnets",
		Method:  "AddSubnets",
		Args:    expectArgs,
		Results: expectResults,
	}

	return args
}

func makeListSubnetsArgs(space *names.SpaceTag, zone string) apitesting.APICall {
	expectResults := params.ListSubnetsResults{}
	expectArgs := params.SubnetsFilters{
		SpaceTag: space.String(),
		Zone:     zone,
	}
	args := apitesting.APICall{
		Facade:  "Subnets",
		Method:  "ListSubnets",
		Results: expectResults,
		Args:    expectArgs,
	}
	return args
}

func (s *SubnetsSuite) TestAddSubnet(c *gc.C) {
	cidr := "1.1.1.0/24"
	providerId := "foo"
	space := "bar"
	zones := []string{"foo", "bar"}
	args := makeAddSubnetsArgs(cidr, providerId, space, zones)
	s.prepareAPICall(c, args)
	err := s.api.AddSubnet(
		cidr,
		network.Id(providerId),
		names.NewSpaceTag(space),
		zones,
	)
	c.Assert(s.apiCaller.CallCount, gc.Equals, 1)
	c.Assert(err, jc.ErrorIsNil)
}

func (s *SubnetsSuite) TestAddSubnetFails(c *gc.C) {
	cidr := "1.1.1.0/24"
	providerId := "foo"
	space := "bar"
	zones := []string{"foo", "bar"}
	args := makeAddSubnetsArgs(cidr, providerId, space, zones)
	args.Error = errors.New("bang")
	s.prepareAPICall(c, args)
	err := s.api.AddSubnet(
		cidr,
		network.Id(providerId),
		names.NewSpaceTag(space),
		zones,
	)
	c.Check(s.apiCaller.CallCount, gc.Equals, 1)
	c.Assert(err, gc.ErrorMatches, "bang")
}

func (s *SubnetsSuite) TestAddSubnetV2(c *gc.C) {
	var called bool
	apicaller := &apitesting.BestVersionCaller{
		APICallerFunc: apitesting.APICallerFunc(
			func(objType string,
				version int,
				id, request string,
				a, result interface{},
			) error {
				c.Check(objType, gc.Equals, "Subnets")
				c.Check(id, gc.Equals, "")
				c.Check(request, gc.Equals, "AddSubnets")
				c.Assert(result, gc.FitsTypeOf, &params.ErrorResults{})
				c.Assert(a, jc.DeepEquals, params.AddSubnetsParamsV2{
					Subnets: []params.AddSubnetParamsV2{{
						SpaceTag:  names.NewSpaceTag("testv2").String(),
						SubnetTag: "subnet-1.1.1.0/24",
					}}})
				*result.(*params.ErrorResults) = params.ErrorResults{
					Results: []params.ErrorResult{{}},
				}
				called = true
				return nil
			},
		),
		BestVersion: 2,
	}
	apiv2 := subnets.NewAPI(apicaller)
	err := apiv2.AddSubnet("1.1.1.0/24", "", names.NewSpaceTag("testv2"), nil)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(called, jc.IsTrue)
}

func (s *SubnetsSuite) TestListSubnetsNoResults(c *gc.C) {
	space := names.NewSpaceTag("foo")
	zone := "bar"
	args := makeListSubnetsArgs(&space, zone)
	s.prepareAPICall(c, args)
	results, err := s.api.ListSubnets(&space, zone)
	c.Assert(s.apiCaller.CallCount, gc.Equals, 1)
	c.Assert(err, jc.ErrorIsNil)

	var expectedResults []params.Subnet
	c.Assert(results, jc.DeepEquals, expectedResults)
}

func (s *SubnetsSuite) TestListSubnetsFails(c *gc.C) {
	space := names.NewSpaceTag("foo")
	zone := "bar"
	args := makeListSubnetsArgs(&space, zone)
	args.Error = errors.New("bang")
	s.prepareAPICall(c, args)
	results, err := s.api.ListSubnets(&space, zone)
	c.Assert(s.apiCaller.CallCount, gc.Equals, 1)
	c.Assert(err, gc.ErrorMatches, "bang")

	var expectedResults []params.Subnet
	c.Assert(results, jc.DeepEquals, expectedResults)
}

func (s *SubnetsSuite) testSubnetsByCIDR(c *gc.C,
	cidrs []string,
	results []params.SubnetsResult,
	err error, expectErr string,
) {
	var expectedResults params.SubnetsResults
	if results != nil {
		expectedResults.Results = results
	}

	s.prepareAPICall(c, apitesting.APICall{
		Facade:  "Subnets",
		Method:  "SubnetsByCIDR",
		Results: expectedResults,
		Error:   err,
	})
	gotResult, gotErr := s.api.SubnetsByCIDR(cidrs)
	c.Assert(s.apiCaller.CallCount, gc.Equals, 1)
	c.Assert(gotResult, jc.DeepEquals, results)

	if expectErr != "" {
		c.Assert(gotErr, gc.ErrorMatches, expectErr)
		return
	}

	if err != nil {
		c.Assert(gotErr, jc.DeepEquals, err)
	} else {
		c.Assert(gotErr, jc.ErrorIsNil)
	}
}

func (s *SubnetsSuite) TestSubnetsByCIDRWithNoCIDRs(c *gc.C) {
	var cidrs []string

	s.testSubnetsByCIDR(c, cidrs, []params.SubnetsResult{}, nil, "")
}

func (s *SubnetsSuite) TestSubnetsByCIDRWithNoResults(c *gc.C) {
	cidrs := []string{"10.0.1.10/24"}

	s.testSubnetsByCIDR(c, cidrs, []params.SubnetsResult{}, nil, "")
}

func (s *SubnetsSuite) TestSubnetsByCIDRWithResults(c *gc.C) {
	cidrs := []string{"10.0.1.10/24"}

	s.testSubnetsByCIDR(c, cidrs, []params.SubnetsResult{{
		Subnets: []params.SubnetV2{{
			ID: "aaabbb",
			Subnet: params.Subnet{
				CIDR: "10.0.1.10/24",
			},
		}},
	}}, nil, "")
}
