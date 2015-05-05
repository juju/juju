// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package cloudsigma

import (
	"github.com/juju/schema"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/environs"
	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/testing"
)

func newConfig(c *gc.C, attrs testing.Attrs) *config.Config {
	attrs = testing.FakeConfig().Merge(attrs)
	cfg, err := config.New(config.NoDefaults, attrs)
	c.Assert(err, gc.IsNil)
	return cfg
}

func validAttrs() testing.Attrs {
	return testing.FakeConfig().Merge(testing.Attrs{
		"type":     "cloudsigma",
		"username": "user",
		"password": "password",
		"region":   "zrh",
		"uuid":     "f54aac3a-9dcd-4a0c-86b5-24091478478c",
	})
}

type configSuite struct {
	testing.BaseSuite
}

func (s *configSuite) SetUpSuite(c *gc.C) {
	s.BaseSuite.SetUpSuite(c)
}

func (s *configSuite) SetUpTest(c *gc.C) {
	s.BaseSuite.SetUpTest(c)
	// speed up tests, do not create heavy stuff inside providers created withing this test suite
	s.PatchValue(&newClient, func(cfg *environConfig) (*environClient, error) {
		return nil, nil
	})
}

var _ = gc.Suite(&configSuite{})

func (s *configSuite) TestNewEnvironConfig(c *gc.C) {

	type checker struct {
		checker gc.Checker
		value   interface{}
	}

	var newConfigTests = []struct {
		info   string
		insert testing.Attrs
		remove []string
		expect testing.Attrs
		err    string
	}{{
		info:   "username is required",
		remove: []string{"username"},
		err:    "username: must not be empty",
	}, {
		info:   "username cannot be empty",
		insert: testing.Attrs{"username": ""},
		err:    "username: must not be empty",
	}, {
		info:   "password is required",
		remove: []string{"password"},
		err:    "password: must not be empty",
	}, {
		info:   "password cannot be empty",
		insert: testing.Attrs{"password": ""},
		err:    "password: must not be empty",
	}, {
		info:   "region is inserted if missing",
		remove: []string{"region"},
		expect: testing.Attrs{"region": "zrh"},
	}, {
		info:   "region must not be empty",
		insert: testing.Attrs{"region": ""},
		err:    "region: must not be empty",
	}}

	for i, test := range newConfigTests {
		c.Logf("test %d: %s", i, test.info)
		attrs := validAttrs().Merge(test.insert).Delete(test.remove...)
		testConfig := newConfig(c, attrs)
		environ, err := environs.New(testConfig)
		if test.err == "" {
			c.Check(err, gc.IsNil)
			attrs := environ.Config().AllAttrs()
			for field, value := range test.expect {
				if chk, ok := value.(checker); ok {
					c.Check(attrs[field], chk.checker, chk.value)
				} else {
					c.Check(attrs[field], gc.Equals, value)
				}
			}
		} else {
			c.Check(environ, gc.IsNil)
			c.Check(err, gc.ErrorMatches, test.err)
		}
	}
}

var changeConfigTests = []struct {
	info   string
	insert testing.Attrs
	remove []string
	expect testing.Attrs
	err    string
}{{
	info:   "no change, no error",
	expect: validAttrs(),
}, {
	info:   "can change username",
	insert: testing.Attrs{"username": "cloudsigma_user"},
	expect: testing.Attrs{"username": "cloudsigma_user"},
}, {
	info:   "can not change username to empty",
	insert: testing.Attrs{"username": ""},
	err:    "username: must not be empty",
}, {
	info:   "can change password",
	insert: testing.Attrs{"password": "cloudsigma_password"},
	expect: testing.Attrs{"password": "cloudsigma_password"},
}, {
	info:   "can not change password to empty",
	insert: testing.Attrs{"password": ""},
	err:    "password: must not be empty",
}, {
	info:   "can change region",
	insert: testing.Attrs{"region": "lvs"},
	err:    "region: cannot change from .* to .*",
}}

func (s *configSuite) TestValidateChange(c *gc.C) {
	baseConfig := newConfig(c, validAttrs())
	for i, test := range changeConfigTests {
		c.Logf("test %d: %s", i, test.info)
		attrs := validAttrs().Merge(test.insert).Delete(test.remove...)
		testConfig := newConfig(c, attrs)
		validatedConfig, err := providerInstance.Validate(testConfig, baseConfig)
		if test.err == "" {
			c.Check(err, gc.IsNil)
			attrs := validatedConfig.AllAttrs()
			for field, value := range test.expect {
				c.Check(attrs[field], gc.Equals, value)
			}
		} else {
			c.Check(validatedConfig, gc.IsNil)
			c.Check(err, gc.ErrorMatches, "invalid config.*: "+test.err)
		}

		// reverse change
		validatedConfig, err = providerInstance.Validate(baseConfig, testConfig)
		if test.err == "" {
			c.Check(err, gc.IsNil)
			attrs := validatedConfig.AllAttrs()
			for field, value := range validAttrs() {
				c.Check(attrs[field], gc.Equals, value)
			}
		} else {
			c.Check(validatedConfig, gc.IsNil)
			c.Check(err, gc.ErrorMatches, "invalid .*config.*: "+test.err)
		}
	}
}

func (s *configSuite) TestSetConfig(c *gc.C) {
	baseConfig := newConfig(c, validAttrs())
	for i, test := range changeConfigTests {
		c.Logf("test %d: %s", i, test.info)
		environ, err := environs.New(baseConfig)
		c.Assert(err, gc.IsNil)
		attrs := validAttrs().Merge(test.insert).Delete(test.remove...)
		testConfig := newConfig(c, attrs)
		err = environ.SetConfig(testConfig)
		newAttrs := environ.Config().AllAttrs()
		if test.err == "" {
			c.Check(err, gc.IsNil)
			for field, value := range test.expect {
				c.Check(newAttrs[field], gc.Equals, value)
			}
		} else {
			c.Check(err, gc.ErrorMatches, test.err)
			for field, value := range baseConfig.UnknownAttrs() {
				c.Check(newAttrs[field], gc.Equals, value)
			}
		}
	}
}

func (s *configSuite) TestConfigName(c *gc.C) {
	baseConfig := newConfig(c, validAttrs().Merge(testing.Attrs{"name": "testname"}))
	environ, err := environs.New(baseConfig)
	c.Assert(err, gc.IsNil)
	c.Check(environ.Config().Name(), gc.Equals, "testname")
}

func (s *configSuite) TestEnvironConfig(c *gc.C) {
	testConfig := newConfig(c, validAttrs())
	ecfg, err := validateConfig(testConfig, nil)
	c.Assert(ecfg, gc.NotNil)
	c.Assert(err, gc.IsNil)
	c.Check(ecfg.username(), gc.Equals, "user")
	c.Check(ecfg.password(), gc.Equals, "password")
	c.Check(ecfg.region(), gc.Equals, "zrh")
}

func (s *configSuite) TestInvalidConfigChange(c *gc.C) {
	oldAttrs := validAttrs().Merge(testing.Attrs{"name": "123"})
	oldConfig := newConfig(c, oldAttrs)
	newAttrs := validAttrs().Merge(testing.Attrs{"name": "321"})
	newConfig := newConfig(c, newAttrs)

	oldecfg, _ := providerInstance.Validate(oldConfig, nil)
	c.Assert(oldecfg, gc.NotNil)

	newecfg, err := providerInstance.Validate(newConfig, oldecfg)
	c.Assert(newecfg, gc.IsNil)
	c.Assert(err, gc.NotNil)
}

var secretAttrsConfigTests = []struct {
	info   string
	insert testing.Attrs
	remove []string
	expect map[string]string
	err    string
}{{
	info:   "no change, no error",
	expect: map[string]string{"password": "password"},
}, {
	info:   "invalid config",
	insert: testing.Attrs{"username": ""},
	err:    ".* must not be empty.*",
}}

func (s *configSuite) TestSecretAttrs(c *gc.C) {
	for i, test := range secretAttrsConfigTests {
		c.Logf("test %d: %s", i, test.info)
		attrs := validAttrs().Merge(test.insert).Delete(test.remove...)
		testConfig := newConfig(c, attrs)
		sa, err := providerInstance.SecretAttrs(testConfig)
		if test.err == "" {
			c.Check(sa, gc.HasLen, len(test.expect))
			for field, value := range test.expect {
				c.Check(sa[field], gc.Equals, value)
			}
			c.Check(err, gc.IsNil)
		} else {
			c.Check(sa, gc.IsNil)
			c.Check(err, gc.ErrorMatches, test.err)
		}
	}
}

func (s *configSuite) TestSecretAttrsAreStrings(c *gc.C) {
	for i, field := range configSecretFields {
		c.Logf("test %d: %s", i, field)
		attrs := validAttrs().Merge(testing.Attrs{field: 0})

		if v, ok := configFields[field]; ok {
			configFields[field] = schema.ForceInt()
			defer func(c schema.Checker) {
				configFields[field] = c
			}(v)
		} else {
			c.Errorf("secrect field %s not found in configFields", field)
			continue
		}

		testConfig := newConfig(c, attrs)
		sa, err := providerInstance.SecretAttrs(testConfig)
		c.Check(sa, gc.IsNil)
		c.Check(err, gc.ErrorMatches, "secret .* field must have a string value; got .*")
	}
}

func (s *configSuite) TestClientConfigChanged(c *gc.C) {
	ecfg := &environConfig{
		Config: newConfig(c, testing.Attrs{"name": "client-test"}),
		attrs: map[string]interface{}{
			"region":   "https://testing.invalid",
			"username": "user",
			"password": "password",
		},
	}

	oldConfig := &environConfig{
		Config: newConfig(c, testing.Attrs{"name": "client-test"}),
		attrs: map[string]interface{}{
			"region":   "https://testing.invalid",
			"username": "user",
			"password": "password",
		},
	}

	rc := oldConfig.clientConfigChanged(ecfg)
	c.Check(rc, gc.Equals, false)

	ecfg.attrs["region"] = ""
	rc = oldConfig.clientConfigChanged(ecfg)
	c.Check(rc, gc.Equals, true)

	ecfg.attrs["region"] = "https://testing.invalid"
	ecfg.attrs["username"] = "user1"
	rc = oldConfig.clientConfigChanged(ecfg)
	c.Check(rc, gc.Equals, true)

	ecfg.attrs["username"] = "user"
	ecfg.attrs["password"] = "password1"
	rc = oldConfig.clientConfigChanged(ecfg)
	c.Check(rc, gc.Equals, true)
}
