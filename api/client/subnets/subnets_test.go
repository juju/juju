// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package subnets_test

import (
	"errors"

	"github.com/juju/names/v4"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	apitesting "github.com/juju/juju/api/base/testing"
	"github.com/juju/juju/api/client/subnets"
	"github.com/juju/juju/rpc/params"
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
