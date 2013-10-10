// Copyright 2013 Joyent Inc.
// Licensed under the AGPLv3, see LICENCE file for details.

package joyent_test

import (
	gc "launchpad.net/gocheck"

	"launchpad.net/juju-core/environs"
	"launchpad.net/juju-core/environs/config"
	"launchpad.net/juju-core/provider/joyent"
	"launchpad.net/juju-core/testing"
	"launchpad.net/juju-core/testing/testbase"
)

func newConfig(c *gc.C, attrs testing.Attrs) *config.Config {
	attrs = testing.FakeConfig().Merge(attrs)
	cfg, err := config.New(config.NoDefaults, attrs)
	c.Assert(err, gc.IsNil)
	return cfg
}

func validAttrs() testing.Attrs {
	return testing.FakeConfig().Merge(testing.Attrs{
		"type":                     "joyent",
		"user":						"dstroppa",
		"key-id":    				"12:c3:a7:cb:a2:29:e2:90:88:3f:04:53:3b:4e:75:40",
		"region": 					"eu-ams-1",
	})
}

type ConfigSuite struct {
	testbase.LoggingSuite
}

var _ = gc.Suite(&ConfigSuite{})

var newConfigTests = []struct {
	info   string
	insert testing.Attrs
	remove []string
	expect testing.Attrs
	err    string
}{{
	info:   "user is required",
	remove: []string{"user"},
	err:    "user: expected string, got nothing",
}, {
	info:   "user cannot be empty",
	insert: testing.Attrs{"user": ""},
	err:    "user: must not be empty",
}, {
	info:   "key-id is required",
	remove: []string{"key-id"},
	err:    "key-id: expected string, got nothing",
}, {
	info:   "key-id cannot be empty",
	insert: testing.Attrs{"key-id": ""},
	err:    "key-id: must not be empty",
}, {
	info:   "region is inserted if missing",
	expect: testing.Attrs{"region": "eu-ams-1"},
}, {
	info:   "region cannot be empty",
	insert: testing.Attrs{"region": ""},
	err:    "region: must not be empty",
}, {
	info:   "region is untouched if present",
	insert: testing.Attrs{"region": "eu-ams-1"},
	expect: testing.Attrs{"region": "eu-ams-1"},
}, {
	info:   "unknown field is not touched",
	insert: testing.Attrs{"unknown-field": 12345},
	expect: testing.Attrs{"unknown-field": 12345},
}}

func (*ConfigSuite) TestNewEnvironConfig(c *gc.C) {
	for i, test := range newConfigTests {
		c.Logf("test %d: %s", i, test.info)
		attrs := validAttrs().Merge(test.insert).Delete(test.remove...)
		testConfig := newConfig(c, attrs)
		environ, err := environs.New(testConfig)
		if test.err == "" {
			c.Check(err, gc.IsNil)
			attrs := environ.Config().AllAttrs()
			for field, value := range test.expect {
				c.Check(attrs[field], gc.Equals, value)
			}
		} else {
			c.Check(environ, gc.IsNil)
			c.Check(err, gc.ErrorMatches, test.err)
		}
	}
}

func (*ConfigSuite) TestValidateNewConfig(c *gc.C) {
	for i, test := range newConfigTests {
		c.Logf("test %d: %s", i, test.info)
		attrs := validAttrs().Merge(test.insert).Delete(test.remove...)
		testConfig := newConfig(c, attrs)
		validatedConfig, err := joyent.Provider.Validate(testConfig, nil)
		if test.err == "" {
			c.Check(err, gc.IsNil)
			attrs := validatedConfig.AllAttrs()
			for field, value := range test.expect {
				c.Check(attrs[field], gc.Equals, value)
			}
		} else {
			c.Check(validatedConfig, gc.IsNil)
			c.Check(err, gc.ErrorMatches, "invalid Joyent provider config: "+test.err)
		}
	}
}

func (*ConfigSuite) TestValidateOldConfig(c *gc.C) {
	knownGoodConfig := newConfig(c, validAttrs())
	for i, test := range newConfigTests {
		c.Logf("test %d: %s", i, test.info)
		attrs := validAttrs().Merge(test.insert).Delete(test.remove...)
		testConfig := newConfig(c, attrs)
		validatedConfig, err := joyent.Provider.Validate(knownGoodConfig, testConfig)
		if test.err == "" {
			c.Check(err, gc.IsNil)
			attrs := validatedConfig.AllAttrs()
			for field, value := range validAttrs() {
				c.Check(attrs[field], gc.Equals, value)
			}
		} else {
			c.Check(validatedConfig, gc.IsNil)
			c.Check(err, gc.ErrorMatches, "original Joyent provider config is invalid: "+test.err)
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
	info:   "can change user",
	insert: testing.Attrs{"user": "joyent_user"},
	expect: testing.Attrs{"user": "joyent_user"},
}, {
	info:   "can change key-id",
	insert: testing.Attrs{"key-id": "11:c4:b6:c0:a3:24:22:96:a8:1f:07:53:3f:8e:14:7a"},
	expect: testing.Attrs{"key-id": "11:c4:b6:c0:a3:24:22:96:a8:1f:07:53:3f:8e:14:7a"},
}, {
	info:   "can change region",
	insert: testing.Attrs{"region": "eu-ams-1"},
	expect: testing.Attrs{"region": "eu-ams-1"},
}, {
	info:   "can insert unknown field",
	insert: testing.Attrs{"unknown": "ignoti"},
	expect: testing.Attrs{"unknown": "ignoti"},
}}

func (s *ConfigSuite) TestValidateChange(c *gc.C) {
	baseConfig := newConfig(c, validAttrs())
	for i, test := range changeConfigTests {
		c.Logf("test %d: %s", i, test.info)
		attrs := validAttrs().Merge(test.insert).Delete(test.remove...)
		testConfig := newConfig(c, attrs)
		validatedConfig, err := joyent.Provider.Validate(testConfig, baseConfig)
		if test.err == "" {
			c.Check(err, gc.IsNil)
			attrs := validatedConfig.AllAttrs()
			for field, value := range test.expect {
				c.Check(attrs[field], gc.Equals, value)
			}
		} else {
			c.Check(validatedConfig, gc.IsNil)
			c.Check(err, gc.ErrorMatches, "invalid config change: "+test.err)
		}
	}
}

func (s *ConfigSuite) TestSetConfig(c *gc.C) {
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
