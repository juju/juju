// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package firewall_test

import (
	"strings"

	"github.com/juju/cmd/v4"
	"github.com/juju/cmd/v4/cmdtesting"
	"github.com/juju/errors"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/cmd/juju/firewall"
	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/testing"
)

type ListSuite struct {
	testing.BaseSuite

	mockAPI *mockListAPI
}

var _ = gc.Suite(&ListSuite{})

func (s *ListSuite) SetUpTest(c *gc.C) {
	s.mockAPI = &mockListAPI{
		rules: "192.168.1.0/16,10.0.0.0/8",
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
Service                 Allowlist subnets
juju-application-offer  0.0.0.0/0
ssh                     192.168.1.0/16,10.0.0.0/8
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

func (s *ListSuite) TestListEmpty(c *gc.C) {
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
	rules string
	err   error
}

func (s *mockListAPI) Close() error {
	return nil
}

func (s *mockListAPI) ModelGet() (map[string]interface{}, error) {
	if s.err != nil {
		return nil, s.err
	}
	return testing.FakeConfig().Merge(testing.Attrs{
		config.SSHAllowKey:         s.rules,
		config.SAASIngressAllowKey: "0.0.0.0/0",
	}), nil
}
