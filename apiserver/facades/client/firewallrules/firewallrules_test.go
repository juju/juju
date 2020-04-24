// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package firewallrules_test

import (
	"github.com/juju/errors"
	"github.com/juju/names/v4"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/apiserver/facades/client/firewallrules"
	"github.com/juju/juju/apiserver/params"
	apiservertesting "github.com/juju/juju/apiserver/testing"
	"github.com/juju/juju/core/firewall"
	"github.com/juju/juju/state"
	coretesting "github.com/juju/juju/testing"
)

type FirewallRulesSuite struct {
	testing.IsolationSuite
	coretesting.JujuOSEnvSuite
	backend mockBackend

	blockChecker mockBlockChecker
	authorizer   apiservertesting.FakeAuthorizer
	api          *firewallrules.API
}

var _ = gc.Suite(&FirewallRulesSuite{})

func (s *FirewallRulesSuite) setAPIUser(c *gc.C, user names.UserTag) {
	s.authorizer.Tag = user
	api, err := firewallrules.NewAPI(
		&s.backend,
		s.authorizer,
		&s.blockChecker,
	)
	c.Assert(err, jc.ErrorIsNil)
	s.api = api
}

func (s *FirewallRulesSuite) SetUpTest(c *gc.C) {
	s.IsolationSuite.SetUpTest(c)
	s.JujuOSEnvSuite.SetUpTest(c)
	s.authorizer = apiservertesting.FakeAuthorizer{
		Tag: names.NewUserTag("admin"),
	}
	s.backend = mockBackend{
		modelUUID: coretesting.ModelTag.Id(),
		rules:     make(map[string]state.FirewallRule),
	}
	s.blockChecker = mockBlockChecker{}
	api, err := firewallrules.NewAPI(
		&s.backend,
		s.authorizer,
		&s.blockChecker,
	)
	c.Assert(err, jc.ErrorIsNil)
	s.api = api
}

func (s *FirewallRulesSuite) TearDownTest(c *gc.C) {
	s.JujuOSEnvSuite.TearDownTest(c)
	s.IsolationSuite.TearDownTest(c)
}

func (s *FirewallRulesSuite) TestSetFirewallRules(c *gc.C) {
	result, err := s.api.SetFirewallRules(params.FirewallRuleArgs{
		Args: []params.FirewallRule{{
			KnownService:   "juju-controller",
			WhitelistCIDRS: []string{"1.2.3.4/8"},
		}},
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result, jc.DeepEquals, params.ErrorResults{[]params.ErrorResult{{Error: nil}}})
	c.Assert(s.backend.rules["juju-controller"], jc.DeepEquals, state.NewFirewallRule(firewall.JujuControllerRule, []string{"1.2.3.4/8"}))
}

func (s *FirewallRulesSuite) TestSetFirewallRulesPermission(c *gc.C) {
	s.setAPIUser(c, names.NewUserTag("mary"))
	_, err := s.api.SetFirewallRules(params.FirewallRuleArgs{
		Args: []params.FirewallRule{{
			KnownService:   "juju-controller",
			WhitelistCIDRS: []string{"1.2.3.4/8"},
		}},
	})
	c.Assert(err, gc.ErrorMatches, ".*permission denied.*")
	c.Assert(s.backend.rules, gc.HasLen, 0)
}

func (s *FirewallRulesSuite) TestSetFirewallRulesBlocked(c *gc.C) {
	s.blockChecker.SetErrors(errors.New("blocked"))
	_, err := s.api.SetFirewallRules(params.FirewallRuleArgs{
		Args: []params.FirewallRule{{
			KnownService:   "juju-controller",
			WhitelistCIDRS: []string{"1.2.3.4/8"},
		}},
	})
	c.Assert(err, gc.ErrorMatches, "blocked")
	s.blockChecker.CheckCallNames(c, "ChangeAllowed")
	c.Assert(s.backend.rules, gc.HasLen, 0)
}

func (s *FirewallRulesSuite) TestListFirewallRules(c *gc.C) {
	result, err := s.api.ListFirewallRules()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result, jc.DeepEquals, params.ListFirewallRulesResults{
		Rules: []params.FirewallRule{{
			KnownService:   params.JujuApplicationOfferRule,
			WhitelistCIDRS: []string{"1.2.3.4/8"},
		}}})
}

func (s *FirewallRulesSuite) TestListFirewallRulesPermission(c *gc.C) {
	s.setAPIUser(c, names.NewUserTag("mary"))
	_, err := s.api.ListFirewallRules()
	c.Assert(err, gc.ErrorMatches, ".*permission denied.*")
}
