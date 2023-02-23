// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package featuretests

import (
	"bytes"

	"github.com/juju/cmd/v3/cmdtesting"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/cmd/juju/firewall"
	jujutesting "github.com/juju/juju/juju/testing"
)

type FirewallRulesSuite struct {
	jujutesting.JujuConnSuite
}

func (s *FirewallRulesSuite) TestFirewallRules(c *gc.C) {
	_, err := cmdtesting.RunCommand(c, firewall.NewSetFirewallRuleCommand(), "ssh", "--allowlist", "192.168.1.0/16")
	c.Assert(err, jc.ErrorIsNil)
	ctx, err := cmdtesting.RunCommand(c, firewall.NewListFirewallRulesCommand(), "--format", "yaml")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(ctx.Stdout.(*bytes.Buffer).String(), gc.Equals, `
- known-service: ssh
  allowlist-subnets:
  - 192.168.1.0/16
- known-service: juju-application-offer
  allowlist-subnets:
  - 0.0.0.0/0
  - ::/0
`[1:])
}
