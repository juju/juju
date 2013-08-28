// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package dummy_test

import (
	gc "launchpad.net/gocheck"

	"launchpad.net/juju-core/environs"
	"launchpad.net/juju-core/environs/config"
	_ "launchpad.net/juju-core/provider/dummy"
	"launchpad.net/juju-core/testing"
)

var _ = gc.Suite(&ConfigSuite{})

type ConfigSuite struct{}

func (*ConfigSuite) TestSecretAttrs(c *gc.C) {
	cfg, err := config.New(map[string]interface{}{
		"name":            "only", // must match the name in environs_test.go
		"type":            "dummy",
		"state-server":    true,
		"authorized-keys": "i-am-a-key",
		"ca-cert":         testing.CACert,
		"ca-private-key":  "",
	})
	c.Assert(err, gc.IsNil)
	env, err := environs.Prepare(cfg)
	c.Assert(err, gc.IsNil)
	expected := map[string]interface{}{
		"secret": "pork",
	}
	actual, err := env.Provider().SecretAttrs(cfg)
	c.Assert(err, gc.IsNil)
	c.Assert(expected, gc.DeepEquals, actual)
}

var firewallModeTests = []struct {
	configFirewallMode string
	firewallMode       config.FirewallMode
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

func (*ConfigSuite) TestFirewallMode(c *gc.C) {
	for _, test := range firewallModeTests {
		c.Logf("test firewall mode %q", test.configFirewallMode)
		cfgMap := map[string]interface{}{
			"name":            "only",
			"type":            "dummy",
			"state-server":    true,
			"authorized-keys": "none",
			"ca-cert":         testing.CACert,
			"ca-private-key":  "",
		}
		if test.configFirewallMode != "" {
			cfgMap["firewall-mode"] = test.configFirewallMode
		}
		cfg, err := config.New(cfgMap)
		if err != nil {
			c.Assert(err, gc.ErrorMatches, test.errorMsg)
			continue
		}

		env, err := environs.Prepare(cfg)
		if err != nil {
			c.Assert(err, gc.ErrorMatches, test.errorMsg)
			continue
		}

		firewallMode := env.Config().FirewallMode()
		c.Assert(firewallMode, gc.Equals, test.firewallMode)
	}
}
