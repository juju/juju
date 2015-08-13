// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package subnets_test

import (
	"errors"

	gc "gopkg.in/check.v1"

	"github.com/juju/juju/api/base"
	apitesting "github.com/juju/juju/api/base/testing"
	"github.com/juju/juju/api/subnets"
	"github.com/juju/juju/apiserver/params"
	coretesting "github.com/juju/juju/testing"
	"github.com/juju/names"
)

// SubnetsSuite tests the client side subnets API
type SubnetsSuite struct {
	coretesting.BaseSuite

	called    int
	apiCaller base.APICaller
	api       *subnets.API
}

var _ = gc.Suite(&SubnetsSuite{})

func (s *SubnetsSuite) init(c *gc.C, args *apitesting.CheckArgs, err error) {
	s.called = 0
	s.apiCaller = apitesting.CheckingAPICaller(c, args, &s.called, err)
	s.api = subnets.NewAPI(s.apiCaller)
	c.Check(s.api, gc.NotNil)
	c.Check(s.called, gc.Equals, 0)
}

// TestNewAPISuccess checks that a new subnets API is created when passed a non-nil caller
func (s *SubnetsSuite) TestNewAPISuccess(c *gc.C) {
	var called int
	apiCaller := apitesting.CheckingAPICaller(c, nil, &called, nil)
	api := subnets.NewAPI(apiCaller)
	c.Check(api, gc.NotNil)
	c.Check(called, gc.Equals, 0)
}

// TestNewAPIWithNilCaller checks that a new subnets API is not created when passed a nil caller
func (s *SubnetsSuite) TestNewAPIWithNilCaller(c *gc.C) {
	panicFunc := func() { subnets.NewAPI(nil) }
	c.Assert(panicFunc, gc.PanicMatches, "caller is nil")
}

func makeAddSubnetsArgs(cidr, providerId, space string, zones []string) apitesting.CheckArgs {
	spaceTag := names.NewSpaceTag(space).String()
	subnetTag := names.NewSubnetTag(cidr).String()

	expectArgs := params.AddSubnetsParams{
		Subnets: []params.AddSubnetParams{
			{
				SpaceTag:         spaceTag,
				SubnetTag:        subnetTag,
				SubnetProviderId: providerId,
				Zones:            zones,
			}}}

	expectResults := params.ErrorResults{}

	args := apitesting.CheckArgs{
		Facade:  "Subnets",
		Method:  "AddSubnets",
		Args:    expectArgs,
		Results: expectResults,
	}

	return args
}

func makeCreateSubnetsArgs(cidr, space string, zones []string, isPublic bool) apitesting.CheckArgs {
	spaceTag := names.NewSpaceTag(space).String()
	subnetTag := names.NewSubnetTag(cidr).String()

	expectArgs := params.CreateSubnetsParams{
		Subnets: []params.CreateSubnetParams{
			{
				SpaceTag:  spaceTag,
				SubnetTag: subnetTag,
				Zones:     zones,
				IsPublic:  isPublic,
			}}}

	expectResults := params.ErrorResults{}

	args := apitesting.CheckArgs{
		Facade:  "Subnets",
		Method:  "CreateSubnets",
		Args:    expectArgs,
		Results: expectResults,
	}

	return args
}

func (s *SpacesSuite) TestAddSubnet(c *gc.C) {
	cidr := "1.1.1.0/24"
	providerId := "foo"
	space := "bar"
	zones := []string{"foo", "bar"}
	args := makeAddSubnetsArgs(cidr, providerId, space, zones)
	s.init(c, &args, nil)
	results, err := s.api.AddSubnet(cidr, providerId, space, zones)
	c.Assert(s.called, gc.Equals, 1)
	c.Assert(err, gc.IsNil)
	c.Assert(results, gc.DeepEquals, args.Results)
}

func (s *SpacesSuite) TestAddSubnetFails(c *gc.C) {
	cidr := "1.1.1.0/24"
	providerId := "foo"
	space := "bar"
	zones := []string{"foo", "bar"}
	args := makeAddSubnetsArgs(cidr, providerId, space, zones)
	s.init(c, &args, errors.New("bang"))
	results, err := s.api.AddSubnet(cidr, providerId, space, zones)
	c.Check(s.called, gc.Equals, 1)
	c.Assert(err, gc.ErrorMatches, "bang")
	c.Assert(results, gc.DeepEquals, args.Results)
}

func (s *SpacesSuite) TestCreateSubnet(c *gc.C) {
	cidr := "1.1.1.0/24"
	space := "bar"
	zones := []string{"foo", "bar"}
	isPublic := true
	args := makeCreateSubnetsArgs(cidr, space, zones, isPublic)
	s.init(c, &args, nil)
	results, err := s.api.CreateSubnet(cidr, space, zones, isPublic)
	c.Assert(s.called, gc.Equals, 1)
	c.Assert(err, gc.IsNil)
	c.Assert(results, gc.DeepEquals, args.Results)
}

func (s *SpacesSuite) TestCreateSubnetFails(c *gc.C) {
	cidr := "1.1.1.0/24"
	isPublic := true
	space := "bar"
	zones := []string{"foo", "bar"}
	args := makeCreateSubnetsArgs(cidr, space, zones, isPublic)
	s.init(c, &args, errors.New("bang"))
	results, err := s.api.CreateSubnet(cidr, space, zones, isPublic)
	c.Check(s.called, gc.Equals, 1)
	c.Assert(err, gc.ErrorMatches, "bang")
	c.Assert(results, gc.DeepEquals, args.Results)
}
