// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package dummy_test

import (
	gc "launchpad.net/gocheck"

	"launchpad.net/juju-core/environs"
	"launchpad.net/juju-core/environs/config"
	"launchpad.net/juju-core/environs/configstore"
	"launchpad.net/juju-core/provider/dummy"
	"launchpad.net/juju-core/testing"
)

var _ = gc.Suite(&ConfigSuite{})

type ConfigSuite struct {
	testing.BaseSuite
}

func (s *ConfigSuite) TearDownTest(c *gc.C) {
	s.BaseSuite.TearDownTest(c)
	dummy.Reset()
}

func (*ConfigSuite) TestSecretAttrs(c *gc.C) {
	attrs := dummy.SampleConfig().Delete("secret")
	cfg, err := config.New(config.NoDefaults, attrs)
	c.Assert(err, gc.IsNil)
	ctx := testing.Context(c)
	env, err := environs.Prepare(cfg, ctx, configstore.NewMem())
	c.Assert(err, gc.IsNil)
	defer env.Destroy()
	expected := map[string]string{
		"secret": "pork",
	}
	actual, err := env.Provider().SecretAttrs(cfg)
	c.Assert(err, gc.IsNil)
	c.Assert(actual, gc.DeepEquals, expected)
}

var firewallModeTests = []struct {
	configFirewallMode string
	firewallMode       string
	errorMsg           string
}{
	{
		// Empty value leads to default value.
		firewallMode: config.FwInstance,
	}, {
		// Explicit default value.
		configFirewallMode: "",
		firewallMode:       config.FwInstance,
	}, {
		// Instance mode.
		configFirewallMode: "instance",
		firewallMode:       config.FwInstance,
	}, {
		// Global mode.
		configFirewallMode: "global",
		firewallMode:       config.FwGlobal,
	}, {
		// Invalid mode.
		configFirewallMode: "invalid",
		errorMsg:           `invalid firewall mode in environment configuration: "invalid"`,
	},
}

func (s *ConfigSuite) TestFirewallMode(c *gc.C) {
	for i, test := range firewallModeTests {
		c.Logf("test %d: %s", i, test.configFirewallMode)
		attrs := dummy.SampleConfig()
		if test.configFirewallMode != "" {
			attrs = attrs.Merge(testing.Attrs{
				"firewall-mode": test.configFirewallMode,
			})
		}
		cfg, err := config.New(config.NoDefaults, attrs)
		if err != nil {
			c.Assert(err, gc.ErrorMatches, test.errorMsg)
			continue
		}
		ctx := testing.Context(c)
		env, err := environs.Prepare(cfg, ctx, configstore.NewMem())
		if test.errorMsg != "" {
			c.Assert(err, gc.ErrorMatches, test.errorMsg)
			continue
		}
		c.Assert(err, gc.IsNil)
		defer env.Destroy()

		firewallMode := env.Config().FirewallMode()
		c.Assert(firewallMode, gc.Equals, test.firewallMode)

		s.TearDownTest(c)
	}
}
