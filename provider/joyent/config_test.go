// Copyright 2013 Joyent Inc.
// Licensed under the AGPLv3, see LICENCE file for details.

package joyent_test

import (
	gc "launchpad.net/gocheck"

	"launchpad.net/juju-core/environs"
	"launchpad.net/juju-core/environs/config"
	jp "launchpad.net/juju-core/provider/joyent"
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
		"sdc-user":     "juju-test",
		"sdc-key-id":   "00:11:22:33:44:55:66:77:88:99:aa:bb:cc:dd:ee:ff",
		"sdc-url":      "https://test.api.joyentcloud.com",
		"manta-user":   "juju-test",
		"manta-key-id": "00:11:22:33:44:55:66:77:88:99:aa:bb:cc:dd:ee:ff",
		"manta-url":    "https://test.manta.joyent.com",
		"key-file":     "~/.ssh/id_rsa",
		"algorithm":    "rsa-sha256",
		"control-dir":  "juju-test",
	})
}

type ConfigSuite struct {
	testbase.LoggingSuite
	originalValues map[string]testbase.Restorer
}

var _ = gc.Suite(&ConfigSuite{})

func (s *ConfigSuite) SetUpSuite(c *gc.C) {
	s.PatchEnvironment(jp.SdcAccount, "tester")
	s.PatchEnvironment(jp.SdcKeyId, "ff:ee:dd:cc:bb:aa:99:88:77:66:55:44:33:22:11:00")
	s.PatchEnvironment(jp.MantaUser, "tester")
	s.PatchEnvironment(jp.MantaKeyId, "ff:ee:dd:cc:bb:aa:99:88:77:66:55:44:33:22:11:00")
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
	info:   "sdc-url is inserted if missing",
	expect: testing.Attrs{"sdc-url": "https://test.api.joyentcloud.com"},
}, {
	info:   "sdc-url cannot be empty",
	insert: testing.Attrs{"sdc-url": ""},
	err:    "sdc-url: must not be empty",
}, {
	info:   "sdc-url is untouched if present",
	insert: testing.Attrs{"sdc-url": "https://test.api.joyentcloud.com"},
	expect: testing.Attrs{"sdc-url": "https://test.api.joyentcloud.com"},
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
	info:   "manta-url is inserted if missing",
	expect: testing.Attrs{"manta-url": "https://test.manta.joyent.com"},
}, {
	info:   "manta-url cannot be empty",
	insert: testing.Attrs{"manta-url": ""},
	err:    "manta-url: must not be empty",
}, {
	info:   "manta-url is untouched if present",
	insert: testing.Attrs{"manta-url": "https://test.manta.joyent.com"},
	expect: testing.Attrs{"manta-url": "https://test.manta.joyent.com"},
}, {
	info:   "key-file is inserted if missing",
	expect: testing.Attrs{"key-file": "~/.ssh/id_rsa"},
}, {
	info:   "key-file cannot be empty",
	insert: testing.Attrs{"key-file": ""},
	err:    "key-file: must not be empty",
}, {
	info:   "algorithm is inserted if missing",
	expect: testing.Attrs{"algorithm": "rsa-sha256"},
}, {
	info:   "algorithm cannot be empty",
	insert: testing.Attrs{"algorithm": ""},
	err:    "algorithm: must not be empty",
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
		validatedConfig, err := jp.Provider.Validate(testConfig, nil)
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
		validatedConfig, err := jp.Provider.Validate(knownGoodConfig, testConfig)
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
	insert: testing.Attrs{"sdc-key-id": "ff:ee:dd:cc:bb:aa:99:88:77:66:55:44:33:22:11:00"},
	expect: testing.Attrs{"sdc-key-id": "ff:ee:dd:cc:bb:aa:99:88:77:66:55:44:33:22:11:00"},
}, {
	info:   "can change sdc-url",
	insert: testing.Attrs{"sdc-url": "https://test.api.joyentcloud.com"},
	expect: testing.Attrs{"sdc-url": "https://test.api.joyentcloud.com"},
}, {
	info:   "can change manta-user",
	insert: testing.Attrs{"manta-user": "manta_user"},
	expect: testing.Attrs{"manta-user": "manta_user"},
}, {
	info:   "can change manta-key-id",
	insert: testing.Attrs{"manta-key-id": "ff:ee:dd:cc:bb:aa:99:88:77:66:55:44:33:22:11:00"},
	expect: testing.Attrs{"manta-key-id": "ff:ee:dd:cc:bb:aa:99:88:77:66:55:44:33:22:11:00"},
}, {
	info:   "can change manta-url",
	insert: testing.Attrs{"manta-url": "https://test.manta.joyent.com"},
	expect: testing.Attrs{"manta-url": "https://test.manta.joyent.com"},
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
		validatedConfig, err := jp.Provider.Validate(testConfig, baseConfig)
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
	expect: testing.Attrs{"sdc-key-id": "ff:ee:dd:cc:bb:aa:99:88:77:66:55:44:33:22:11:00"},
}, {
	info:   "can get manta-user from env variable",
	insert: testing.Attrs{"manta-user": ""},
	expect: testing.Attrs{"manta-user": "tester"},
}, {
	info:   "can get manta-key-id from env variable",
	insert: testing.Attrs{"manta-key-id": ""},
	expect: testing.Attrs{"manta-key-id": "ff:ee:dd:cc:bb:aa:99:88:77:66:55:44:33:22:11:00"},
}}

func (s *ConfigSuite) TestPrepare(c *gc.C) {
	for i, test := range prepareConfigTests {
		c.Logf("test %d: %s", i, test.info)
		attrs := validAttrs().Merge(test.insert).Delete(test.remove...)
		testConfig := newConfig(c, attrs)
		preparedConfig, err := jp.Provider.Prepare(testConfig)
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
