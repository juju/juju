// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package firewaller_test

import (
	"github.com/juju/names/v4"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/utils"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/api"
	"github.com/juju/juju/api/base/testing"
	"github.com/juju/juju/api/firewaller"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/core/relation"
	jujutesting "github.com/juju/juju/juju/testing"
	"github.com/juju/juju/state"
	coretesting "github.com/juju/juju/testing"
)

// NOTE: This suite is intended for embedding into other suites,
// so common code can be reused. Do not add test cases to it,
// otherwise they'll be run by each other suite that embeds it.
type firewallerSuite struct {
	jujutesting.JujuConnSuite

	st          api.Connection
	machines    []*state.Machine
	application *state.Application
	charm       *state.Charm
	units       []*state.Unit
	relations   []*state.Relation

	firewaller *firewaller.Client
}

var _ = gc.Suite(&firewallerSuite{})

func (s *firewallerSuite) SetUpTest(c *gc.C) {
	s.JujuConnSuite.SetUpTest(c)

	// Reset previous machines and units (if any) and create 3
	// machines for the tests. The first one is a manager node.
	s.machines = make([]*state.Machine, 3)
	s.units = make([]*state.Unit, 3)

	var err error
	s.machines[0], err = s.State.AddMachine("quantal", state.JobManageModel, state.JobHostUnits)
	c.Assert(err, jc.ErrorIsNil)
	password, err := utils.RandomPassword()
	c.Assert(err, jc.ErrorIsNil)
	err = s.machines[0].SetPassword(password)
	c.Assert(err, jc.ErrorIsNil)
	err = s.machines[0].SetProvisioned("i-manager", "", "fake_nonce", nil)
	c.Assert(err, jc.ErrorIsNil)
	s.st = s.OpenAPIAsMachine(c, s.machines[0].Tag(), password, "fake_nonce")
	c.Assert(s.st, gc.NotNil)

	// Note that the specific machine ids allocated are assumed
	// to be numerically consecutive from zero.
	for i := 1; i <= 2; i++ {
		s.machines[i], err = s.State.AddMachine("quantal", state.JobHostUnits)
		c.Check(err, jc.ErrorIsNil)
	}
	// Create an application and three units for these machines.
	s.charm = s.AddTestingCharm(c, "wordpress")
	s.application = s.AddTestingApplication(c, "wordpress", s.charm)
	// Add the rest of the units and assign them.
	for i := 0; i <= 2; i++ {
		s.units[i], err = s.application.AddUnit(state.AddUnitParams{})
		c.Check(err, jc.ErrorIsNil)
		err = s.units[i].AssignToMachine(s.machines[i])
		c.Check(err, jc.ErrorIsNil)
	}

	// Create a relation.
	s.AddTestingApplication(c, "mysql", s.AddTestingCharm(c, "mysql"))
	eps, err := s.State.InferEndpoints("wordpress", "mysql")
	c.Assert(err, jc.ErrorIsNil)

	s.relations = make([]*state.Relation, 1)
	s.relations[0], err = s.State.AddRelation(eps...)
	c.Assert(err, jc.ErrorIsNil)

	// Create the firewaller API facade.
	firewallerClient, err := firewaller.NewClient(s.st)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(firewallerClient, gc.NotNil)
	s.firewaller = firewallerClient
	// Before we get into the tests, ensure that all the creation events have flowed through the system.
	s.WaitForModelWatchersIdle(c, s.Model.UUID())
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
	_, err = client.WatchEgressAddressesForRelation(relationTag)
	c.Check(err, gc.ErrorMatches, "FAIL")
	c.Check(callCount, gc.Equals, 1)
}

func (s *firewallerSuite) TestWatchInressAddressesForRelation(c *gc.C) {
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
	_, err = client.WatchIngressAddressesForRelation(relationTag)
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
	_, err = client.ControllerAPIInfoForModel(coretesting.ModelTag.Id())
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
	_, err = client.MacaroonForRelation("mysql:db wordpress:db")
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
	err = client.SetRelationStatus("mysql:db wordpress:db", relation.Suspended, "a message")
	c.Check(err, gc.ErrorMatches, "FAIL")
	c.Check(callCount, gc.Equals, 1)
}

func (s *firewallerSuite) TestFirewallRules(c *gc.C) {
	var callCount int
	apiCaller := testing.APICallerFunc(func(objType string, version int, id, request string, arg, result interface{}) error {
		c.Check(objType, gc.Equals, "Firewaller")
		c.Check(version, gc.Equals, 0)
		c.Check(id, gc.Equals, "")
		c.Check(request, gc.Equals, "FirewallRules")
		c.Assert(arg, gc.DeepEquals, params.KnownServiceArgs{
			KnownServices: []params.KnownServiceValue{
				params.JujuApplicationOfferRule, params.JujuControllerRule,
			},
		})
		c.Assert(result, gc.FitsTypeOf, &params.ListFirewallRulesResults{})
		*(result.(*params.ListFirewallRulesResults)) = params.ListFirewallRulesResults{
			Rules: []params.FirewallRule{
				{KnownService: params.JujuApplicationOfferRule, WhitelistCIDRS: []string{"10.0.0.0/16"}},
			},
		}
		callCount++
		return nil
	})
	client, err := firewaller.NewClient(apiCaller)
	c.Assert(err, jc.ErrorIsNil)
	result, err := client.FirewallRules("juju-application-offer", "juju-controller")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result, gc.HasLen, 1)
	c.Check(callCount, gc.Equals, 1)
}
