// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"context"
	"testing"

	gomock "github.com/canonical/gomock/gomock"
	"github.com/juju/tc"

	coreerrors "github.com/juju/juju/core/errors"
	"github.com/juju/juju/core/network"
	networktesting "github.com/juju/juju/core/network/testing"
	domainnetwork "github.com/juju/juju/domain/network"
	"github.com/juju/juju/internal/errors"
	"github.com/juju/juju/internal/testhelpers"
)

type subnetSuite struct {
	testhelpers.IsolationSuite

	st *MockState
}

func TestSubnetSuite(t *testing.T) {
	tc.Run(t, &subnetSuite{})
}

func (s *subnetSuite) setupMocks(c *tc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)

	s.st = NewMockState(ctrl)

	return ctrl
}

func (s *subnetSuite) TestFailImportSubnet(c *tc.C) {
	defer s.setupMocks(c).Finish()

	args := domainnetwork.ImportSubnetArgs{
		UUID:              domainnetwork.SubnetUUID("subnet-uuid-0"),
		CIDR:              "192.168.0.0/20",
		ProviderId:        "provider-id-0",
		ProviderNetworkId: "provider-network-id-0",
		AvailabilityZones: []string{"az0"},
	}

	// Verify that the passed args match and return an error.
	s.st.EXPECT().ImportSubnets(gomock.Any(), gomock.Any()).
		DoAndReturn(
			func(
				ctx context.Context,
				subnets []domainnetwork.ImportSubnetArgs,
			) error {
				c.Assert(subnets, tc.HasLen, 1)
				got := subnets[0]
				c.Assert(got.UUID, tc.Equals, args.UUID)
				c.Assert(got.CIDR, tc.Equals, args.CIDR)
				c.Assert(got.ProviderId, tc.Equals, args.ProviderId)
				c.Assert(got.ProviderNetworkId, tc.Equals, args.ProviderNetworkId)
				c.Assert(got.AvailabilityZones, tc.SameContents, args.AvailabilityZones)
				return errors.New("boom")
			})

	err := NewMigrationService(s.st, nil).ImportSubnets(c.Context(), []domainnetwork.ImportSubnetArgs{args})
	c.Assert(err, tc.ErrorMatches, "boom")
}

func (s *subnetSuite) TestImportSubnet(c *tc.C) {
	defer s.setupMocks(c).Finish()

	args := domainnetwork.ImportSubnetArgs{
		UUID:              domainnetwork.SubnetUUID("subnet-uuid-0"),
		CIDR:              "192.168.0.0/20",
		ProviderId:        "provider-id-0",
		ProviderNetworkId: "provider-network-id-0",
		AvailabilityZones: []string{"az0"},
	}

	// Verify that the passed args match and don't return an error.
	s.st.EXPECT().ImportSubnets(gomock.Any(), gomock.Any()).
		DoAndReturn(
			func(
				ctx context.Context,
				subnets []domainnetwork.ImportSubnetArgs,
			) error {
				c.Assert(subnets, tc.HasLen, 1)
				got := subnets[0]
				c.Assert(got, tc.DeepEquals, args)
				return nil
			})

	err := NewMigrationService(s.st, nil).ImportSubnets(c.Context(), []domainnetwork.ImportSubnetArgs{args})
	c.Assert(err, tc.ErrorIsNil)
}

func (s *subnetSuite) TestImportSubnetEmptyArgs(c *tc.C) {
	defer s.setupMocks(c).Finish()

	// Verify that ImportSubnets with an empty slice returns nil without
	// calling the state layer.
	err := NewMigrationService(s.st, nil).ImportSubnets(c.Context(), nil)
	c.Assert(err, tc.ErrorIsNil)
}

func (s *subnetSuite) TestRetrieveAllSubnets(c *tc.C) {
	defer s.setupMocks(c).Finish()

	subnetInfos := domainnetwork.SubnetInfos{
		{
			UUID:              domainnetwork.SubnetUUID("subnet-uuid-0"),
			CIDR:              "192.168.0.0/20",
			ProviderId:        "provider-id-0",
			ProviderNetworkId: "provider-network-id-0",
			AvailabilityZones: []string{"az0"},
		},
	}
	s.st.EXPECT().GetAllSubnets(gomock.Any()).Return(subnetInfos, nil)
	subnets, err := NewService(s.st, nil).GetAllSubnets(c.Context())
	c.Assert(err, tc.ErrorIsNil)
	c.Check(subnets, tc.HasLen, 1)
	c.Check(subnets[0].ID, tc.Equals, network.Id("subnet-uuid-0"))
	c.Check(subnets[0].CIDR, tc.Equals, "192.168.0.0/20")
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

	spUUID := networktesting.GenSpaceUUID(c)
	s.st.EXPECT().UpdateSubnet(gomock.Any(), "subnet0", spUUID)
	err := NewService(s.st, nil).UpdateSubnet(c.Context(), "subnet0", spUUID)
	c.Assert(err, tc.ErrorIsNil)
}

func (s *subnetSuite) TestFailUpdateSubnet(c *tc.C) {
	defer s.setupMocks(c).Finish()

	spUUID := networktesting.GenSpaceUUID(c)
	s.st.EXPECT().UpdateSubnet(gomock.Any(), "unknown-subnet", spUUID).
		Return(errors.Errorf("subnet %q %w", "unknown-subnet", coreerrors.NotFound))
	err := NewService(s.st, nil).UpdateSubnet(c.Context(), "unknown-subnet", spUUID)
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
