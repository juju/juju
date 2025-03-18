// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package firewaller_test

import (
	"context"

	"github.com/juju/names/v6"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

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

var _ = gc.Suite(&firewallerSuite{})

func (s *firewallerSuite) TestModelFirewallRules(c *gc.C) {
	var callCount int
	apiCaller := testing.APICallerFunc(func(objType string, version int, id, request string, arg, result interface{}) error {
		c.Check(objType, gc.Equals, "Firewaller")
		c.Check(version, gc.Equals, 0)
		c.Check(id, gc.Equals, "")
		c.Check(request, gc.Equals, "ModelFirewallRules")
		c.Assert(arg, gc.IsNil)
		c.Assert(result, gc.FitsTypeOf, &params.IngressRulesResult{})
		*(result.(*params.IngressRulesResult)) = params.IngressRulesResult{
			Error: &params.Error{Message: "FAIL"},
		}
		callCount++
		return nil
	})
	client, err := firewaller.NewClient(apiCaller)
	c.Assert(err, jc.ErrorIsNil)
	_, err = client.ModelFirewallRules(context.Background())
	c.Check(err, gc.ErrorMatches, "FAIL")
	c.Check(callCount, gc.Equals, 1)
}

func (s *firewallerSuite) TestWatchModelFirewallRules(c *gc.C) {
	var callCount int
	apiCaller := testing.APICallerFunc(func(objType string, version int, id, request string, arg, result interface{}) error {
		c.Check(objType, gc.Equals, "Firewaller")
		c.Check(version, gc.Equals, 0)
		c.Check(id, gc.Equals, "")
		c.Check(request, gc.Equals, "WatchModelFirewallRules")
		c.Assert(arg, gc.IsNil)
		c.Assert(result, gc.FitsTypeOf, &params.NotifyWatchResult{})
		*(result.(*params.NotifyWatchResult)) = params.NotifyWatchResult{
			Error: &params.Error{Message: "FAIL"},
		}
		callCount++
		return nil
	})
	client, err := firewaller.NewClient(apiCaller)
	c.Assert(err, jc.ErrorIsNil)
	_, err = client.WatchModelFirewallRules(context.Background())
	c.Check(err, gc.ErrorMatches, "FAIL")
	c.Check(callCount, gc.Equals, 1)
}

func (s *firewallerSuite) TestWatchModelMachines(c *gc.C) {
	var callCount int
	apiCaller := testing.APICallerFunc(func(objType string, version int, id, request string, arg, result interface{}) error {
		c.Check(objType, gc.Equals, "Firewaller")
		c.Check(version, gc.Equals, 0)
		c.Check(id, gc.Equals, "")
		c.Check(request, gc.Equals, "WatchModelMachines")
		c.Assert(arg, gc.IsNil)
		c.Assert(result, gc.FitsTypeOf, &params.StringsWatchResult{})
		*(result.(*params.StringsWatchResult)) = params.StringsWatchResult{
			Error: &params.Error{Message: "FAIL"},
		}
		callCount++
		return nil
	})
	client, err := firewaller.NewClient(apiCaller)
	c.Assert(err, jc.ErrorIsNil)
	_, err = client.WatchModelMachines(context.Background())
	c.Check(err, gc.ErrorMatches, "FAIL")
	c.Check(callCount, gc.Equals, 1)
}

func (s *firewallerSuite) TestWatchEgressAddressesForRelation(c *gc.C) {
	var callCount int
	relationTag := names.NewRelationTag("mediawiki:db mysql:db")
	apiCaller := testing.APICallerFunc(func(objType string, version int, id, request string, arg, result interface{}) error {
		c.Check(objType, gc.Equals, "Firewaller")
		c.Check(version, gc.Equals, 0)
		c.Check(id, gc.Equals, "")
		c.Check(request, gc.Equals, "WatchEgressAddressesForRelations")
		c.Assert(arg, gc.DeepEquals, params.Entities{Entities: []params.Entity{{Tag: relationTag.String()}}})
		c.Assert(result, gc.FitsTypeOf, &params.StringsWatchResults{})
		*(result.(*params.StringsWatchResults)) = params.StringsWatchResults{
			Results: []params.StringsWatchResult{{
				Error: &params.Error{Message: "FAIL"},
			}},
		}
		callCount++
		return nil
	})
	client, err := firewaller.NewClient(apiCaller)
	c.Assert(err, jc.ErrorIsNil)
	_, err = client.WatchEgressAddressesForRelation(context.Background(), relationTag)
	c.Check(err, gc.ErrorMatches, "FAIL")
	c.Check(callCount, gc.Equals, 1)
}

func (s *firewallerSuite) TestWatchIngressAddressesForRelation(c *gc.C) {
	c.Skip("Re-enable this test whenever CMR will be fully implemented and the related watcher rewired.")
	var callCount int
	relationTag := names.NewRelationTag("mediawiki:db mysql:db")
	apiCaller := testing.APICallerFunc(func(objType string, version int, id, request string, arg, result interface{}) error {
		c.Check(objType, gc.Equals, "Firewaller")
		c.Check(version, gc.Equals, 0)
		c.Check(id, gc.Equals, "")
		c.Check(request, gc.Equals, "WatchIngressAddressesForRelations")
		c.Assert(arg, gc.DeepEquals, params.Entities{Entities: []params.Entity{{Tag: relationTag.String()}}})
		c.Assert(result, gc.FitsTypeOf, &params.StringsWatchResults{})
		*(result.(*params.StringsWatchResults)) = params.StringsWatchResults{
			Results: []params.StringsWatchResult{{
				Error: &params.Error{Message: "FAIL"},
			}},
		}
		callCount++
		return nil
	})
	client, err := firewaller.NewClient(apiCaller)
	c.Assert(err, jc.ErrorIsNil)
	_, err = client.WatchIngressAddressesForRelation(context.Background(), relationTag)
	c.Check(err, gc.ErrorMatches, "FAIL")
	c.Check(callCount, gc.Equals, 1)
}

func (s *firewallerSuite) TestControllerAPIInfoForModel(c *gc.C) {
	var callCount int
	apiCaller := testing.APICallerFunc(func(objType string, version int, id, request string, arg, result interface{}) error {
		c.Check(objType, gc.Equals, "Firewaller")
		c.Check(version, gc.Equals, 0)
		c.Check(id, gc.Equals, "")
		c.Check(request, gc.Equals, "ControllerAPIInfoForModels")
		c.Assert(arg, gc.DeepEquals, params.Entities{Entities: []params.Entity{{Tag: coretesting.ModelTag.String()}}})
		c.Assert(result, gc.FitsTypeOf, &params.ControllerAPIInfoResults{})
		*(result.(*params.ControllerAPIInfoResults)) = params.ControllerAPIInfoResults{
			Results: []params.ControllerAPIInfoResult{{
				Error: &params.Error{Message: "FAIL"},
			}},
		}
		callCount++
		return nil
	})
	client, err := firewaller.NewClient(apiCaller)
	c.Assert(err, jc.ErrorIsNil)
	_, err = client.ControllerAPIInfoForModel(context.Background(), coretesting.ModelTag.Id())
	c.Check(err, gc.ErrorMatches, "FAIL")
	c.Check(callCount, gc.Equals, 1)
}

func (s *firewallerSuite) TestMacaroonForRelation(c *gc.C) {
	var callCount int
	apiCaller := testing.APICallerFunc(func(objType string, version int, id, request string, arg, result interface{}) error {
		c.Check(objType, gc.Equals, "Firewaller")
		c.Check(version, gc.Equals, 0)
		c.Check(id, gc.Equals, "")
		c.Check(request, gc.Equals, "MacaroonForRelations")
		c.Assert(arg, gc.DeepEquals, params.Entities{Entities: []params.Entity{
			{Tag: names.NewRelationTag("mysql:db wordpress:db").String()}}})
		c.Assert(result, gc.FitsTypeOf, &params.MacaroonResults{})
		*(result.(*params.MacaroonResults)) = params.MacaroonResults{
			Results: []params.MacaroonResult{{
				Error: &params.Error{Message: "FAIL"},
			}},
		}
		callCount++
		return nil
	})
	client, err := firewaller.NewClient(apiCaller)
	c.Assert(err, jc.ErrorIsNil)
	_, err = client.MacaroonForRelation(context.Background(), "mysql:db wordpress:db")
	c.Check(err, gc.ErrorMatches, "FAIL")
	c.Check(callCount, gc.Equals, 1)
}

func (s *firewallerSuite) TestSetRelationStatus(c *gc.C) {
	var callCount int
	apiCaller := testing.APICallerFunc(func(objType string, version int, id, request string, arg, result interface{}) error {
		c.Check(objType, gc.Equals, "Firewaller")
		c.Check(version, gc.Equals, 0)
		c.Check(id, gc.Equals, "")
		c.Check(request, gc.Equals, "SetRelationsStatus")
		c.Assert(arg, gc.DeepEquals, params.SetStatus{Entities: []params.EntityStatusArgs{
			{
				Tag:    names.NewRelationTag("mysql:db wordpress:db").String(),
				Status: "suspended",
				Info:   "a message",
			}}})
		c.Assert(result, gc.FitsTypeOf, &params.ErrorResults{})
		*(result.(*params.ErrorResults)) = params.ErrorResults{
			Results: []params.ErrorResult{{
				Error: &params.Error{Message: "FAIL"},
			}},
		}
		callCount++
		return nil
	})
	client, err := firewaller.NewClient(apiCaller)
	c.Assert(err, jc.ErrorIsNil)
	err = client.SetRelationStatus(context.Background(), "mysql:db wordpress:db", relation.Suspended, "a message")
	c.Check(err, gc.ErrorMatches, "FAIL")
	c.Check(callCount, gc.Equals, 1)
}

func (s *firewallerSuite) TestAllSpaceInfos(c *gc.C) {
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
			c.Check(objType, gc.Equals, "Firewaller")
			c.Check(version, gc.Equals, 6)
			c.Check(id, gc.Equals, "")
			c.Check(request, gc.Equals, "SpaceInfos")
			c.Assert(arg, gc.DeepEquals, params.SpaceInfosParams{})
			c.Assert(result, gc.FitsTypeOf, &params.SpaceInfos{})
			*(result.(*params.SpaceInfos)) = params.FromNetworkSpaceInfos(expSpaceInfos)
			callCount++
			return nil
		}),
		BestVersion: 6,
	}

	client, err := firewaller.NewClient(apiCaller)
	c.Assert(err, jc.ErrorIsNil)
	got, err := client.AllSpaceInfos(context.Background())
	c.Assert(err, jc.ErrorIsNil)
	c.Check(callCount, gc.Equals, 1)
	c.Assert(got, gc.DeepEquals, expSpaceInfos)
}
