// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package azure

import (
	. "launchpad.net/gocheck"
	"launchpad.net/juju-core/environs/config"
	"launchpad.net/juju-core/testing"
)

type ConfigSuite struct{}

var _ = Suite(new(ConfigSuite))

// makeConfigMap creates a minimal map of standard configuration items.
// It's just the bare minimum to produce a configuration object.
func makeConfigMap() map[string]interface{} {
	return map[string]interface{}{
		"name":            "testenv",
		"type":            "azure",
		"ca-cert":         testing.CACert,
		"ca-private-key":  testing.CAKey,
	}
}

func (ConfigSuite) TestParsesSettings(c *C) {
	configMap := makeConfigMap()
	config, err := config.New(configMap)
	c.Assert(err, IsNil)
	c.Assert(config, NotNil)
	c.Check(config.Name(), Equals, configMap["name"])
}
