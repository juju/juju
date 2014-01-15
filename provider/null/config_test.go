// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package null

import (
	"fmt"
	"regexp"

	gc "launchpad.net/gocheck"

	"launchpad.net/juju-core/environs/config"
	"launchpad.net/juju-core/provider"
	coretesting "launchpad.net/juju-core/testing"
	"launchpad.net/juju-core/testing/testbase"
)

type configSuite struct {
	testbase.LoggingSuite
}

var _ = gc.Suite(&configSuite{})

func minimalConfigValues() map[string]interface{} {
	return map[string]interface{}{
		"name":             "test",
		"type":             provider.Null,
		"bootstrap-host":   "hostname",
		"storage-auth-key": "whatever",
		// While the ca-cert bits aren't entirely minimal, they avoid the need
		// to set up a fake home.
		"ca-cert":        coretesting.CACert,
		"ca-private-key": coretesting.CAKey,
	}
}

func minimalConfig(c *gc.C) *config.Config {
	minimal := minimalConfigValues()
	testConfig, err := config.New(config.UseDefaults, minimal)
	c.Assert(err, gc.IsNil)
	return testConfig
}

func getEnvironConfig(c *gc.C, attrs map[string]interface{}) *environConfig {
	testConfig, err := config.New(config.UseDefaults, attrs)
	c.Assert(err, gc.IsNil)
	envConfig, err := nullProvider{}.validate(testConfig, nil)
	c.Assert(err, gc.IsNil)
	return envConfig
}

func (s *configSuite) TestValidateConfig(c *gc.C) {
	testConfig := minimalConfig(c)
	testConfig, err := testConfig.Apply(map[string]interface{}{"bootstrap-host": ""})
	c.Assert(err, gc.IsNil)
	_, err = nullProvider{}.Validate(testConfig, nil)
	c.Assert(err, gc.ErrorMatches, "bootstrap-host must be specified")

	testConfig, err = testConfig.Apply(map[string]interface{}{"storage-auth-key": nil})
	c.Assert(err, gc.IsNil)
	_, err = nullProvider{}.Validate(testConfig, nil)
	c.Assert(err, gc.ErrorMatches, "storage-auth-key: expected string, got nothing")

	testConfig = minimalConfig(c)
	valid, err := nullProvider{}.Validate(testConfig, nil)
	c.Assert(err, gc.IsNil)

	unknownAttrs := valid.UnknownAttrs()
	c.Assert(unknownAttrs["bootstrap-host"], gc.Equals, "hostname")
	c.Assert(unknownAttrs["bootstrap-user"], gc.Equals, "")
	c.Assert(unknownAttrs["storage-listen-ip"], gc.Equals, "")
	c.Assert(unknownAttrs["storage-port"], gc.Equals, int64(8040))
}

func (s *configSuite) TestConfigMutability(c *gc.C) {
	testConfig := minimalConfig(c)
	valid, err := nullProvider{}.Validate(testConfig, nil)
	c.Assert(err, gc.IsNil)
	unknownAttrs := valid.UnknownAttrs()

	// Make sure the immutable values can't be changed. It'd be nice to be
	// able to change these, but that would involve somehow updating the
	// machine agent's config/upstart config.
	oldConfig := testConfig
	for k, v := range map[string]interface{}{
		"bootstrap-host":    "new-hostname",
		"bootstrap-user":    "new-username",
		"storage-listen-ip": "10.0.0.123",
		"storage-port":      int64(1234),
	} {
		testConfig = minimalConfig(c)
		testConfig, err = testConfig.Apply(map[string]interface{}{k: v})
		c.Assert(err, gc.IsNil)
		_, err := nullProvider{}.Validate(testConfig, oldConfig)
		oldv := unknownAttrs[k]
		errmsg := fmt.Sprintf("cannot change %s from %q to %q", k, oldv, v)
		c.Assert(err, gc.ErrorMatches, regexp.QuoteMeta(errmsg))
	}
}

func (s *configSuite) TestBootstrapHostUser(c *gc.C) {
	values := minimalConfigValues()
	testConfig := getEnvironConfig(c, values)
	c.Assert(testConfig.bootstrapHost(), gc.Equals, "hostname")
	c.Assert(testConfig.bootstrapUser(), gc.Equals, "")
	values["bootstrap-host"] = "127.0.0.1"
	values["bootstrap-user"] = "ubuntu"
	testConfig = getEnvironConfig(c, values)
	c.Assert(testConfig.bootstrapHost(), gc.Equals, "127.0.0.1")
	c.Assert(testConfig.bootstrapUser(), gc.Equals, "ubuntu")
}

func (s *configSuite) TestStorageParams(c *gc.C) {
	values := minimalConfigValues()
	testConfig := getEnvironConfig(c, values)
	c.Assert(testConfig.storageAddr(), gc.Equals, "hostname:8040")
	c.Assert(testConfig.storageListenAddr(), gc.Equals, ":8040")
	values["storage-listen-ip"] = "10.0.0.123"
	values["storage-port"] = int64(1234)
	testConfig = getEnvironConfig(c, values)
	c.Assert(testConfig.storageAddr(), gc.Equals, "hostname:1234")
	c.Assert(testConfig.storageListenAddr(), gc.Equals, "10.0.0.123:1234")
}
