// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state_test

import (
	"regexp"

	"github.com/juju/errors"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
	"gopkg.in/mgo.v2/bson"

	"github.com/juju/juju/state"
)

type FirewallRulesSuite struct {
	ConnSuite
}

var _ = gc.Suite(&FirewallRulesSuite{})

func (s *FirewallRulesSuite) TestSaveInvalidWhitelistCIDR(c *gc.C) {
	rules := state.NewFirewallRules(s.State)
	err := rules.Save(state.FirewallRule{
		WellKnownService: state.JujuControllerRule,
		WhitelistCIDRs:   []string{"192.168.1"},
	})
	c.Assert(err, gc.ErrorMatches, regexp.QuoteMeta(`CIDR "192.168.1" not valid`))
}

func (s *FirewallRulesSuite) TestSaveInvalidBlacklistCIDR(c *gc.C) {
	rules := state.NewFirewallRules(s.State)
	err := rules.Save(state.FirewallRule{
		WellKnownService: state.JujuControllerRule,
		WhitelistCIDRs:   []string{"10.0.0.0/8"},
		BlacklistCIDRs:   []string{"192.168.1"},
	})
	c.Assert(err, gc.ErrorMatches, regexp.QuoteMeta(`CIDR "192.168.1" not valid`))
}

func (s *FirewallRulesSuite) assertSavedRules(c *gc.C, service state.WellKnownServiceType, expectedWhitelist, expectedBlacklist []string) {
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
	cidrs = nil
	for _, m := range raw["blacklist-cidrs"].([]interface{}) {
		cidrs = append(cidrs, m.(string))
	}
	c.Assert(cidrs, jc.SameContents, expectedBlacklist)
}

func (s *FirewallRulesSuite) TestSaveInvalid(c *gc.C) {
	rules := state.NewFirewallRules(s.State)
	err := rules.Save(state.FirewallRule{
		WellKnownService: "foo",
		WhitelistCIDRs:   []string{"192.168.1.0/16"},
		BlacklistCIDRs:   []string{"10.0.0.0/8"},
	})
	c.Assert(err, jc.Satisfies, errors.IsNotValid)
	c.Assert(err, gc.ErrorMatches, `well known service type "foo" not valid`)
}

func (s *FirewallRulesSuite) TestSave(c *gc.C) {
	rules := state.NewFirewallRules(s.State)
	err := rules.Save(state.FirewallRule{
		WellKnownService: state.SSHRule,
		WhitelistCIDRs:   []string{"192.168.1.0/16"},
		BlacklistCIDRs:   []string{"10.0.0.0/8"},
	})
	c.Assert(err, jc.ErrorIsNil)
	s.assertSavedRules(c, state.SSHRule, []string{"192.168.1.0/16"}, []string{"10.0.0.0/8"})
}

func (s *FirewallRulesSuite) TestSaveIdempotent(c *gc.C) {
	rules := state.NewFirewallRules(s.State)
	rule := state.FirewallRule{
		WellKnownService: state.SSHRule,
		WhitelistCIDRs:   []string{"192.168.1.0/16"},
		BlacklistCIDRs:   []string{"10.0.0.0/8"},
	}
	err := rules.Save(rule)
	c.Assert(err, jc.ErrorIsNil)
	err = rules.Save(rule)
	c.Assert(err, jc.ErrorIsNil)
	s.assertSavedRules(c, state.SSHRule, []string{"192.168.1.0/16"}, []string{"10.0.0.0/8"})
}

func (s *FirewallRulesSuite) TestRule(c *gc.C) {
	rules := state.NewFirewallRules(s.State)
	err := rules.Save(state.FirewallRule{
		WellKnownService: state.JujuApplicationOfferRule,
		WhitelistCIDRs:   []string{"192.168.1.0/16"},
		BlacklistCIDRs:   []string{"10.0.0.0/8"},
	})
	c.Assert(err, jc.ErrorIsNil)
	result, err := rules.Rule(state.JujuApplicationOfferRule)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result.WhitelistCIDRs, jc.DeepEquals, []string{"192.168.1.0/16"})
	c.Assert(result.BlacklistCIDRs, jc.DeepEquals, []string{"10.0.0.0/8"})
	_, err = rules.Rule(state.JujuControllerRule)
	c.Assert(err, jc.Satisfies, errors.IsNotFound)
}

func (s *FirewallRulesSuite) TestAllRules(c *gc.C) {
	rules := state.NewFirewallRules(s.State)
	err := rules.Save(state.FirewallRule{
		WellKnownService: state.JujuApplicationOfferRule,
		WhitelistCIDRs:   []string{"192.168.1.0/16"},
		BlacklistCIDRs:   []string{"10.0.0.0/8"},
	})
	c.Assert(err, jc.ErrorIsNil)
	err = rules.Save(state.FirewallRule{
		WellKnownService: state.JujuControllerRule,
		WhitelistCIDRs:   []string{"192.168.2.0/16"},
	})
	c.Assert(err, jc.ErrorIsNil)
	result, err := rules.AllRules()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result, gc.HasLen, 2)
	appRuleIndex := 0
	ctrlRuleIndex := 1
	if result[0].WellKnownService == state.JujuControllerRule {
		appRuleIndex = 1
		ctrlRuleIndex = 0
	}
	c.Assert(result[appRuleIndex].WellKnownService, gc.Equals, state.JujuApplicationOfferRule)
	c.Assert(result[appRuleIndex].WhitelistCIDRs, jc.DeepEquals, []string{"192.168.1.0/16"})
	c.Assert(result[appRuleIndex].BlacklistCIDRs, jc.DeepEquals, []string{"10.0.0.0/8"})
	c.Assert(result[ctrlRuleIndex].WellKnownService, gc.Equals, state.JujuControllerRule)
	c.Assert(result[ctrlRuleIndex].WhitelistCIDRs, jc.DeepEquals, []string{"192.168.2.0/16"})
	c.Assert(result[ctrlRuleIndex].BlacklistCIDRs, jc.DeepEquals, []string{})
}

func (s *FirewallRulesSuite) TestUpdate(c *gc.C) {
	rules := state.NewFirewallRules(s.State)
	err := rules.Save(state.FirewallRule{
		WellKnownService: state.JujuApplicationOfferRule,
		WhitelistCIDRs:   []string{"192.168.1.0/16"},
		BlacklistCIDRs:   []string{"10.0.0.0/8"},
	})
	c.Assert(err, jc.ErrorIsNil)
	err = rules.Save(state.FirewallRule{
		WellKnownService: state.JujuApplicationOfferRule,
		WhitelistCIDRs:   []string{"192.168.2.0/16"},
	})
	c.Assert(err, jc.ErrorIsNil)
	s.assertSavedRules(c, state.JujuApplicationOfferRule, []string{"192.168.2.0/16"}, nil)
}
