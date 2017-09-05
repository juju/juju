// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package featuretests

import (
	"bytes"

	"github.com/juju/cmd/cmdtesting"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/cmd/juju/firewall"
	jujutesting "github.com/juju/juju/juju/testing"
)

type FirewallRulesSuite struct {
	jujutesting.JujuConnSuite
}

func (s *FirewallRulesSuite) TestFirewallRules(c *gc.C) {
	_, err := cmdtesting.RunCommand(c, firewall.NewSetFirewallRuleCommand(), "ssh", "--whitelist", "192.168.1.0/16", "--blacklist", "10.0.0.0/8")
	c.Assert(err, jc.ErrorIsNil)
	ctx, err := cmdtesting.RunCommand(c, firewall.NewListFirewallRulesCommand(), "--format", "yaml")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(ctx.Stdout.(*bytes.Buffer).String(), gc.Equals, `
- known-service: ssh
  whitelist-subnets:
  - 192.168.1.0/16
  blacklist-subnets:
  - 10.0.0.0/8
`[1:])
}
