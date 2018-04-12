// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package firewall_test

import (
	"github.com/juju/cmd"
	"github.com/juju/cmd/cmdtesting"
	"github.com/juju/errors"
	"github.com/juju/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/cmd/juju/firewall"
)

type SetRuleSuite struct {
	testing.BaseSuite

	mockAPI *mockSetRuleAPI
}

var _ = gc.Suite(&SetRuleSuite{})

func (s *SetRuleSuite) SetUpTest(c *gc.C) {
	s.mockAPI = &mockSetRuleAPI{}
}

func (s *SetRuleSuite) TestInitMissingService(c *gc.C) {
	_, err := s.runSetRule(c, "--whitelist", "10.0.0.0/8")
	c.Assert(err, gc.ErrorMatches, "no well known service specified")
}

func (s *SetRuleSuite) TestInitInvalidWhitelist(c *gc.C) {
	_, err := s.runSetRule(c, "--whitelist", "foo", "ssh")
	c.Assert(err, gc.ErrorMatches, `invalid white-list subnet: invalid CIDR address: foo`)
}

func (s *SetRuleSuite) TestInitMissingWhitelist(c *gc.C) {
	_, err := s.runSetRule(c, "ssh")
	c.Assert(err, gc.ErrorMatches, `no whitelist subnets specified`)
}

func (s *SetRuleSuite) TestSetRule(c *gc.C) {
	_, err := s.runSetRule(c, "--whitelist", "10.2.1.0/8,192.168.1.0/8", "ssh")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(s.mockAPI.rule, jc.DeepEquals, params.FirewallRule{
		KnownService:   params.SSHRule,
		WhitelistCIDRS: []string{"10.2.1.0/8", "192.168.1.0/8"},
	})
}

func (s *SetRuleSuite) TestSetError(c *gc.C) {
	s.mockAPI.err = errors.New("fail")
	_, err := s.runSetRule(c, "ssh", "--whitelist", "10.0.0.0/8")
	c.Assert(err, gc.ErrorMatches, ".*fail.*")
}

func (s *SetRuleSuite) runSetRule(c *gc.C, args ...string) (*cmd.Context, error) {
	return cmdtesting.RunCommand(c, firewall.NewSetRulesCommandForTest(s.mockAPI), args...)
}

type mockSetRuleAPI struct {
	rule params.FirewallRule
	err  error
}

func (s *mockSetRuleAPI) Close() error {
	return nil
}

func (s *mockSetRuleAPI) SetFirewallRule(service string, whiteListCidrs []string) error {
	if s.err != nil {
		return s.err
	}
	s.rule = params.FirewallRule{
		KnownService:   params.KnownServiceValue(service),
		WhitelistCIDRS: whiteListCidrs,
	}
	return nil
}
