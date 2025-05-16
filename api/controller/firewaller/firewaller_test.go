// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package firewaller_test

import (
	stdtesting "testing"

	"github.com/juju/names/v6"
	"github.com/juju/tc"

	"github.com/juju/juju/api/base/testing"
	"github.com/juju/juju/api/controller/firewaller"
	"github.com/juju/juju/core/network"
	"github.com/juju/juju/core/relation"
	coretesting "github.com/juju/juju/internal/testing"
	"github.com/juju/juju/rpc/params"
)

// NOTE: This suite is intended for embedding into other suites,
// so common code can be reused. Do not add test cases to it,
// otherwise they'll be run by each other suite that embeds it.
type firewallerSuite struct {
	coretesting.BaseSuite
}

func TestFirewallerSuite(t *stdtesting.T) { tc.Run(t, &firewallerSuite{}) }
func (s *firewallerSuite) TestModelFirewallRules(c *tc.C) {
	var callCount int
	apiCaller := testing.APICallerFunc(func(objType string, version int, id, request string, arg, result interface{}) error {
		c.Check(objType, tc.Equals, "Firewaller")
		c.Check(version, tc.Equals, 0)
		c.Check(id, tc.Equals, "")
		c.Check(request, tc.Equals, "ModelFirewallRules")
		c.Assert(arg, tc.IsNil)
		c.Assert(result, tc.FitsTypeOf, &params.IngressRulesResult{})
		*(result.(*params.IngressRulesResult)) = params.IngressRulesResult{
			Error: &params.Error{Message: "FAIL"},
		}
		callCount++
		return nil
	})
	client, err := firewaller.NewClient(apiCaller)
	c.Assert(err, tc.ErrorIsNil)
	_, err = client.ModelFirewallRules(c.Context())
	c.Check(err, tc.ErrorMatches, "FAIL")
	c.Check(callCount, tc.Equals, 1)
}

func (s *firewallerSuite) TestWatchModelFirewallRules(c *tc.C) {
	var callCount int
	apiCaller := testing.APICallerFunc(func(objType string, version int, id, request string, arg, result interface{}) error {
		c.Check(objType, tc.Equals, "Firewaller")
		c.Check(version, tc.Equals, 0)
		c.Check(id, tc.Equals, "")
		c.Check(request, tc.Equals, "WatchModelFirewallRules")
		c.Assert(arg, tc.IsNil)
		c.Assert(result, tc.FitsTypeOf, &params.NotifyWatchResult{})
		*(result.(*params.NotifyWatchResult)) = params.NotifyWatchResult{
			Error: &params.Error{Message: "FAIL"},
		}
		callCount++
		return nil
	})
	client, err := firewaller.NewClient(apiCaller)
	c.Assert(err, tc.ErrorIsNil)
	_, err = client.WatchModelFirewallRules(c.Context())
	c.Check(err, tc.ErrorMatches, "FAIL")
	c.Check(callCount, tc.Equals, 1)
}

func (s *firewallerSuite) TestWatchModelMachines(c *tc.C) {
	var callCount int
	apiCaller := testing.APICallerFunc(func(objType string, version int, id, request string, arg, result interface{}) error {
		c.Check(objType, tc.Equals, "Firewaller")
		c.Check(version, tc.Equals, 0)
		c.Check(id, tc.Equals, "")
		c.Check(request, tc.Equals, "WatchModelMachines")
		c.Assert(arg, tc.IsNil)
		c.Assert(result, tc.FitsTypeOf, &params.StringsWatchResult{})
		*(result.(*params.StringsWatchResult)) = params.StringsWatchResult{
			Error: &params.Error{Message: "FAIL"},
		}
		callCount++
		return nil
	})
	client, err := firewaller.NewClient(apiCaller)
	c.Assert(err, tc.ErrorIsNil)
	_, err = client.WatchModelMachines(c.Context())
	c.Check(err, tc.ErrorMatches, "FAIL")
	c.Check(callCount, tc.Equals, 1)
}

func (s *firewallerSuite) TestWatchEgressAddressesForRelation(c *tc.C) {
	var callCount int
	relationTag := names.NewRelationTag("mediawiki:db mysql:db")
	apiCaller := testing.APICallerFunc(func(objType string, version int, id, request string, arg, result interface{}) error {
		c.Check(objType, tc.Equals, "Firewaller")
		c.Check(version, tc.Equals, 0)
		c.Check(id, tc.Equals, "")
		c.Check(request, tc.Equals, "WatchEgressAddressesForRelations")
		c.Assert(arg, tc.DeepEquals, params.Entities{Entities: []params.Entity{{Tag: relationTag.String()}}})
		c.Assert(result, tc.FitsTypeOf, &params.StringsWatchResults{})
		*(result.(*params.StringsWatchResults)) = params.StringsWatchResults{
			Results: []params.StringsWatchResult{{
				Error: &params.Error{Message: "FAIL"},
			}},
		}
		callCount++
		return nil
	})
	client, err := firewaller.NewClient(apiCaller)
	c.Assert(err, tc.ErrorIsNil)
	_, err = client.WatchEgressAddressesForRelation(c.Context(), relationTag)
	c.Check(err, tc.ErrorMatches, "FAIL")
	c.Check(callCount, tc.Equals, 1)
}

func (s *firewallerSuite) TestWatchIngressAddressesForRelation(c *tc.C) {
	c.Skip("Re-enable this test whenever CMR will be fully implemented and the related watcher rewired.")
	var callCount int
	relationTag := names.NewRelationTag("mediawiki:db mysql:db")
	apiCaller := testing.APICallerFunc(func(objType string, version int, id, request string, arg, result interface{}) error {
		c.Check(objType, tc.Equals, "Firewaller")
		c.Check(version, tc.Equals, 0)
		c.Check(id, tc.Equals, "")
		c.Check(request, tc.Equals, "WatchIngressAddressesForRelations")
		c.Assert(arg, tc.DeepEquals, params.Entities{Entities: []params.Entity{{Tag: relationTag.String()}}})
		c.Assert(result, tc.FitsTypeOf, &params.StringsWatchResults{})
		*(result.(*params.StringsWatchResults)) = params.StringsWatchResults{
			Results: []params.StringsWatchResult{{
				Error: &params.Error{Message: "FAIL"},
			}},
		}
		callCount++
		return nil
	})
	client, err := firewaller.NewClient(apiCaller)
	c.Assert(err, tc.ErrorIsNil)
	_, err = client.WatchIngressAddressesForRelation(c.Context(), relationTag)
	c.Check(err, tc.ErrorMatches, "FAIL")
	c.Check(callCount, tc.Equals, 1)
}

func (s *firewallerSuite) TestControllerAPIInfoForModel(c *tc.C) {
	var callCount int
	apiCaller := testing.APICallerFunc(func(objType string, version int, id, request string, arg, result interface{}) error {
		c.Check(objType, tc.Equals, "Firewaller")
		c.Check(version, tc.Equals, 0)
		c.Check(id, tc.Equals, "")
		c.Check(request, tc.Equals, "ControllerAPIInfoForModels")
		c.Assert(arg, tc.DeepEquals, params.Entities{Entities: []params.Entity{{Tag: coretesting.ModelTag.String()}}})
		c.Assert(result, tc.FitsTypeOf, &params.ControllerAPIInfoResults{})
		*(result.(*params.ControllerAPIInfoResults)) = params.ControllerAPIInfoResults{
			Results: []params.ControllerAPIInfoResult{{
				Error: &params.Error{Message: "FAIL"},
			}},
		}
		callCount++
		return nil
	})
	client, err := firewaller.NewClient(apiCaller)
	c.Assert(err, tc.ErrorIsNil)
	_, err = client.ControllerAPIInfoForModel(c.Context(), coretesting.ModelTag.Id())
	c.Check(err, tc.ErrorMatches, "FAIL")
	c.Check(callCount, tc.Equals, 1)
}

func (s *firewallerSuite) TestMacaroonForRelation(c *tc.C) {
	var callCount int
	apiCaller := testing.APICallerFunc(func(objType string, version int, id, request string, arg, result interface{}) error {
		c.Check(objType, tc.Equals, "Firewaller")
		c.Check(version, tc.Equals, 0)
		c.Check(id, tc.Equals, "")
		c.Check(request, tc.Equals, "MacaroonForRelations")
		c.Assert(arg, tc.DeepEquals, params.Entities{Entities: []params.Entity{
			{Tag: names.NewRelationTag("mysql:db wordpress:db").String()}}})
		c.Assert(result, tc.FitsTypeOf, &params.MacaroonResults{})
		*(result.(*params.MacaroonResults)) = params.MacaroonResults{
			Results: []params.MacaroonResult{{
				Error: &params.Error{Message: "FAIL"},
			}},
		}
		callCount++
		return nil
	})
	client, err := firewaller.NewClient(apiCaller)
	c.Assert(err, tc.ErrorIsNil)
	_, err = client.MacaroonForRelation(c.Context(), "mysql:db wordpress:db")
	c.Check(err, tc.ErrorMatches, "FAIL")
	c.Check(callCount, tc.Equals, 1)
}

func (s *firewallerSuite) TestSetRelationStatus(c *tc.C) {
	var callCount int
	apiCaller := testing.APICallerFunc(func(objType string, version int, id, request string, arg, result interface{}) error {
		c.Check(objType, tc.Equals, "Firewaller")
		c.Check(version, tc.Equals, 0)
		c.Check(id, tc.Equals, "")
		c.Check(request, tc.Equals, "SetRelationsStatus")
		c.Assert(arg, tc.DeepEquals, params.SetStatus{Entities: []params.EntityStatusArgs{
			{
				Tag:    names.NewRelationTag("mysql:db wordpress:db").String(),
				Status: "suspended",
				Info:   "a message",
			}}})
		c.Assert(result, tc.FitsTypeOf, &params.ErrorResults{})
		*(result.(*params.ErrorResults)) = params.ErrorResults{
			Results: []params.ErrorResult{{
				Error: &params.Error{Message: "FAIL"},
			}},
		}
		callCount++
		return nil
	})
	client, err := firewaller.NewClient(apiCaller)
	c.Assert(err, tc.ErrorIsNil)
	err = client.SetRelationStatus(c.Context(), "mysql:db wordpress:db", relation.Suspended, "a message")
	c.Check(err, tc.ErrorMatches, "FAIL")
	c.Check(callCount, tc.Equals, 1)
}

func (s *firewallerSuite) TestAllSpaceInfos(c *tc.C) {
	expSpaceInfos := network.SpaceInfos{
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
				},
			},
		},
	}

	var callCount int
	apiCaller := testing.BestVersionCaller{
		APICallerFunc: testing.APICallerFunc(func(objType string, version int, id, request string, arg, result interface{}) error {
			c.Check(objType, tc.Equals, "Firewaller")
			c.Check(version, tc.Equals, 6)
			c.Check(id, tc.Equals, "")
			c.Check(request, tc.Equals, "SpaceInfos")
			c.Assert(arg, tc.DeepEquals, params.SpaceInfosParams{})
			c.Assert(result, tc.FitsTypeOf, &params.SpaceInfos{})
			*(result.(*params.SpaceInfos)) = params.FromNetworkSpaceInfos(expSpaceInfos)
			callCount++
			return nil
		}),
		BestVersion: 6,
	}

	client, err := firewaller.NewClient(apiCaller)
	c.Assert(err, tc.ErrorIsNil)
	got, err := client.AllSpaceInfos(c.Context())
	c.Assert(err, tc.ErrorIsNil)
	c.Check(callCount, tc.Equals, 1)
	c.Assert(got, tc.DeepEquals, expSpaceInfos)
}
