package dummy_test

import (
	. "launchpad.net/gocheck"
	"launchpad.net/juju-core/environs"
	"launchpad.net/juju-core/environs/config"
	_ "launchpad.net/juju-core/environs/dummy"
)

var _ = Suite(&ConfigSuite{})

type ConfigSuite struct{}

func (*ConfigSuite) TestSecretAttrs(c *C) {
	cfg, err := config.New(map[string]interface{}{
		"name":            "only", // must match the name in environs_test.go
		"type":            "dummy",
		"state-server":    true,
		"authorized-keys": "i-am-a-key",
	})
	c.Assert(err, IsNil)
	env, err := environs.New(cfg)
	c.Assert(err, IsNil)
	expected := map[string]interface{}{
		"secret": "pork",
	}
	actual, err := env.Provider().SecretAttrs(cfg)
	c.Assert(err, IsNil)
	c.Assert(expected, DeepEquals, actual)
}

func (*ConfigSuite) TestFirewallMode(c *C) {
	cfg, err := config.New(map[string]interface{}{
		"name":            "only",
		"type":            "dummy",
		"state-server":    true,
		"authorized-keys": "i-am-a-key",
	})
	c.Assert(err, IsNil)
	env, err := environs.New(cfg)
	c.Assert(err, IsNil)
	firewallMode := env.Config().FirewallMode()
	c.Assert(firewallMode, Equals, config.FwInstance)

	cfg, err = config.New(map[string]interface{}{
		"name":            "only",
		"type":            "dummy",
		"state-server":    true,
		"authorized-keys": "i-am-a-key",
		"firewall-mode":   "default",
	})
	c.Assert(err, IsNil)
	env, err = environs.New(cfg)
	c.Assert(err, IsNil)
	firewallMode = env.Config().FirewallMode()
	c.Assert(firewallMode, Equals, config.FwInstance)

	cfg, err = config.New(map[string]interface{}{
		"name":            "only",
		"type":            "dummy",
		"state-server":    true,
		"authorized-keys": "i-am-a-key",
		"firewall-mode":   "instance",
	})
	c.Assert(err, IsNil)
	env, err = environs.New(cfg)
	c.Assert(err, IsNil)
	firewallMode = env.Config().FirewallMode()
	c.Assert(firewallMode, Equals, config.FwInstance)

	cfg, err = config.New(map[string]interface{}{
		"name":            "only",
		"type":            "dummy",
		"state-server":    true,
		"authorized-keys": "i-am-a-key",
		"firewall-mode":   "global",
	})
	c.Assert(err, IsNil)
	env, err = environs.New(cfg)
	c.Assert(err, ErrorMatches, `provider does not support global firewall mode`)

	cfg, err = config.New(map[string]interface{}{
		"name":            "only",
		"type":            "dummy",
		"state-server":    true,
		"authorized-keys": "i-am-a-key",
		"firewall-mode":   "invalid",
	})
	c.Assert(err, ErrorMatches, `invalid firewall mode in environment configuration: .*`)
}
