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
	coretesting.FakeJujuXDGDataHomeSuite
}

var _ = gc.Suite(&configSuite{})

func MinimalConfigValues() map[string]interface{} {
	return map[string]interface{}{
		"name":           "test",
		"type":           "manual",
		"bootstrap-host": "hostname",
		"bootstrap-user": "",
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

func getModelConfig(c *gc.C, attrs map[string]interface{}) *environConfig {
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

	values := MinimalConfigValues()
	delete(values, "bootstrap-user")
	testConfig, err = config.New(config.UseDefaults, values)
	c.Assert(err, jc.ErrorIsNil)

	valid, err := manualProvider{}.Validate(testConfig, nil)
	c.Assert(err, jc.ErrorIsNil)
	unknownAttrs := valid.UnknownAttrs()
	c.Assert(unknownAttrs["bootstrap-host"], gc.Equals, "hostname")
	c.Assert(unknownAttrs["bootstrap-user"], gc.Equals, "")
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
		"bootstrap-host": "new-hostname",
		"bootstrap-user": "new-username",
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
	testConfig := getModelConfig(c, values)
	c.Assert(testConfig.bootstrapHost(), gc.Equals, "hostname")
	c.Assert(testConfig.bootstrapUser(), gc.Equals, "")
	values["bootstrap-host"] = "127.0.0.1"
	values["bootstrap-user"] = "ubuntu"
	testConfig = getModelConfig(c, values)
	c.Assert(testConfig.bootstrapHost(), gc.Equals, "127.0.0.1")
	c.Assert(testConfig.bootstrapUser(), gc.Equals, "ubuntu")
}

func (s *configSuite) TestStorageCompat(c *gc.C) {
	// Older environment configurations will not have the
	// use-sshstorage attribute. We treat them as if they
	// have use-sshstorage=false.
	values := MinimalConfigValues()
	delete(values, "use-sshstorage")
	cfg, err := config.New(config.UseDefaults, values)
	c.Assert(err, jc.ErrorIsNil)
	envConfig := newModelConfig(cfg, values)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(envConfig.useSSHStorage(), jc.IsFalse)
}
