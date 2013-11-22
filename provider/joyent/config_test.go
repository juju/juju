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
		"type":         "joyent",
		"sdc-user":     "dstroppa",
		"sdc-key-id":   "12:c3:a7:cb:a2:29:e2:90:88:3f:04:53:3b:4e:75:40",
		"sdc-region":   "us-west-1",
		"manta-user":   "dstroppa",
		"manta-key-id": "12:c3:a7:cb:a2:29:e2:90:88:3f:04:53:3b:4e:75:40",
		"manta-region": "us-east",
		"control-dir":  "juju-test",
	})
}

type ConfigSuite struct {
	testbase.LoggingSuite
	originalValues map[string]testbase.Restorer
}

var _ = gc.Suite(&ConfigSuite{})

func (s *ConfigSuite) SetUpSuite(c *gc.C) {
	s.PatchEnvironment(joyent.SdcAccount, "tester")
	s.PatchEnvironment(joyent.SdcKeyId, "11:c4:b6:c0:a3:24:22:96:a8:1f:07:53:3f:8e:14:7a")
	s.PatchEnvironment(joyent.MantaUser, "tester")
	s.PatchEnvironment(joyent.MantaKeyId, "11:c4:b6:c0:a3:24:22:96:a8:1f:07:53:3f:8e:14:7a")
}

var newConfigTests = []struct {
	info   string
	insert testing.Attrs
	remove []string
	expect testing.Attrs
	err    string
}{{
	info:   "sdc-user is required",
	remove: []string{"sdc-user"},
	err:    "sdc-user: expected string, got nothing",
}, {
	info:   "sdc-user cannot be empty",
	insert: testing.Attrs{"sdc-user": ""},
	err:    "sdc-user: must not be empty",
}, {
	info:   "sdc-key-id is required",
	remove: []string{"sdc-key-id"},
	err:    "sdc-key-id: expected string, got nothing",
}, {
	info:   "sdc-key-id cannot be empty",
	insert: testing.Attrs{"sdc-key-id": ""},
	err:    "sdc-key-id: must not be empty",
}, {
	info:   "sdc-region is inserted if missing",
	expect: testing.Attrs{"sdc-region": "us-west-1"},
}, {
	info:   "sdc-region cannot be empty",
	insert: testing.Attrs{"sdc-region": ""},
	err:    "sdc-region: must not be empty",
}, {
	info:   "sdc-region is untouched if present",
	insert: testing.Attrs{"sdc-region": "us-west-1"},
	expect: testing.Attrs{"sdc-region": "us-west-1"},
}, {
	info:   "manta-user is required",
	remove: []string{"manta-user"},
	err:    "manta-user: expected string, got nothing",
}, {
	info:   "manta-user cannot be empty",
	insert: testing.Attrs{"manta-user": ""},
	err:    "manta-user: must not be empty",
}, {
	info:   "manta-key-id is required",
	remove: []string{"manta-key-id"},
	err:    "manta-key-id: expected string, got nothing",
}, {
	info:   "manta-key-id cannot be empty",
	insert: testing.Attrs{"manta-key-id": ""},
	err:    "manta-key-id: must not be empty",
}, {
	info:   "manta-region is inserted if missing",
	expect: testing.Attrs{"manta-region": "us-east"},
}, {
	info:   "manta-region cannot be empty",
	insert: testing.Attrs{"manta-region": ""},
	err:    "manta-region: must not be empty",
}, {
	info:   "manta-region is untouched if present",
	insert: testing.Attrs{"manta-region": "us-east"},
	expect: testing.Attrs{"manta-region": "us-east"},
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
			c.Assert(err, gc.IsNil)
			attrs := environ.Config().AllAttrs()
			for field, value := range test.expect {
				c.Check(attrs[field], gc.Equals, value)
			}
		} else {
			c.Assert(environ, gc.IsNil)
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
			c.Assert(err, gc.IsNil)
			attrs := validatedConfig.AllAttrs()
			for field, value := range test.expect {
				c.Check(attrs[field], gc.Equals, value)
			}
		} else {
			c.Assert(validatedConfig, gc.IsNil)
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
			c.Assert(err, gc.IsNil)
			attrs := validatedConfig.AllAttrs()
			for field, value := range validAttrs() {
				c.Check(attrs[field], gc.Equals, value)
			}
		} else {
			c.Assert(validatedConfig, gc.IsNil)
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
	info:   "can change sdc-user",
	insert: testing.Attrs{"sdc-user": "joyent_user"},
	expect: testing.Attrs{"sdc-user": "joyent_user"},
}, {
	info:   "can change sdc-key-id",
	insert: testing.Attrs{"sdc-key-id": "11:c4:b6:c0:a3:24:22:96:a8:1f:07:53:3f:8e:14:7a"},
	expect: testing.Attrs{"sdc-key-id": "11:c4:b6:c0:a3:24:22:96:a8:1f:07:53:3f:8e:14:7a"},
}, {
	info:   "can change sdc-region",
	insert: testing.Attrs{"sdc-region": "us-west-1"},
	expect: testing.Attrs{"sdc-region": "us-west-1"},
}, {
	info:   "can change manta-user",
	insert: testing.Attrs{"manta-user": "manta_user"},
	expect: testing.Attrs{"manta-user": "manta_user"},
}, {
	info:   "can change manta-key-id",
	insert: testing.Attrs{"manta-key-id": "11:c4:b6:c0:a3:24:22:96:a8:1f:07:53:3f:8e:14:7a"},
	expect: testing.Attrs{"manta-key-id": "11:c4:b6:c0:a3:24:22:96:a8:1f:07:53:3f:8e:14:7a"},
}, {
	info:   "can change manta-region",
	insert: testing.Attrs{"manta-region": "us-east"},
	expect: testing.Attrs{"manta-region": "us-east"},
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
			c.Assert(err, gc.IsNil)
			attrs := validatedConfig.AllAttrs()
			for field, value := range test.expect {
				c.Check(attrs[field], gc.Equals, value)
			}
		} else {
			c.Assert(validatedConfig, gc.IsNil)
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
			c.Assert(err, gc.IsNil)
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

var prepareConfigTests = []struct {
	info   string
	insert testing.Attrs
	remove []string
	expect testing.Attrs
	err    string
}{{
	info:   "All value provided, nothig to do",
	expect: validAttrs(),
}, {
	info:   "can get sdc-user from env variable",
	insert: testing.Attrs{"sdc-user": ""},
	expect: testing.Attrs{"sdc-user": "tester"},
}, {
	info:   "can get sdc-key-id from env variable",
	insert: testing.Attrs{"sdc-key-id": ""},
	expect: testing.Attrs{"sdc-key-id": "11:c4:b6:c0:a3:24:22:96:a8:1f:07:53:3f:8e:14:7a"},
}, {
	info:   "can get manta-user from env variable",
	insert: testing.Attrs{"manta-user": ""},
	expect: testing.Attrs{"manta-user": "tester"},
}, {
	info:   "can get manta-key-id from env variable",
	insert: testing.Attrs{"manta-key-id": ""},
	expect: testing.Attrs{"manta-key-id": "11:c4:b6:c0:a3:24:22:96:a8:1f:07:53:3f:8e:14:7a"},
}}

func (s *ConfigSuite) TestPrepare(c *gc.C) {
	for i, test := range prepareConfigTests {
		c.Logf("test %d: %s", i, test.info)
		attrs := validAttrs().Merge(test.insert).Delete(test.remove...)
		testConfig := newConfig(c, attrs)
		preparedConfig, err := joyent.Provider.Prepare(testConfig)
		if test.err == "" {
			c.Assert(err, gc.IsNil)
			attrs := preparedConfig.Config().AllAttrs()
			for field, value := range test.expect {
				c.Check(attrs[field], gc.Equals, value)
			}
		} else {
			c.Assert(preparedConfig, gc.IsNil)
			c.Check(err, gc.ErrorMatches, "invalid prepare config: "+test.err)
		}
	}
}
