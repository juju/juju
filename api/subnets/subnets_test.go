// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package subnets_test

import (
	"errors"

	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
	"gopkg.in/juju/names.v2"

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
	s.api = subnets.NewAPI(s.apiCaller)
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
	subnetTag := names.NewSubnetTag(cidr).String()
	if providerId != "" {
		subnetTag = ""
	}

	expectArgs := params.AddSubnetsParams{
		Subnets: []params.AddSubnetParams{{
			SpaceTag:         spaceTag,
			SubnetTag:        subnetTag,
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

func makeCreateSubnetsArgs(cidr, space string, zones []string, isPublic bool) apitesting.APICall {
	spaceTag := names.NewSpaceTag(space).String()
	subnetTag := names.NewSubnetTag(cidr).String()

	expectArgs := params.CreateSubnetsParams{
		Subnets: []params.CreateSubnetParams{{
			SpaceTag:  spaceTag,
			SubnetTag: subnetTag,
			Zones:     zones,
			IsPublic:  isPublic,
		}}}

	expectResults := params.ErrorResults{
		Results: []params.ErrorResult{{}},
	}

	args := apitesting.APICall{
		Facade:  "Subnets",
		Method:  "CreateSubnets",
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
		names.NewSubnetTag(cidr),
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
		names.NewSubnetTag(cidr),
		network.Id(providerId),
		names.NewSpaceTag(space),
		zones,
	)
	c.Check(s.apiCaller.CallCount, gc.Equals, 1)
	c.Assert(err, gc.ErrorMatches, "bang")
}

func (s *SubnetsSuite) TestCreateSubnet(c *gc.C) {
	cidr := "1.1.1.0/24"
	space := "bar"
	zones := []string{"foo", "bar"}
	isPublic := true
	args := makeCreateSubnetsArgs(cidr, space, zones, isPublic)
	s.prepareAPICall(c, args)
	err := s.api.CreateSubnet(
		names.NewSubnetTag(cidr),
		names.NewSpaceTag(space),
		zones,
		isPublic,
	)
	c.Assert(s.apiCaller.CallCount, gc.Equals, 1)
	c.Assert(err, jc.ErrorIsNil)
}

func (s *SubnetsSuite) TestCreateSubnetFails(c *gc.C) {
	cidr := "1.1.1.0/24"
	isPublic := true
	space := "bar"
	zones := []string{"foo", "bar"}
	args := makeCreateSubnetsArgs(cidr, space, zones, isPublic)
	args.Error = errors.New("bang")
	s.prepareAPICall(c, args)
	err := s.api.CreateSubnet(
		names.NewSubnetTag(cidr),
		names.NewSpaceTag(space),
		zones,
		isPublic,
	)
	c.Check(s.apiCaller.CallCount, gc.Equals, 1)
	c.Assert(err, gc.ErrorMatches, "bang")
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
