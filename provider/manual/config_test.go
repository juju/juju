// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package manual

import (
	"fmt"
	"regexp"

	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/environs"
	"github.com/juju/juju/environs/config"
	coretesting "github.com/juju/juju/testing"
)

type configSuite struct {
	coretesting.FakeJujuXDGDataHomeSuite
}

var _ = gc.Suite(&configSuite{})

func CloudSpec() environs.CloudSpec {
	return environs.CloudSpec{
		Name:     "manual",
		Type:     "manual",
		Endpoint: "hostname",
	}
}

func MinimalConfigValues() map[string]interface{} {
	return map[string]interface{}{
		"name":            "test",
		"type":            "manual",
		"uuid":            coretesting.ModelTag.Id(),
		"controller-uuid": coretesting.ModelTag.Id(),
		"firewall-mode":   "instance",
		"bootstrap-user":  "",
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

func (s *configSuite) TestBootstrapUser(c *gc.C) {
	values := MinimalConfigValues()
	testConfig := getModelConfig(c, values)
	c.Assert(testConfig.bootstrapUser(), gc.Equals, "")
	values["bootstrap-user"] = "ubuntu"
	testConfig = getModelConfig(c, values)
	c.Assert(testConfig.bootstrapUser(), gc.Equals, "ubuntu")
}
