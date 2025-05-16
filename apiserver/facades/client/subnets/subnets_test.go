// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package subnets

import (
	stdtesting "testing"

	"github.com/juju/errors"
	"github.com/juju/names/v6"
	"github.com/juju/tc"
	"go.uber.org/mock/gomock"

	"github.com/juju/juju/apiserver/facade/facadetest"
	facademocks "github.com/juju/juju/apiserver/facade/mocks"
	apiservertesting "github.com/juju/juju/apiserver/testing"
	"github.com/juju/juju/core/life"
	modeltesting "github.com/juju/juju/core/model/testing"
	"github.com/juju/juju/core/network"
	loggertesting "github.com/juju/juju/internal/logger/testing"
	"github.com/juju/juju/internal/testing"
	"github.com/juju/juju/rpc/params"
)

// SubnetSuite uses mocks for testing.
// All future facade tests should be added to this suite.
type SubnetsSuite struct {
	testing.BaseSuite
	mockResource       *facademocks.MockResources
	mockAuthorizer     *facademocks.MockAuthorizer
	mockNetworkService *MockNetworkService

	facade *API
}

func TestSubnetsSuite(t *stdtesting.T) { tc.Run(t, &SubnetsSuite{}) }
func (s *SubnetsSuite) TestAuthDenied(c *tc.C) {
	_, err := newAPI(facadetest.ModelContext{
		Auth_: apiservertesting.FakeAuthorizer{
			Tag: names.NewMachineTag("1"),
		},
	})
	c.Assert(err, tc.ErrorMatches, "permission denied")
}

func (s *SubnetsSuite) TestSubnetsByCIDR(c *tc.C) {
	ctrl := s.setUpMocks(c)
	defer ctrl.Finish()

	cidrs := []string{"10.10.10.0/24", "10.10.20.0/24", "not-a-cidr"}

	subnet := network.SubnetInfo{
		ID:                "1",
		CIDR:              "10.10.20.0/24",
		SpaceName:         "space",
		VLANTag:           0,
		ProviderId:        network.Id("0"),
		ProviderNetworkId: network.Id("1"),
		AvailabilityZones: []string{"bar", "bam"},
	}

	gomock.InOrder(
		s.mockNetworkService.EXPECT().SubnetsByCIDR(gomock.Any(), cidrs[0]).Return(nil, errors.New("bad-mongo")),
		s.mockNetworkService.EXPECT().SubnetsByCIDR(gomock.Any(), cidrs[1]).Return([]network.SubnetInfo{subnet}, nil),
		// No call for cidrs[2]; the input is invalidated.
	)

	arg := params.CIDRParams{CIDRS: cidrs}
	res, err := s.facade.SubnetsByCIDR(c.Context(), arg)
	c.Assert(err, tc.ErrorIsNil)

	results := res.Results
	c.Assert(results, tc.HasLen, 3)

	c.Check(results[0].Error.Message, tc.Equals, "bad-mongo")
	c.Check(results[1].Subnets, tc.HasLen, 1)
	c.Check(results[2].Error.Message, tc.Equals, `CIDR "not-a-cidr" not valid`)
}

func (s *SubnetsSuite) setUpMocks(c *tc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)

	s.mockResource = facademocks.NewMockResources(ctrl)
	s.mockAuthorizer = facademocks.NewMockAuthorizer(ctrl)
	s.mockAuthorizer.EXPECT().HasPermission(gomock.Any(), gomock.Any(), gomock.Any()).Return(nil).AnyTimes()

	s.mockNetworkService = NewMockNetworkService(ctrl)

	tag := names.NewModelTag(modeltesting.GenModelUUID(c).String())
	s.facade = newAPIWithBacking(
		tag,
		s.mockResource,
		s.mockAuthorizer,
		loggertesting.WrapCheckLog(c),
		s.mockNetworkService,
	)
	return ctrl
}

type stubZone struct {
	ZoneName      string
	ZoneAvailable bool
}

var _ network.AvailabilityZone = (*stubZone)(nil)

func (f *stubZone) Name() string {
	return f.ZoneName
}

func (f *stubZone) Available() bool {
	return f.ZoneAvailable
}

var zoneResults = network.AvailabilityZones{
	&stubZone{"zone1", true},
	&stubZone{"zone2", false},
}

// GoString implements fmt.GoStringer.
func (s *SubnetsSuite) TestAllZonesUsesBackingZonesWhenAvailable(c *tc.C) {
	defer s.setUpMocks(c).Finish()

	s.mockNetworkService.EXPECT().GetProviderAvailabilityZones(gomock.Any()).Return(zoneResults, nil)

	results, err := s.facade.AllZones(c.Context())
	c.Assert(err, tc.ErrorIsNil)

	expected := make([]params.ZoneResult, len(zoneResults))
	for i, zone := range zoneResults {
		expected[i].Name = zone.Name()
		expected[i].Available = zone.Available()
	}
	c.Assert(results, tc.DeepEquals, params.ZoneResults{Results: expected})
}

func (s *SubnetsSuite) TestListSubnetsAndFiltering(c *tc.C) {
	defer s.setUpMocks(c).Finish()
	expected := []params.Subnet{{
		CIDR:              "10.10.0.0/24",
		ProviderId:        "sn-zadf00d",
		ProviderNetworkId: "godspeed",
		VLANTag:           0,
		Life:              life.Alive,
		SpaceTag:          "space-private",
		Zones:             []string{"zone1"},
	}, {
		CIDR:              "2001:db8::/32",
		ProviderId:        "sn-ipv6",
		ProviderNetworkId: "",
		VLANTag:           0,
		Life:              life.Alive,
		SpaceTag:          "space-dmz",
		Zones:             []string{"zone1", "zone3"},
	}}
	// No filtering.
	args := params.SubnetsFilters{}
	s.mockNetworkService.EXPECT().GetAllSubnets(gomock.Any()).Return(
		network.SubnetInfos{
			{
				CIDR:              "10.10.0.0/24",
				ProviderId:        "sn-zadf00d",
				ProviderNetworkId: "godspeed",
				VLANTag:           0,
				Life:              life.Alive,
				SpaceName:         "private",
				AvailabilityZones: []string{"zone1"},
			}, {
				CIDR:              "2001:db8::/32",
				ProviderId:        "sn-ipv6",
				ProviderNetworkId: "",
				VLANTag:           0,
				Life:              life.Alive,
				SpaceName:         "dmz",
				AvailabilityZones: []string{"zone1", "zone3"},
			},
		}, nil).Times(4)
	subnets, err := s.facade.ListSubnets(c.Context(), args)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(subnets.Results, tc.DeepEquals, expected)

	// Filter by space only.
	args.SpaceTag = "space-dmz"
	subnets, err = s.facade.ListSubnets(c.Context(), args)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(subnets.Results, tc.DeepEquals, expected[1:])

	// Filter by zone only.
	args.SpaceTag = ""
	args.Zone = "zone3"
	subnets, err = s.facade.ListSubnets(c.Context(), args)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(subnets.Results, tc.DeepEquals, expected[1:])

	// Filter by both space and zone.
	args.SpaceTag = "space-private"
	args.Zone = "zone1"
	subnets, err = s.facade.ListSubnets(c.Context(), args)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(subnets.Results, tc.DeepEquals, expected[:1])
}

func (s *SubnetsSuite) TestListSubnetsInvalidSpaceTag(c *tc.C) {
	defer s.setUpMocks(c).Finish()
	args := params.SubnetsFilters{SpaceTag: "invalid"}
	s.mockNetworkService.EXPECT().GetAllSubnets(gomock.Any())
	_, err := s.facade.ListSubnets(c.Context(), args)
	c.Assert(err, tc.ErrorMatches, `"invalid" is not a valid tag`)
}

func (s *SubnetsSuite) TestListSubnetsAllSubnetError(c *tc.C) {
	defer s.setUpMocks(c).Finish()
	boom := errors.New("no subnets for you")
	s.mockNetworkService.EXPECT().GetAllSubnets(gomock.Any()).Return(nil, boom)
	_, err := s.facade.ListSubnets(c.Context(), params.SubnetsFilters{})
	c.Assert(err, tc.ErrorMatches, "no subnets for you")
}
