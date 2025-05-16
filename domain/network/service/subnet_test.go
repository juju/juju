// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"context"
	stdtesting "testing"

	"github.com/juju/tc"
	gomock "go.uber.org/mock/gomock"

	coreerrors "github.com/juju/juju/core/errors"
	"github.com/juju/juju/core/network"
	"github.com/juju/juju/internal/errors"
	"github.com/juju/juju/internal/testhelpers"
)

type subnetSuite struct {
	testhelpers.IsolationSuite

	st *MockState
}

func TestSubnetSuite(t *stdtesting.T) { tc.Run(t, &subnetSuite{}) }
func (s *subnetSuite) setupMocks(c *tc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)

	s.st = NewMockState(ctrl)

	return ctrl
}

func (s *subnetSuite) TestFailAddSubnet(c *tc.C) {
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
				c.Assert(subnet.CIDR, tc.Equals, subnetInfo.CIDR)
				c.Assert(subnet.ProviderId, tc.Equals, subnetInfo.ProviderId)
				c.Assert(subnet.ProviderNetworkId, tc.Equals, subnetInfo.ProviderNetworkId)
				c.Assert(subnet.AvailabilityZones, tc.SameContents, subnetInfo.AvailabilityZones)
				return errors.New("boom")
			})

	_, err := NewService(s.st, nil).AddSubnet(c.Context(), subnetInfo)
	c.Assert(err, tc.ErrorMatches, "boom")
}

func (s *subnetSuite) TestAddSubnet(c *tc.C) {
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
				c.Assert(subnet.CIDR, tc.Equals, subnetInfo.CIDR)
				c.Assert(subnet.ProviderId, tc.Equals, subnetInfo.ProviderId)
				c.Assert(subnet.ProviderNetworkId, tc.Equals, subnetInfo.ProviderNetworkId)
				c.Assert(subnet.AvailabilityZones, tc.SameContents, subnetInfo.AvailabilityZones)
				expectedUUID = subnet.ID
				return nil
			})

	returnedUUID, err := NewService(s.st, nil).AddSubnet(c.Context(), subnetInfo)
	c.Assert(err, tc.ErrorIsNil)
	// Verify that the passed UUID is also returned.
	c.Assert(returnedUUID, tc.Equals, expectedUUID)
}

func (s *subnetSuite) TestRetrieveAllSubnets(c *tc.C) {
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
	subnets, err := NewService(s.st, nil).GetAllSubnets(c.Context())
	c.Assert(err, tc.ErrorIsNil)
	c.Check(subnets, tc.SameContents, subnetInfos)
}

func (s *subnetSuite) TestRetrieveSubnetByID(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.st.EXPECT().GetSubnet(gomock.Any(), "subnet0")
	_, err := NewService(s.st, nil).Subnet(c.Context(), "subnet0")
	c.Assert(err, tc.ErrorIsNil)
}

func (s *subnetSuite) TestFailRetrieveSubnetByID(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.st.EXPECT().GetSubnet(gomock.Any(), "unknown-subnet").
		Return(nil, errors.Errorf("subnet %q %w", "unknown-subnet", coreerrors.NotFound))
	_, err := NewService(s.st, nil).Subnet(c.Context(), "unknown-subnet")
	c.Assert(err, tc.ErrorMatches, "subnet \"unknown-subnet\" not found")
}

func (s *subnetSuite) TestRetrieveSubnetByCIDRs(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.st.EXPECT().GetSubnetsByCIDR(gomock.Any(), "192.168.1.1", "10.0.0.1")
	_, err := NewService(s.st, nil).SubnetsByCIDR(c.Context(), "192.168.1.1", "10.0.0.1")
	c.Assert(err, tc.ErrorIsNil)
}

func (s *subnetSuite) TestFailRetrieveSubnetByCIDRs(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.st.EXPECT().GetSubnetsByCIDR(gomock.Any(), "192.168.1.1", "10.0.0.1").
		Return(nil, errors.New("querying subnets"))
	_, err := NewService(s.st, nil).SubnetsByCIDR(c.Context(), "192.168.1.1", "10.0.0.1")
	c.Assert(err, tc.ErrorMatches, "querying subnets")
}

func (s *subnetSuite) TestUpdateSubnet(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.st.EXPECT().UpdateSubnet(gomock.Any(), "subnet0", "space0")
	err := NewService(s.st, nil).UpdateSubnet(c.Context(), "subnet0", "space0")
	c.Assert(err, tc.ErrorIsNil)
}

func (s *subnetSuite) TestFailUpdateSubnet(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.st.EXPECT().UpdateSubnet(gomock.Any(), "unknown-subnet", "space0").
		Return(errors.Errorf("subnet %q %w", "unknown-subnet", coreerrors.NotFound))
	err := NewService(s.st, nil).UpdateSubnet(c.Context(), "unknown-subnet", "space0")
	c.Assert(err, tc.ErrorMatches, "subnet \"unknown-subnet\" not found")
}

func (s *subnetSuite) TestRemoveSubnet(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.st.EXPECT().DeleteSubnet(gomock.Any(), "subnet0")
	err := NewService(s.st, nil).RemoveSubnet(c.Context(), "subnet0")
	c.Assert(err, tc.ErrorIsNil)
}

func (s *subnetSuite) TestFailRemoveSubnet(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.st.EXPECT().DeleteSubnet(gomock.Any(), "unknown-subnet").
		Return(errors.Errorf("subnet %q %w", "unknown-subnet", coreerrors.NotFound))
	err := NewService(s.st, nil).RemoveSubnet(c.Context(), "unknown-subnet")
	c.Assert(err, tc.ErrorMatches, "subnet \"unknown-subnet\" not found")
}
