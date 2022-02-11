// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package firewaller_test

import (
	"github.com/juju/names/v4"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	apitesting "github.com/juju/juju/api/testing"
	"github.com/juju/juju/apiserver/common"
	"github.com/juju/juju/apiserver/facades/controller/firewaller"
	apiservertesting "github.com/juju/juju/apiserver/testing"
	"github.com/juju/juju/core/crossmodel"
	"github.com/juju/juju/core/network"
	"github.com/juju/juju/core/network/firewall"
	"github.com/juju/juju/core/status"
	"github.com/juju/juju/rpc/params"
	"github.com/juju/juju/state"
	coretesting "github.com/juju/juju/testing"
)

var _ = gc.Suite(&RemoteFirewallerSuite{})

type RemoteFirewallerSuite struct {
	coretesting.BaseSuite

	resources  *common.Resources
	authorizer *apiservertesting.FakeAuthorizer
	st         *mockState
	api        *firewaller.FirewallerAPIV4
}

func (s *RemoteFirewallerSuite) SetUpTest(c *gc.C) {
	s.BaseSuite.SetUpTest(c)

	s.resources = common.NewResources()
	s.AddCleanup(func(_ *gc.C) { s.resources.StopAll() })

	s.authorizer = &apiservertesting.FakeAuthorizer{
		Tag:        names.NewMachineTag("0"),
		Controller: true,
	}

	s.st = newMockState(coretesting.ModelTag.Id())
	api, err := firewaller.NewFirewallerAPI(s.st, s.resources, s.authorizer, &mockCloudSpecAPI{})
	c.Assert(err, jc.ErrorIsNil)
	s.api = &firewaller.FirewallerAPIV4{FirewallerAPIV3: api, ControllerConfigAPI: common.NewControllerConfig(s.st)}
}

func (s *RemoteFirewallerSuite) TestWatchIngressAddressesForRelations(c *gc.C) {
	db2Relation := newMockRelation(123)
	s.st.relations["remote-db2:db django:db"] = db2Relation

	result, err := s.api.WatchIngressAddressesForRelations(
		params.Entities{Entities: []params.Entity{{
			Tag: names.NewRelationTag("remote-db2:db django:db").String(),
		}}})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result.Results, gc.HasLen, 1)
	c.Assert(result.Results[0].Changes, jc.SameContents, []string{"1.2.3.4/32"})
	c.Assert(result.Results[0].Error, gc.IsNil)
	c.Assert(result.Results[0].StringsWatcherId, gc.Equals, "1")

	resource := s.resources.Get("1")
	c.Assert(resource, gc.NotNil)
	c.Assert(resource, gc.Implements, new(state.StringsWatcher))

	s.st.CheckCalls(c, []testing.StubCall{
		{"KeyRelation", []interface{}{"remote-db2:db django:db"}},
	})
}

func (s *RemoteFirewallerSuite) TestControllerAPIInfoForModels(c *gc.C) {
	controllerInfo := &mockControllerInfo{
		uuid: "some uuid",
		info: crossmodel.ControllerInfo{
			Addrs:  []string{"1.2.3.4/32"},
			CACert: coretesting.CACert,
		},
	}
	s.st.controllerInfo[coretesting.ModelTag.Id()] = controllerInfo
	result, err := s.api.ControllerAPIInfoForModels(
		params.Entities{Entities: []params.Entity{{
			Tag: coretesting.ModelTag.String(),
		}}})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result.Results, gc.HasLen, 1)
	c.Assert(result.Results[0].Addresses, jc.SameContents, []string{"1.2.3.4/32"})
	c.Assert(result.Results[0].Error, gc.IsNil)
	c.Assert(result.Results[0].CACert, gc.Equals, coretesting.CACert)
}

func (s *RemoteFirewallerSuite) TestMacaroonForRelations(c *gc.C) {
	mac, err := apitesting.NewMacaroon("apimac")
	c.Assert(err, jc.ErrorIsNil)
	entity := names.NewRelationTag("mysql:db wordpress:db")
	s.st.macaroons[entity] = mac
	result, err := s.api.MacaroonForRelations(
		params.Entities{Entities: []params.Entity{{
			Tag: entity.String(),
		}}})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result.Results, gc.HasLen, 1)
	c.Assert(result.Results[0].Error, gc.IsNil)
	c.Assert(result.Results[0].Result, jc.DeepEquals, mac)
}

func (s *RemoteFirewallerSuite) TestSetRelationStatus(c *gc.C) {
	db2Relation := newMockRelation(123)
	s.st.relations["remote-db2:db django:db"] = db2Relation
	entity := names.NewRelationTag("remote-db2:db django:db")
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
	rule := state.NewFirewallRule(firewall.JujuApplicationOfferRule, []string{"192.168.0.0/16"})
	s.st.firewallRules[firewall.JujuApplicationOfferRule] = &rule
	result, err := s.api.FirewallRules(params.KnownServiceArgs{
		KnownServices: []params.KnownServiceValue{params.JujuApplicationOfferRule, params.SSHRule}})
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
	st         *mockState
	api        *firewaller.FirewallerAPIV6
}

func (s *FirewallerSuite) SetUpTest(c *gc.C) {
	s.BaseSuite.SetUpTest(c)

	s.resources = common.NewResources()
	s.AddCleanup(func(_ *gc.C) { s.resources.StopAll() })

	s.authorizer = &apiservertesting.FakeAuthorizer{
		Tag:        names.NewMachineTag("0"),
		Controller: true,
	}

	s.st = newMockState(coretesting.ModelTag.Id())

	api, err := firewaller.NewFirewallerAPI(s.st, s.resources, s.authorizer, &mockCloudSpecAPI{})
	c.Assert(err, jc.ErrorIsNil)
	s.api = &firewaller.FirewallerAPIV6{
		&firewaller.FirewallerAPIV5{
			&firewaller.FirewallerAPIV4{
				FirewallerAPIV3:     api,
				ControllerConfigAPI: common.NewControllerConfig(newMockState(coretesting.ModelTag.Id())),
			},
		},
	}
}

func (s *FirewallerSuite) TestOpenedMachinePortRanges(c *gc.C) {
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
	s.st.machines["0"] = mockMachine
	s.st.spaceInfos = network.SpaceInfos{
		{ID: network.AlphaSpaceId, Name: "alpha", Subnets: []network.SubnetInfo{
			{ID: "11", CIDR: "10.0.0.0/24"},
			{ID: "12", CIDR: "10.0.1.0/24"},
		}},
		{ID: "42", Name: "questions-about-the-universe", Subnets: []network.SubnetInfo{
			{ID: "13", CIDR: "192.168.0.0/24"},
			{ID: "14", CIDR: "192.168.1.0/24"},
		}},
	}
	s.st.applicationEndpointBindings = map[string]map[string]string{
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
	// Set up our mocks
	s.st.spaceInfos = network.SpaceInfos{
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

	// Test call output
	req := params.SpaceInfosParams{
		FilterBySpaceIDs: []string{network.AlphaSpaceId, "42"},
	}
	res, err := s.api.SpaceInfos(req)
	c.Assert(err, jc.ErrorIsNil)

	// Hydrate a network.SpaceInfos from the response
	gotSpaceInfos := params.ToNetworkSpaceInfos(res)
	c.Assert(gotSpaceInfos, gc.DeepEquals, s.st.spaceInfos[0:1], gc.Commentf("expected to get back a filtered list of the space infos"))
}
