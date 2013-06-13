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
		"name":           "testenv",
		"type":           "azure",
		"ca-cert":        testing.CACert,
		"ca-private-key": testing.CAKey,
	}
}

func (ConfigSuite) TestNewParsesSettings(c *C) {
	attrs := makeConfigMap()
	config, err := config.New(attrs)
	c.Assert(err, IsNil)
	c.Assert(config, NotNil)
	c.Check(config.Name(), Equals, attrs["name"])
}

func (ConfigSuite) TestValidateAcceptsNilOldConfig(c *C) {
	provider := azureEnvironProvider{}
	attrs := makeConfigMap()
	config, err := config.New(attrs)
	c.Assert(err, IsNil)
	result, err := provider.Validate(config, nil)
	c.Assert(err, IsNil)
	c.Check(result.Name(), Equals, attrs["name"])
}

func (ConfigSuite) TestValidateAcceptsUnchangedConfig(c *C) {
	provider := azureEnvironProvider{}
	attrs := makeConfigMap()
	oldConfig, err := config.New(attrs)
	c.Assert(err, IsNil)
	newConfig, err := config.New(attrs)
	c.Assert(err, IsNil)
	result, err := provider.Validate(newConfig, oldConfig)
	c.Assert(err, IsNil)
	c.Check(result.Name(), Equals, attrs["name"])
}

func (ConfigSuite) TestValidateChecksConfigChanges(c *C) {
	provider := azureEnvironProvider{}
	oldAttrs := makeConfigMap()
	oldConfig, err := config.New(oldAttrs)
	c.Assert(err, IsNil)
	newAttrs := makeConfigMap()
	newAttrs["name"] = "different-name"
	newConfig, err := config.New(newAttrs)
	c.Assert(err, IsNil)
	_, err = provider.Validate(newConfig, oldConfig)
	c.Check(err, NotNil)
}
