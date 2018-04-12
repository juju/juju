// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package firewall_test

import (
	"strings"

	"github.com/juju/cmd"
	"github.com/juju/cmd/cmdtesting"
	"github.com/juju/errors"
	"github.com/juju/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/cmd/juju/firewall"
)

type ListSuite struct {
	testing.BaseSuite

	mockAPI *mockListAPI
}

var _ = gc.Suite(&ListSuite{})

func (s *ListSuite) SetUpTest(c *gc.C) {
	s.mockAPI = &mockListAPI{
		rules: []params.FirewallRule{
			{
				KnownService:   "ssh",
				WhitelistCIDRS: []string{"192.168.1.0/16", "10.0.0.0/8"},
			}, {
				KnownService:   "juju-controller",
				WhitelistCIDRS: []string{"10.2.0.0/16"},
			},
		},
	}
}

func (s *ListSuite) TestListError(c *gc.C) {
	s.mockAPI.err = errors.New("fail")
	_, err := s.runList(c, nil)
	c.Assert(err, gc.ErrorMatches, ".*fail.*")
}

func (s *ListSuite) TestListTabular(c *gc.C) {
	s.assertValidList(
		c,
		[]string{"--format", "tabular"},
		`
Service          Whitelist subnets
juju-controller  10.2.0.0/16
ssh              192.168.1.0/16,10.0.0.0/8

`[1:],
		"",
	)
}

func (s *ListSuite) TestListYAML(c *gc.C) {
	s.assertValidList(
		c,
		[]string{"--format", "yaml"},
		`
- known-service: ssh
  whitelist-subnets:
  - 192.168.1.0/16
  - 10.0.0.0/8
- known-service: juju-controller
  whitelist-subnets:
  - 10.2.0.0/16
`[1:],
		"",
	)
}

func (s *ListSuite) runList(c *gc.C, args []string) (*cmd.Context, error) {
	return cmdtesting.RunCommand(c, firewall.NewListRulesCommandForTest(s.mockAPI), args...)
}

func (s *ListSuite) assertValidList(c *gc.C, args []string, expectedValid, expectedErr string) {
	context, err := s.runList(c, args)
	c.Assert(err, jc.ErrorIsNil)

	obtainedErr := strings.Replace(cmdtesting.Stderr(context), "\n", "", -1)
	c.Assert(obtainedErr, gc.Matches, expectedErr)

	obtainedValid := cmdtesting.Stdout(context)
	c.Assert(obtainedValid, gc.Matches, expectedValid)
}

type mockListAPI struct {
	rules []params.FirewallRule
	err   error
}

func (s *mockListAPI) Close() error {
	return nil
}

func (s *mockListAPI) ListFirewallRules() ([]params.FirewallRule, error) {
	if s.err != nil {
		return nil, s.err
	}
	return s.rules, nil
}
