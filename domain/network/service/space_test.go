// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"context"
	"fmt"

	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gomock "go.uber.org/mock/gomock"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/core/network"
)

type spaceSuite struct {
	testing.IsolationSuite

	st     *MockState
	logger *MockLogger
}

var _ = gc.Suite(&spaceSuite{})

func (s *spaceSuite) setupMocks(c *gc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)

	s.st = NewMockState(ctrl)
	s.logger = NewMockLogger(ctrl)

	return ctrl
}

func (s *spaceSuite) TestSaveProviderSubnetsWithoutSpaceUUID(c *gc.C) {
	defer s.setupMocks(c).Finish()

	twoSubnets := []network.SubnetInfo{
		{
			ProviderId:        "1",
			AvailabilityZones: []string{"1", "2"},
			CIDR:              "10.0.0.1/24",
		},
		{
			ProviderId:        "2",
			AvailabilityZones: []string{"3", "4"},
			CIDR:              "10.100.30.1/24",
		},
	}

	s.st.EXPECT().UpsertSubnets(context.Background(), twoSubnets)

	err := NewSpaceService(s.st, s.logger).SaveProviderSubnets(context.Background(), twoSubnets, "", nil)
	c.Assert(err, jc.ErrorIsNil)
}

func (s *spaceSuite) TestSaveProviderSubnetsOnlyAddsSubnets(c *gc.C) {
	defer s.setupMocks(c).Finish()

	twoSubnets := []network.SubnetInfo{
		{
			ProviderId:        "1",
			AvailabilityZones: []string{"1", "2"},
			CIDR:              "10.0.0.1/24",
		},
		{
			ProviderId:        "2",
			AvailabilityZones: []string{"3", "4"},
			CIDR:              "10.100.30.1/24",
		},
	}

	s.st.EXPECT().UpsertSubnets(context.Background(), twoSubnets)

	err := NewSpaceService(s.st, s.logger).SaveProviderSubnets(context.Background(), twoSubnets, "", nil)
	c.Assert(err, jc.ErrorIsNil)

	anotherSubnet := []network.SubnetInfo{
		{
			ProviderId:        "3",
			AvailabilityZones: []string{"1", "2"},
			CIDR:              "10.0.1.1/24",
		},
	}

	s.st.EXPECT().UpsertSubnets(context.Background(), anotherSubnet)

	err = NewSpaceService(s.st, s.logger).SaveProviderSubnets(context.Background(), anotherSubnet, "", nil)
	c.Assert(err, jc.ErrorIsNil)
}

func (s *spaceSuite) TestSaveProviderSubnetsOnlyIdempotent(c *gc.C) {
	defer s.setupMocks(c).Finish()

	oneSubnet := []network.SubnetInfo{
		{
			ProviderId:        "1",
			AvailabilityZones: []string{"1", "2"},
			CIDR:              "10.0.0.1/24",
		},
	}

	s.st.EXPECT().UpsertSubnets(context.Background(), oneSubnet)
	err := NewSpaceService(s.st, s.logger).SaveProviderSubnets(context.Background(), oneSubnet, "", nil)
	c.Assert(err, jc.ErrorIsNil)

	// We expect the same subnets to be passed to the state methods.
	s.st.EXPECT().UpsertSubnets(context.Background(), oneSubnet)
	err = NewSpaceService(s.st, s.logger).SaveProviderSubnets(context.Background(), oneSubnet, "", nil)
	c.Assert(err, jc.ErrorIsNil)
}

func (s *spaceSuite) TestSaveProviderSubnetsWithFAN(c *gc.C) {
	defer s.setupMocks(c).Finish()

	twoSubnets := []network.SubnetInfo{
		{
			ProviderId:        "1",
			AvailabilityZones: []string{"1", "2"},
			CIDR:              "10.0.0.1/24",
		},
		{
			ProviderId:        "2",
			AvailabilityZones: []string{"3", "4"},
			CIDR:              "10.100.30.1/24",
		},
	}
	expected := append(twoSubnets, network.SubnetInfo{
		ProviderId:        network.Id(fmt.Sprintf("2-%s-10-100-30-0-24", network.InFan)),
		AvailabilityZones: []string{"3", "4"},
		CIDR:              "253.30.0.0/16",
		FanInfo: &network.FanCIDRs{
			FanLocalUnderlay: "10.100.30.1/24",
			FanOverlay:       "253.0.0.0/8",
		}},
	)

	s.st.EXPECT().UpsertSubnets(context.Background(), gomock.Any()).Do(
		func(ctx context.Context, subnets []network.SubnetInfo) {
			c.Check(subnets, gc.HasLen, 3)
			c.Check(subnets, gc.DeepEquals, expected)
		},
	)

	fanConfig, err := network.ParseFanConfig("10.100.0.0/16=253.0.0.0/8")
	c.Assert(err, jc.ErrorIsNil)
	err = NewSpaceService(s.st, s.logger).SaveProviderSubnets(context.Background(), twoSubnets, "", fanConfig)
	c.Assert(err, jc.ErrorIsNil)
}

func (s *spaceSuite) TestSaveProviderSubnetsIgnoreInterfaceLocalMulticast(c *gc.C) {
	defer s.setupMocks(c).Finish()

	oneSubnet := []network.SubnetInfo{
		{
			ProviderId:        "1",
			AvailabilityZones: []string{"1", "2"},
			CIDR:              "ff51:dead:beef::/48",
		},
	}

	s.st.EXPECT().UpsertSubnets(gomock.Any(), gomock.Any()).Times(0)
	err := NewSpaceService(s.st, s.logger).SaveProviderSubnets(context.Background(), oneSubnet, "", nil)
	c.Assert(err, jc.ErrorIsNil)
}

func (s *spaceSuite) TestSaveProviderSubnetsIgnoreLinkLocalMulticast(c *gc.C) {
	defer s.setupMocks(c).Finish()

	oneSubnet := []network.SubnetInfo{
		{
			ProviderId:        "1",
			AvailabilityZones: []string{"1", "2"},
			CIDR:              "ff32:dead:beef::/48",
		},
	}

	s.st.EXPECT().UpsertSubnets(gomock.Any(), gomock.Any()).Times(0)
	err := NewSpaceService(s.st, s.logger).SaveProviderSubnets(context.Background(), oneSubnet, "", nil)
	c.Assert(err, jc.ErrorIsNil)
}

func (s *spaceSuite) TestSaveProviderSubnetsIgnoreIPV6LinkLocalUnicast(c *gc.C) {
	defer s.setupMocks(c).Finish()

	oneSubnet := []network.SubnetInfo{
		{
			ProviderId:        "1",
			AvailabilityZones: []string{"1", "2"},
			CIDR:              "fe80:dead:beef::/48",
		},
	}

	s.st.EXPECT().UpsertSubnets(gomock.Any(), gomock.Any()).Times(0)
	err := NewSpaceService(s.st, s.logger).SaveProviderSubnets(context.Background(), oneSubnet, "", nil)
	c.Assert(err, jc.ErrorIsNil)
}

func (s *spaceSuite) TestSaveProviderSubnetsIgnoreIPV4LinkLocalUnicast(c *gc.C) {
	defer s.setupMocks(c).Finish()

	oneSubnet := []network.SubnetInfo{
		{
			ProviderId:        "1",
			AvailabilityZones: []string{"1", "2"},
			CIDR:              "169.254.13.0/24",
		},
	}

	s.st.EXPECT().UpsertSubnets(gomock.Any(), gomock.Any()).Times(0)
	err := NewSpaceService(s.st, s.logger).SaveProviderSubnets(context.Background(), oneSubnet, "", nil)
	c.Assert(err, jc.ErrorIsNil)
}
