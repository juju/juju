// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package firewaller_test

import (
	"github.com/golang/mock/gomock"
	"github.com/juju/errors"
	"github.com/juju/names/v4"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	apitesting "github.com/juju/juju/v2/api/testing"
	"github.com/juju/juju/v2/apiserver/common"
	"github.com/juju/juju/v2/apiserver/facades/controller/firewaller"
	"github.com/juju/juju/v2/apiserver/facades/controller/firewaller/mocks"
	apiservertesting "github.com/juju/juju/v2/apiserver/testing"
	"github.com/juju/juju/v2/core/network"
	"github.com/juju/juju/v2/core/network/firewall"
	corefirewall "github.com/juju/juju/v2/core/network/firewall"
	"github.com/juju/juju/v2/core/status"
	"github.com/juju/juju/v2/rpc/params"
	"github.com/juju/juju/v2/state"
	coretesting "github.com/juju/juju/v2/testing"
)

var _ = gc.Suite(&RemoteFirewallerSuite{})

type RemoteFirewallerSuite struct {
	coretesting.BaseSuite

	resources  *common.Resources
	authorizer *apiservertesting.FakeAuthorizer

	st  *mocks.MockState
	cc  *mocks.MockControllerConfigAPI
	api *firewaller.FirewallerAPIV4
}

func (s *RemoteFirewallerSuite) SetUpTest(c *gc.C) {
	s.BaseSuite.SetUpTest(c)

	s.resources = common.NewResources()
	s.AddCleanup(func(_ *gc.C) { s.resources.StopAll() })

	s.authorizer = &apiservertesting.FakeAuthorizer{
		Tag:        names.NewMachineTag("0"),
		Controller: true,
	}
}

func (s *RemoteFirewallerSuite) setup(c *gc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)

	s.st = mocks.NewMockState(ctrl)
	s.cc = mocks.NewMockControllerConfigAPI(ctrl)
	api, err := firewaller.NewFirewallerAPI(s.st, s.resources, s.authorizer, &mockCloudSpecAPI{})
	c.Assert(err, jc.ErrorIsNil)
	s.api = &firewaller.FirewallerAPIV4{FirewallerAPIV3: api, ControllerConfigAPI: s.cc}
	return ctrl
}

func (s *RemoteFirewallerSuite) TestWatchIngressAddressesForRelations(c *gc.C) {
	defer s.setup(c).Finish()

	db2Relation := newMockRelation(123)
	s.st.EXPECT().ModelUUID().Return(coretesting.ModelTag.Id()).AnyTimes()
	s.st.EXPECT().KeyRelation("remote-db2:db django:db").Return(db2Relation, nil)

	result, err := s.api.WatchIngressAddressesForRelations(
		params.Entities{Entities: []params.Entity{{
			Tag: names.NewRelationTag("remote-db2:db django:db").String(),
		}}},
	)

	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result.Results, gc.HasLen, 1)
	c.Assert(result.Results[0].Changes, jc.SameContents, []string{"1.2.3.4/32"})
	c.Assert(result.Results[0].Error, gc.IsNil)
	c.Assert(result.Results[0].StringsWatcherId, gc.Equals, "1")

	resource := s.resources.Get("1")
	c.Assert(resource, gc.NotNil)
	c.Assert(resource, gc.Implements, new(state.StringsWatcher))
}

func (s *RemoteFirewallerSuite) TestMacaroonForRelations(c *gc.C) {
	defer s.setup(c).Finish()

	mac, err := apitesting.NewMacaroon("apimac")
	c.Assert(err, jc.ErrorIsNil)
	entity := names.NewRelationTag("mysql:db wordpress:db")
	s.st.EXPECT().GetMacaroon(entity).Return(mac, nil)

	result, err := s.api.MacaroonForRelations(
		params.Entities{Entities: []params.Entity{{
			Tag: entity.String(),
		}}},
	)

	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result.Results, gc.HasLen, 1)
	c.Assert(result.Results[0].Error, gc.IsNil)
	c.Assert(result.Results[0].Result, jc.DeepEquals, mac)
}

func (s *RemoteFirewallerSuite) TestSetRelationStatus(c *gc.C) {
	defer s.setup(c).Finish()

	db2Relation := newMockRelation(123)
	entity := names.NewRelationTag("remote-db2:db django:db")
	s.st.EXPECT().ModelUUID().Return(coretesting.ModelTag.Id()).AnyTimes()
	s.st.EXPECT().KeyRelation("remote-db2:db django:db").Return(db2Relation, nil)

	result, err := s.api.SetRelationsStatus(
		params.SetStatus{Entities: []params.EntityStatusArgs{{
			Tag:    entity.String(),
			Status: "suspended",
			Info:   "a message",
		}}})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result.Results, gc.HasLen, 1)
	c.Assert(result.Results[0].Error, gc.IsNil)
	c.Assert(db2Relation.status, jc.DeepEquals, status.StatusInfo{Status: status.Suspended, Message: "a message"})
}

func (s *RemoteFirewallerSuite) TestFirewallRules(c *gc.C) {
	defer s.setup(c).Finish()

	rule := state.NewFirewallRule(firewall.JujuApplicationOfferRule, []string{"192.168.0.0/16"})
	s.st.EXPECT().FirewallRule(corefirewall.WellKnownServiceType(params.JujuApplicationOfferRule)).Return(&rule, nil)
	s.st.EXPECT().FirewallRule(corefirewall.WellKnownServiceType(params.SSHRule)).Return(nil, errors.NotFoundf("firewall rule for %q", params.SSHRule))

	result, err := s.api.FirewallRules(params.KnownServiceArgs{
		KnownServices: []params.KnownServiceValue{params.JujuApplicationOfferRule, params.SSHRule}},
	)

	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result.Rules, gc.HasLen, 1)
	c.Assert(result.Rules[0].KnownService, gc.Equals, params.KnownServiceValue("juju-application-offer"))
	c.Assert(result.Rules[0].WhitelistCIDRS, jc.SameContents, []string{"192.168.0.0/16"})
}

var _ = gc.Suite(&FirewallerSuite{})

type FirewallerSuite struct {
	coretesting.BaseSuite

	resources  *common.Resources
	authorizer *apiservertesting.FakeAuthorizer

	st  *mocks.MockState
	cc  *mocks.MockControllerConfigAPI
	api *firewaller.FirewallerAPIV6
}

func (s *FirewallerSuite) SetUpTest(c *gc.C) {
	s.BaseSuite.SetUpTest(c)

	s.resources = common.NewResources()
	s.AddCleanup(func(_ *gc.C) { s.resources.StopAll() })

	s.authorizer = &apiservertesting.FakeAuthorizer{
		Tag:        names.NewMachineTag("0"),
		Controller: true,
	}
}

func (s *FirewallerSuite) setup(c *gc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)

	s.st = mocks.NewMockState(ctrl)
	s.cc = mocks.NewMockControllerConfigAPI(ctrl)
	api, err := firewaller.NewFirewallerAPI(s.st, s.resources, s.authorizer, &mockCloudSpecAPI{})
	c.Assert(err, jc.ErrorIsNil)
	s.api = &firewaller.FirewallerAPIV6{
		&firewaller.FirewallerAPIV5{
			&firewaller.FirewallerAPIV4{
				FirewallerAPIV3:     api,
				ControllerConfigAPI: s.cc,
			},
		},
	}
	return ctrl
}

func (s *FirewallerSuite) TestOpenedMachinePortRanges(c *gc.C) {
	defer s.setup(c).Finish()

	// Set up our mocks
	mockMachine := newMockMachine("0")
	mockMachine.openedPortRanges = newMockMachinePortRanges(
		newMockUnitPortRanges(
			"wordpress/0",
			network.GroupedPortRanges{
				"": []network.PortRange{
					network.MustParsePortRange("80/tcp"),
				},
			},
		),
		newMockUnitPortRanges(
			"mysql/0",
			network.GroupedPortRanges{
				"foo": []network.PortRange{
					network.MustParsePortRange("3306/tcp"),
				},
			},
		),
	)
	spaceInfos := network.SpaceInfos{
		{ID: network.AlphaSpaceId, Name: "alpha", Subnets: []network.SubnetInfo{
			{ID: "11", CIDR: "10.0.0.0/24"},
			{ID: "12", CIDR: "10.0.1.0/24"},
		}},
		{ID: "42", Name: "questions-about-the-universe", Subnets: []network.SubnetInfo{
			{ID: "13", CIDR: "192.168.0.0/24"},
			{ID: "14", CIDR: "192.168.1.0/24"},
		}},
	}
	applicationEndpointBindings := map[string]map[string]string{
		"mysql": {
			"":    network.AlphaSpaceId,
			"foo": "42",
		},
		"wordpress": {
			"":           network.AlphaSpaceId,
			"monitoring": network.AlphaSpaceId,
			"web":        "42",
		},
	}
	s.st.EXPECT().Machine("0").Return(mockMachine, nil)
	s.st.EXPECT().SpaceInfos().Return(spaceInfos, nil)
	s.st.EXPECT().AllEndpointBindings().Return(applicationEndpointBindings, nil)

	// Test call output
	req := params.Entities{
		Entities: []params.Entity{
			{Tag: names.NewMachineTag("0").String()},
		},
	}
	res, err := s.api.OpenedMachinePortRanges(req)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(res.Results, gc.HasLen, 1)

	c.Assert(res.Results[0].Error, gc.IsNil)
	c.Assert(res.Results[0].UnitPortRanges, gc.DeepEquals, map[string][]params.OpenUnitPortRanges{
		"unit-wordpress-0": {
			{
				Endpoint:    "",
				SubnetCIDRs: []string{"10.0.0.0/24", "10.0.1.0/24", "192.168.0.0/24", "192.168.1.0/24"},
				PortRanges: []params.PortRange{
					params.FromNetworkPortRange(network.MustParsePortRange("80/tcp")),
				},
			},
		},
		"unit-mysql-0": {
			{
				Endpoint:    "foo",
				SubnetCIDRs: []string{"192.168.0.0/24", "192.168.1.0/24"},
				PortRanges: []params.PortRange{
					params.FromNetworkPortRange(network.MustParsePortRange("3306/tcp")),
				},
			},
		},
	})
}

func (s *FirewallerSuite) TestAllSpaceInfos(c *gc.C) {
	defer s.setup(c).Finish()

	// Set up our mocks
	spaceInfos := network.SpaceInfos{
		{
			ID:         "42",
			Name:       "questions-about-the-universe",
			ProviderId: "provider-id-2",
			Subnets: []network.SubnetInfo{
				{
					ID:                "13",
					CIDR:              "1.168.1.0/24",
					ProviderId:        "provider-subnet-id-1",
					ProviderSpaceId:   "provider-space-id-1",
					ProviderNetworkId: "provider-network-id-1",
					VLANTag:           42,
					AvailabilityZones: []string{"az1", "az2"},
					SpaceID:           "42",
					SpaceName:         "questions-about-the-universe",
					FanInfo: &network.FanCIDRs{
						FanLocalUnderlay: "192.168.0.0/16",
						FanOverlay:       "1.0.0.0/8",
					},
					IsPublic: true,
				},
			}},
		{ID: "99", Name: "special", Subnets: []network.SubnetInfo{
			{ID: "999", CIDR: "192.168.2.0/24"},
		}},
	}
	s.st.EXPECT().SpaceInfos().Return(spaceInfos, nil)

	// Test call output
	req := params.SpaceInfosParams{
		FilterBySpaceIDs: []string{network.AlphaSpaceId, "42"},
	}
	res, err := s.api.SpaceInfos(req)
	c.Assert(err, jc.ErrorIsNil)

	// Hydrate a network.SpaceInfos from the response
	gotSpaceInfos := params.ToNetworkSpaceInfos(res)
	c.Assert(gotSpaceInfos, gc.DeepEquals, spaceInfos[0:1], gc.Commentf("expected to get back a filtered list of the space infos"))
}
