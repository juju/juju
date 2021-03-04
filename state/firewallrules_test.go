// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state_test

import (
	"regexp"

	"github.com/juju/errors"
	"github.com/juju/mgo/v2/bson"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/core/network/firewall"
	"github.com/juju/juju/state"
)

type FirewallRulesSuite struct {
	ConnSuite
}

var _ = gc.Suite(&FirewallRulesSuite{})

func (s *FirewallRulesSuite) TestSaveInvalidWhitelistCIDR(c *gc.C) {
	rules := state.NewFirewallRules(s.State)
	err := rules.Save(state.NewFirewallRule(firewall.JujuControllerRule, []string{"192.168.1"}))
	c.Assert(errors.Cause(err), gc.ErrorMatches, regexp.QuoteMeta(`CIDR "192.168.1" not valid`))
}

func (s *FirewallRulesSuite) assertSavedRules(c *gc.C, service firewall.WellKnownServiceType, expectedWhitelist []string) {
	coll, closer := state.GetCollection(s.State, "firewallRules")
	defer closer()

	var raw bson.M
	err := coll.FindId(string(service)).One(&raw)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(raw["known-service"], gc.Equals, string(service))
	var cidrs []string
	for _, m := range raw["whitelist-cidrs"].([]interface{}) {
		cidrs = append(cidrs, m.(string))
	}
	c.Assert(cidrs, jc.SameContents, expectedWhitelist)
}

func (s *FirewallRulesSuite) TestSaveInvalid(c *gc.C) {
	rules := state.NewFirewallRules(s.State)
	err := rules.Save(state.NewFirewallRule("foo", []string{"192.168.1.0/16"}))
	c.Assert(err, jc.Satisfies, errors.IsNotValid)
	c.Assert(errors.Cause(err), gc.ErrorMatches, `well known service type "foo" not valid`)
}

func (s *FirewallRulesSuite) TestSave(c *gc.C) {
	rules := state.NewFirewallRules(s.State)
	err := rules.Save(state.NewFirewallRule(firewall.SSHRule, []string{"192.168.1.0/16"}))
	c.Assert(err, jc.ErrorIsNil)
	s.assertSavedRules(c, firewall.SSHRule, []string{"192.168.1.0/16"})
}

func (s *FirewallRulesSuite) TestSaveIdempotent(c *gc.C) {
	rules := state.NewFirewallRules(s.State)
	rule := state.NewFirewallRule(firewall.SSHRule, []string{"192.168.1.0/16"})
	err := rules.Save(rule)
	c.Assert(err, jc.ErrorIsNil)
	err = rules.Save(rule)
	c.Assert(err, jc.ErrorIsNil)
	s.assertSavedRules(c, firewall.SSHRule, []string{"192.168.1.0/16"})
}

func (s *FirewallRulesSuite) TestRule(c *gc.C) {
	rules := state.NewFirewallRules(s.State)
	err := rules.Save(state.NewFirewallRule(firewall.JujuApplicationOfferRule, []string{"192.168.1.0/16"}))
	c.Assert(err, jc.ErrorIsNil)
	result, err := rules.Rule(firewall.JujuApplicationOfferRule)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result.WhitelistCIDRs(), jc.DeepEquals, []string{"192.168.1.0/16"})
	_, err = rules.Rule(firewall.JujuControllerRule)
	c.Assert(err, jc.Satisfies, errors.IsNotFound)
}

func (s *FirewallRulesSuite) TestAllRules(c *gc.C) {
	rules := state.NewFirewallRules(s.State)
	err := rules.Save(state.NewFirewallRule(firewall.JujuApplicationOfferRule, []string{"192.168.1.0/16"}))
	c.Assert(err, jc.ErrorIsNil)
	err = rules.Save(state.NewFirewallRule(firewall.JujuControllerRule, []string{"192.168.2.0/16"}))
	c.Assert(err, jc.ErrorIsNil)
	result, err := rules.AllRules()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result, gc.HasLen, 2)
	appRuleIndex := 0
	ctrlRuleIndex := 1
	if result[0].WellKnownService() == firewall.JujuControllerRule {
		appRuleIndex = 1
		ctrlRuleIndex = 0
	}
	c.Assert(result[appRuleIndex].WellKnownService(), gc.Equals, firewall.JujuApplicationOfferRule)
	c.Assert(result[appRuleIndex].WhitelistCIDRs(), jc.DeepEquals, []string{"192.168.1.0/16"})
	c.Assert(result[ctrlRuleIndex].WellKnownService(), gc.Equals, firewall.JujuControllerRule)
	c.Assert(result[ctrlRuleIndex].WhitelistCIDRs(), jc.DeepEquals, []string{"192.168.2.0/16"})
}

func (s *FirewallRulesSuite) TestUpdate(c *gc.C) {
	rules := state.NewFirewallRules(s.State)
	err := rules.Save(state.NewFirewallRule(firewall.JujuApplicationOfferRule, []string{"192.168.1.0/16"}))
	c.Assert(err, jc.ErrorIsNil)
	err = rules.Save(state.NewFirewallRule(firewall.JujuApplicationOfferRule, []string{"192.168.2.0/16"}))
	c.Assert(err, jc.ErrorIsNil)
	s.assertSavedRules(c, firewall.JujuApplicationOfferRule, []string{"192.168.2.0/16"})
}

func (s *FirewallRulesSuite) TestRemove(c *gc.C) {
	rules := state.NewFirewallRules(s.State)
	err := rules.Save(state.NewFirewallRule(firewall.SSHRule, []string{"192.168.1.0/16"}))
	c.Assert(err, jc.ErrorIsNil)
	err = rules.Save(state.NewFirewallRule(firewall.JujuControllerRule, []string{"192.168.1.1/24"}))
	c.Assert(err, jc.ErrorIsNil)

	err = rules.Remove(firewall.SSHRule)
	c.Assert(err, jc.ErrorIsNil)

	result, err := rules.AllRules()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result, gc.HasLen, 1)

	c.Assert(result[0].WellKnownService(), gc.DeepEquals, firewall.JujuControllerRule)
}
