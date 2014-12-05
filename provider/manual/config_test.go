// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package manual

import (
	"fmt"
	"regexp"

	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/environs/config"
	coretesting "github.com/juju/juju/testing"
)

type configSuite struct {
	coretesting.FakeJujuHomeSuite
}

var _ = gc.Suite(&configSuite{})

func MinimalConfigValues() map[string]interface{} {
	return map[string]interface{}{
		"name":              "test",
		"type":              "manual",
		"bootstrap-host":    "hostname",
		"bootstrap-user":    "",
		"storage-auth-key":  "whatever",
		"storage-port":      8040,
		"storage-listen-ip": "",
		// Not strictly necessary, but simplifies testing by disabling
		// ssh storage by default.
		"use-sshstorage": false,
		// While the ca-cert bits aren't entirely minimal, they avoid the need
		// to set up a fake home.
		"ca-cert":        coretesting.CACert,
		"ca-private-key": coretesting.CAKey,
	}
}

func MinimalConfig(c *gc.C) *config.Config {
	minimal := MinimalConfigValues()
	testConfig, err := config.New(config.UseDefaults, minimal)
	c.Assert(err, jc.ErrorIsNil)
	return testConfig
}

func getEnvironConfig(c *gc.C, attrs map[string]interface{}) *environConfig {
	testConfig, err := config.New(config.UseDefaults, attrs)
	c.Assert(err, jc.ErrorIsNil)
	envConfig, err := manualProvider{}.validate(testConfig, nil)
	c.Assert(err, jc.ErrorIsNil)
	return envConfig
}

func (s *configSuite) TestValidateConfig(c *gc.C) {
	testConfig := MinimalConfig(c)
	testConfig, err := testConfig.Apply(map[string]interface{}{"bootstrap-host": ""})
	c.Assert(err, jc.ErrorIsNil)
	_, err = manualProvider{}.Validate(testConfig, nil)
	c.Assert(err, gc.ErrorMatches, "bootstrap-host must be specified")

	testConfig, err = testConfig.Apply(map[string]interface{}{"storage-auth-key": nil})
	c.Assert(err, jc.ErrorIsNil)
	_, err = manualProvider{}.Validate(testConfig, nil)
	c.Assert(err, gc.ErrorMatches, "storage-auth-key: expected string, got nothing")

	values := MinimalConfigValues()
	delete(values, "bootstrap-user")
	delete(values, "storage-listen-ip")
	delete(values, "storage-port")
	testConfig, err = config.New(config.UseDefaults, values)
	c.Assert(err, jc.ErrorIsNil)

	valid, err := manualProvider{}.Validate(testConfig, nil)
	c.Assert(err, jc.ErrorIsNil)
	unknownAttrs := valid.UnknownAttrs()
	c.Assert(unknownAttrs["bootstrap-host"], gc.Equals, "hostname")
	c.Assert(unknownAttrs["bootstrap-user"], gc.Equals, "")
	c.Assert(unknownAttrs["storage-listen-ip"], gc.Equals, "")
	c.Assert(unknownAttrs["storage-port"], gc.Equals, int(8040))
}

func (s *configSuite) TestConfigMutability(c *gc.C) {
	testConfig := MinimalConfig(c)
	valid, err := manualProvider{}.Validate(testConfig, nil)
	c.Assert(err, jc.ErrorIsNil)
	unknownAttrs := valid.UnknownAttrs()

	// Make sure the immutable values can't be changed. It'd be nice to be
	// able to change these, but that would involve somehow updating the
	// machine agent's config/upstart config.
	oldConfig := testConfig
	for k, v := range map[string]interface{}{
		"bootstrap-host":    "new-hostname",
		"bootstrap-user":    "new-username",
		"storage-listen-ip": "10.0.0.123",
		"storage-port":      1234,
	} {
		testConfig = MinimalConfig(c)
		testConfig, err = testConfig.Apply(map[string]interface{}{k: v})
		c.Assert(err, jc.ErrorIsNil)
		_, err := manualProvider{}.Validate(testConfig, oldConfig)
		oldv := unknownAttrs[k]
		errmsg := fmt.Sprintf("cannot change %s from %q to %q", k, oldv, v)
		c.Assert(err, gc.ErrorMatches, regexp.QuoteMeta(errmsg))
	}
}

func (s *configSuite) TestBootstrapHostUser(c *gc.C) {
	values := MinimalConfigValues()
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
	values := MinimalConfigValues()
	testConfig := getEnvironConfig(c, values)
	c.Assert(testConfig.storageAddr(), gc.Equals, "hostname:8040")
	c.Assert(testConfig.storageListenAddr(), gc.Equals, ":8040")
	values["storage-listen-ip"] = "10.0.0.123"
	values["storage-port"] = 1234
	testConfig = getEnvironConfig(c, values)
	c.Assert(testConfig.storageAddr(), gc.Equals, "hostname:1234")
	c.Assert(testConfig.storageListenAddr(), gc.Equals, "10.0.0.123:1234")
}

func (s *configSuite) TestStorageCompat(c *gc.C) {
	// Older environment configurations will not have the
	// use-sshstorage attribute. We treat them as if they
	// have use-sshstorage=false.
	values := MinimalConfigValues()
	delete(values, "use-sshstorage")
	cfg, err := config.New(config.UseDefaults, values)
	c.Assert(err, jc.ErrorIsNil)
	envConfig := newEnvironConfig(cfg, values)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(envConfig.useSSHStorage(), jc.IsFalse)
}

func (s *configSuite) TestConfigWithFloatStoragePort(c *gc.C) {
	// When the config values get serialized through JSON, the integers
	// get coerced to float64 values.  The parsing needs to handle this.
	values := MinimalConfigValues()
	values["storage-port"] = float64(8040)

	cfg, err := config.New(config.UseDefaults, values)
	c.Assert(err, jc.ErrorIsNil)
	valid, err := ProviderInstance.Validate(cfg, nil)
	c.Assert(err, jc.ErrorIsNil)
	unknownAttrs := valid.UnknownAttrs()
	c.Assert(unknownAttrs["storage-port"], gc.Equals, int(8040))

	env, err := ProviderInstance.Open(cfg)
	c.Assert(err, jc.ErrorIsNil)
	// really, we're asserting that this doesn't panic :)
	c.Assert(env.(*manualEnviron).cfg.storagePort(), gc.Equals, int(8040))
}
