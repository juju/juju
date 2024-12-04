// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"context"

	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gomock "go.uber.org/mock/gomock"
	gc "gopkg.in/check.v1"

	coreerrors "github.com/juju/juju/core/errors"
	"github.com/juju/juju/core/network"
	"github.com/juju/juju/internal/errors"
)

type subnetSuite struct {
	testing.IsolationSuite

	st *MockState
}

var _ = gc.Suite(&subnetSuite{})

func (s *subnetSuite) setupMocks(c *gc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)

	s.st = NewMockState(ctrl)

	return ctrl
}

func (s *subnetSuite) TestFailAddSubnet(c *gc.C) {
	defer s.setupMocks(c).Finish()

	subnetInfo := network.SubnetInfo{
		CIDR:              "192.168.0.0/20",
		ProviderId:        "provider-id-0",
		ProviderNetworkId: "provider-network-id-0",
		AvailabilityZones: []string{"az0"},
	}

	// Verify that the passed subnetInfo matches and return an error.
	s.st.EXPECT().AddSubnet(gomock.Any(), gomock.Any()).
		DoAndReturn(
			func(
				ctx context.Context,
				subnet network.SubnetInfo,
			) error {
				c.Assert(subnet.CIDR, gc.Equals, subnetInfo.CIDR)
				c.Assert(subnet.ProviderId, gc.Equals, subnetInfo.ProviderId)
				c.Assert(subnet.ProviderNetworkId, gc.Equals, subnetInfo.ProviderNetworkId)
				c.Assert(subnet.AvailabilityZones, jc.SameContents, subnetInfo.AvailabilityZones)
				return errors.New("boom")
			})

	_, err := NewService(s.st, nil).AddSubnet(context.Background(), subnetInfo)
	c.Assert(err, gc.ErrorMatches, "boom")
}

func (s *subnetSuite) TestAddSubnet(c *gc.C) {
	defer s.setupMocks(c).Finish()

	subnetInfo := network.SubnetInfo{
		CIDR:              "192.168.0.0/20",
		ProviderId:        "provider-id-0",
		ProviderNetworkId: "provider-network-id-0",
		AvailabilityZones: []string{"az0"},
	}

	var expectedUUID network.Id
	// Verify that the passed subnetInfo matches and don't return an error.
	s.st.EXPECT().AddSubnet(gomock.Any(), gomock.Any()).
		Do(
			func(
				ctx context.Context,
				subnet network.SubnetInfo,
			) error {
				c.Assert(subnet.CIDR, gc.Equals, subnetInfo.CIDR)
				c.Assert(subnet.ProviderId, gc.Equals, subnetInfo.ProviderId)
				c.Assert(subnet.ProviderNetworkId, gc.Equals, subnetInfo.ProviderNetworkId)
				c.Assert(subnet.AvailabilityZones, jc.SameContents, subnetInfo.AvailabilityZones)
				expectedUUID = subnet.ID
				return nil
			})

	returnedUUID, err := NewService(s.st, nil).AddSubnet(context.Background(), subnetInfo)
	c.Assert(err, jc.ErrorIsNil)
	// Verify that the passed UUID is also returned.
	c.Assert(returnedUUID, gc.Equals, expectedUUID)
}

func (s *subnetSuite) TestRetrieveAllSubnets(c *gc.C) {
	defer s.setupMocks(c).Finish()

	subnetInfos := network.SubnetInfos{
		{
			CIDR:              "192.168.0.0/20",
			ProviderId:        "provider-id-0",
			ProviderNetworkId: "provider-network-id-0",
			AvailabilityZones: []string{"az0"},
		},
	}
	s.st.EXPECT().GetAllSubnets(gomock.Any()).Return(subnetInfos, nil)
	subnets, err := NewService(s.st, nil).GetAllSubnets(context.Background())
	c.Assert(err, jc.ErrorIsNil)
	c.Check(subnets, jc.SameContents, subnetInfos)
}

func (s *subnetSuite) TestRetrieveSubnetByID(c *gc.C) {
	defer s.setupMocks(c).Finish()

	s.st.EXPECT().GetSubnet(gomock.Any(), "subnet0")
	_, err := NewService(s.st, nil).Subnet(context.Background(), "subnet0")
	c.Assert(err, jc.ErrorIsNil)
}

func (s *subnetSuite) TestFailRetrieveSubnetByID(c *gc.C) {
	defer s.setupMocks(c).Finish()

	s.st.EXPECT().GetSubnet(gomock.Any(), "unknown-subnet").
		Return(nil, errors.Errorf("subnet %q %w", "unknown-subnet", coreerrors.NotFound))
	_, err := NewService(s.st, nil).Subnet(context.Background(), "unknown-subnet")
	c.Assert(err, gc.ErrorMatches, "subnet \"unknown-subnet\" not found")
}

func (s *subnetSuite) TestRetrieveSubnetByCIDRs(c *gc.C) {
	defer s.setupMocks(c).Finish()

	s.st.EXPECT().GetSubnetsByCIDR(gomock.Any(), "192.168.1.1", "10.0.0.1")
	_, err := NewService(s.st, nil).SubnetsByCIDR(context.Background(), "192.168.1.1", "10.0.0.1")
	c.Assert(err, jc.ErrorIsNil)
}

func (s *subnetSuite) TestFailRetrieveSubnetByCIDRs(c *gc.C) {
	defer s.setupMocks(c).Finish()

	s.st.EXPECT().GetSubnetsByCIDR(gomock.Any(), "192.168.1.1", "10.0.0.1").
		Return(nil, errors.New("querying subnets"))
	_, err := NewService(s.st, nil).SubnetsByCIDR(context.Background(), "192.168.1.1", "10.0.0.1")
	c.Assert(err, gc.ErrorMatches, "querying subnets")
}

func (s *subnetSuite) TestUpdateSubnet(c *gc.C) {
	defer s.setupMocks(c).Finish()

	s.st.EXPECT().UpdateSubnet(gomock.Any(), "subnet0", "space0")
	err := NewService(s.st, nil).UpdateSubnet(context.Background(), "subnet0", "space0")
	c.Assert(err, jc.ErrorIsNil)
}

func (s *subnetSuite) TestFailUpdateSubnet(c *gc.C) {
	defer s.setupMocks(c).Finish()

	s.st.EXPECT().UpdateSubnet(gomock.Any(), "unknown-subnet", "space0").
		Return(errors.Errorf("subnet %q %w", "unknown-subnet", coreerrors.NotFound))
	err := NewService(s.st, nil).UpdateSubnet(context.Background(), "unknown-subnet", "space0")
	c.Assert(err, gc.ErrorMatches, "subnet \"unknown-subnet\" not found")
}

func (s *subnetSuite) TestRemoveSubnet(c *gc.C) {
	defer s.setupMocks(c).Finish()

	s.st.EXPECT().DeleteSubnet(gomock.Any(), "subnet0")
	err := NewService(s.st, nil).RemoveSubnet(context.Background(), "subnet0")
	c.Assert(err, jc.ErrorIsNil)
}

func (s *subnetSuite) TestFailRemoveSubnet(c *gc.C) {
	defer s.setupMocks(c).Finish()

	s.st.EXPECT().DeleteSubnet(gomock.Any(), "unknown-subnet").
		Return(errors.Errorf("subnet %q %w", "unknown-subnet", coreerrors.NotFound))
	err := NewService(s.st, nil).RemoveSubnet(context.Background(), "unknown-subnet")
	c.Assert(err, gc.ErrorMatches, "subnet \"unknown-subnet\" not found")
}
