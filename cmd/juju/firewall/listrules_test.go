// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package firewall_test

import (
	"context"
	"strings"

	"github.com/juju/errors"
	"github.com/juju/tc"

	"github.com/juju/juju/cmd/juju/firewall"
	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/internal/cmd"
	"github.com/juju/juju/internal/cmd/cmdtesting"
	"github.com/juju/juju/internal/testing"
)

type ListSuite struct {
	testing.BaseSuite

	mockAPI *mockListAPI
}

var _ = tc.Suite(&ListSuite{})

func (s *ListSuite) SetUpTest(c *tc.C) {
	s.mockAPI = &mockListAPI{
		rules: "192.168.1.0/16,10.0.0.0/8",
	}
}

func (s *ListSuite) TestListError(c *tc.C) {
	s.mockAPI.err = errors.New("fail")
	_, err := s.runList(c, nil)
	c.Assert(err, tc.ErrorMatches, ".*fail.*")
}

func (s *ListSuite) TestListTabular(c *tc.C) {
	s.assertValidList(
		c,
		[]string{"--format", "tabular"},
		`
Service                 Allowlist subnets
juju-application-offer  0.0.0.0/0
ssh                     192.168.1.0/16,10.0.0.0/8
`[1:],
		"",
	)
}

func (s *ListSuite) TestListYAML(c *tc.C) {
	s.assertValidList(
		c,
		[]string{"--format", "yaml"},
		`
- known-service: ssh
  allowlist-subnets:
  - 192.168.1.0/16
  - 10.0.0.0/8
- known-service: juju-application-offer
  allowlist-subnets:
  - 0.0.0.0/0
`[1:],
		"",
	)
}

func (s *ListSuite) TestListEmpty(c *tc.C) {
	s.mockAPI.rules = ""
	s.assertValidList(
		c,
		[]string{"--format", "tabular"},
		`
Service                 Allowlist subnets
juju-application-offer  0.0.0.0/0
ssh                     
`[1:],
		"",
	)

}

func (s *ListSuite) runList(c *tc.C, args []string) (*cmd.Context, error) {
	return cmdtesting.RunCommand(c, firewall.NewListRulesCommandForTest(s.mockAPI), args...)
}

func (s *ListSuite) assertValidList(c *tc.C, args []string, expectedValid, expectedErr string) {
	context, err := s.runList(c, args)
	c.Assert(err, tc.ErrorIsNil)

	obtainedErr := strings.Replace(cmdtesting.Stderr(context), "\n", "", -1)
	c.Assert(obtainedErr, tc.Matches, expectedErr)

	obtainedValid := cmdtesting.Stdout(context)
	c.Assert(obtainedValid, tc.Matches, expectedValid)
}

type mockListAPI struct {
	rules string
	err   error
}

func (s *mockListAPI) Close() error {
	return nil
}

func (s *mockListAPI) ModelGet(ctx context.Context) (map[string]interface{}, error) {
	if s.err != nil {
		return nil, s.err
	}
	return testing.FakeConfig().Merge(testing.Attrs{
		config.SSHAllowKey:         s.rules,
		config.SAASIngressAllowKey: "0.0.0.0/0",
	}), nil
}
