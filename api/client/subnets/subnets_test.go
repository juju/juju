// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package subnets_test

import (
	"context"

	"github.com/juju/errors"
	"github.com/juju/names/v6"
	"github.com/juju/tc"
	"go.uber.org/mock/gomock"

	basemocks "github.com/juju/juju/api/base/mocks"
	"github.com/juju/juju/api/client/subnets"
	"github.com/juju/juju/rpc/params"
)

// SubnetsSuite tests the client side subnets API
type SubnetsSuite struct {
}

var _ = tc.Suite(&SubnetsSuite{})

// TestNewAPISuccess checks that a new subnets API is created when passed a non-nil caller
func (s *SubnetsSuite) TestNewAPISuccess(c *tc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	apiCaller := basemocks.NewMockAPICallCloser(ctrl)
	apiCaller.EXPECT().BestFacadeVersion("Subnets").Return(4)

	api := subnets.NewAPI(apiCaller)
	c.Check(api, tc.NotNil)
}

// TestNewAPIWithNilCaller checks that a new subnets API is not created when passed a nil caller
func (s *SubnetsSuite) TestNewAPIWithNilCaller(c *tc.C) {
	panicFunc := func() { subnets.NewAPI(nil) }
	c.Assert(panicFunc, tc.PanicMatches, "caller is nil")
}

func makeListSubnetsArgs(space *names.SpaceTag, zone string) (params.SubnetsFilters, params.ListSubnetsResults) {
	expectArgs := params.SubnetsFilters{
		SpaceTag: space.String(),
		Zone:     zone,
	}
	return expectArgs, params.ListSubnetsResults{}
}

func (s *SubnetsSuite) TestListSubnetsNoResults(c *tc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	space := names.NewSpaceTag("foo")
	zone := "bar"
	args, results := makeListSubnetsArgs(&space, zone)
	result := new(params.ListSubnetsResults)

	mockFacadeCaller := basemocks.NewMockFacadeCaller(ctrl)
	mockFacadeCaller.EXPECT().FacadeCall(gomock.Any(), "ListSubnets", args, result).SetArg(3, results).Return(nil)
	client := subnets.NewAPIFromCaller(mockFacadeCaller)

	obtainedResults, err := client.ListSubnets(context.Background(), &space, zone)

	c.Assert(err, tc.ErrorIsNil)

	var expectedResults []params.Subnet
	c.Assert(obtainedResults, tc.DeepEquals, expectedResults)
}

func (s *SubnetsSuite) TestListSubnetsFails(c *tc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	space := names.NewSpaceTag("foo")
	zone := "bar"
	args, results := makeListSubnetsArgs(&space, zone)
	result := new(params.ListSubnetsResults)

	mockFacadeCaller := basemocks.NewMockFacadeCaller(ctrl)
	mockFacadeCaller.EXPECT().FacadeCall(gomock.Any(), "ListSubnets", args, result).SetArg(3, results).Return(errors.New("bang"))
	client := subnets.NewAPIFromCaller(mockFacadeCaller)

	obtainedResults, err := client.ListSubnets(context.Background(), &space, zone)
	c.Assert(err, tc.ErrorMatches, "bang")

	var expectedResults []params.Subnet
	c.Assert(obtainedResults, tc.DeepEquals, expectedResults)
}

func (s *SubnetsSuite) testSubnetsByCIDR(c *tc.C,
	ctrl *gomock.Controller,
	cidrs []string,
	results []params.SubnetsResult,
	err error, expectErr string,
) {
	var expectedResults params.SubnetsResults
	if results != nil {
		expectedResults.Results = results
	}
	args := params.CIDRParams{CIDRS: cidrs}

	result := new(params.SubnetsResults)
	mockFacadeCaller := basemocks.NewMockFacadeCaller(ctrl)
	mockFacadeCaller.EXPECT().FacadeCall(gomock.Any(), "SubnetsByCIDR", args, result).SetArg(3, expectedResults).Return(err)
	client := subnets.NewAPIFromCaller(mockFacadeCaller)

	gotResult, gotErr := client.SubnetsByCIDR(context.Background(), cidrs)
	c.Assert(gotResult, tc.DeepEquals, results)

	if expectErr != "" {
		c.Assert(gotErr, tc.ErrorMatches, expectErr)
		return
	}

	if err != nil {
		c.Assert(gotErr, tc.DeepEquals, err)
	} else {
		c.Assert(gotErr, tc.ErrorIsNil)
	}
}

func (s *SubnetsSuite) TestSubnetsByCIDRWithNoCIDRs(c *tc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	var cidrs []string

	s.testSubnetsByCIDR(c, ctrl, cidrs, []params.SubnetsResult{}, nil, "")
}

func (s *SubnetsSuite) TestSubnetsByCIDRWithNoResults(c *tc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	cidrs := []string{"10.0.1.10/24"}

	s.testSubnetsByCIDR(c, ctrl, cidrs, []params.SubnetsResult{}, nil, "")
}

func (s *SubnetsSuite) TestSubnetsByCIDRWithResults(c *tc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	cidrs := []string{"10.0.1.10/24"}

	s.testSubnetsByCIDR(c, ctrl, cidrs, []params.SubnetsResult{{
		Subnets: []params.SubnetV2{{
			ID: "aaabbb",
			Subnet: params.Subnet{
				CIDR: "10.0.1.10/24",
			},
		}},
	}}, nil, "")
}
