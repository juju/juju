// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package firewall_test

import (
	"context"
	stdtesting "testing"

	"github.com/juju/errors"
	"github.com/juju/tc"

	"github.com/juju/juju/cmd/juju/firewall"
	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/internal/cmd"
	"github.com/juju/juju/internal/cmd/cmdtesting"
	"github.com/juju/juju/internal/testing"
)

type SetRuleSuite struct {
	testing.BaseSuite

	mockAPI *mockSetRuleAPI
}

func TestSetRuleSuite(t *stdtesting.T) { tc.Run(t, &SetRuleSuite{}) }
func (s *SetRuleSuite) SetUpTest(c *tc.C) {
	s.mockAPI = &mockSetRuleAPI{}
}

func (s *SetRuleSuite) TestInitMissingService(c *tc.C) {
	_, err := s.runSetRule(c, "--allowlist", "10.0.0.0/8")
	c.Assert(err, tc.ErrorMatches, "no well known service specified")
}

func (s *SetRuleSuite) TestInitMissingWhitelist(c *tc.C) {
	_, err := s.runSetRule(c, "ssh")
	c.Assert(err, tc.ErrorMatches, `no allowlist subnets specified`)
}

func (s *SetRuleSuite) TestSetRuleSSH(c *tc.C) {
	_, err := s.runSetRule(c, "--allowlist", "10.2.1.0/8,192.168.1.0/8", "ssh")
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(s.mockAPI.sshRule, tc.Equals, "10.2.1.0/8,192.168.1.0/8")
}

func (s *SetRuleSuite) TestSetRuleSAAS(c *tc.C) {
	_, err := s.runSetRule(c, "--allowlist", "10.2.1.0/8,192.168.1.0/8", "juju-application-offer")
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(s.mockAPI.saasRule, tc.Equals, "10.2.1.0/8,192.168.1.0/8")
}

func (s *SetRuleSuite) TestWhitelistAndAllowlist(c *tc.C) {
	_, err := s.runSetRule(c, "ssh", "--allowlist", "192.168.0.0/24", "--whitelist", "192.168.1.0/24")
	c.Assert(err, tc.ErrorMatches, "cannot specify both whitelist and allowlist")
}

func (s *SetRuleSuite) TestSetError(c *tc.C) {
	s.mockAPI.err = errors.New("fail")
	_, err := s.runSetRule(c, "ssh", "--allowlist", "10.0.0.0/8")
	c.Assert(err, tc.ErrorMatches, ".*fail.*")
}

func (s *SetRuleSuite) runSetRule(c *tc.C, args ...string) (*cmd.Context, error) {
	return cmdtesting.RunCommand(c, firewall.NewSetRulesCommandForTest(s.mockAPI), args...)
}

type mockSetRuleAPI struct {
	sshRule  string
	saasRule string
	err      error
}

func (s *mockSetRuleAPI) Close() error {
	return nil
}

func (s *mockSetRuleAPI) ModelSet(ctx context.Context, cfg map[string]interface{}) error {
	if s.err != nil {
		return s.err
	}
	sshRule, ok := cfg[config.SSHAllowKey].(string)
	if ok {
		s.sshRule = sshRule
	}
	saasRule, ok := cfg[config.SAASIngressAllowKey].(string)
	if ok {
		s.saasRule = saasRule
	}

	return nil
}
