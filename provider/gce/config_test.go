// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package gce_test

import (
	gitjujutesting "github.com/juju/testing"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/environs"
	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/provider/gce"
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
		"type":                "gce",
		"gce-secret-field":    "seekrit",
		"gce-immutable-field": "static",
	})
}

type ConfigSuite struct {
	gitjujutesting.LoggingSuite
}

var _ = gc.Suite(&ConfigSuite{})

var newConfigTests = []struct {
	info   string
	insert testing.Attrs
	remove []string
	expect testing.Attrs
	err    string
}{{
	info:   "gce-immutable-field is required",
	remove: []string{"gce-immutable-field"},
	err:    "gce-immutable-field: expected string, got nothing",
}, {
	info:   "gce-immutable-field cannot be empty",
	insert: testing.Attrs{"gce-immutable-field": ""},
	err:    "gce-immutable-field: must not be empty",
}, {
	info:   "gce-secret-field is required",
	remove: []string{"gce-secret-field"},
	err:    "gce-secret-field: expected string, got nothing",
}, {
	info:   "gce-secret-field cannot be empty",
	insert: testing.Attrs{"gce-secret-field": ""},
	err:    "gce-secret-field: must not be empty",
}, {
	info:   "gce-default-field is inserted if missing",
	expect: testing.Attrs{"gce-default-field": "<specific default value>"},
}, {
	info:   "gce-default-field cannot be empty",
	insert: testing.Attrs{"gce-default-field": ""},
	err:    "gce-default-field: must not be empty",
}, {
	info:   "gce-default-field is untouched if present",
	insert: testing.Attrs{"gce-default-field": "<user value>"},
	expect: testing.Attrs{"gce-default-field": "<user value>"},
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
		validatedConfig, err := gce.Provider.Validate(testConfig, nil)
		if test.err == "" {
			c.Check(err, gc.IsNil)
			attrs := validatedConfig.AllAttrs()
			for field, value := range test.expect {
				c.Check(attrs[field], gc.Equals, value)
			}
		} else {
			c.Check(validatedConfig, gc.IsNil)
			c.Check(err, gc.ErrorMatches, "invalid config: "+test.err)
		}
	}
}

func (*ConfigSuite) TestValidateOldConfig(c *gc.C) {
	knownGoodConfig := newConfig(c, validAttrs())
	for i, test := range newConfigTests {
		c.Logf("test %d: %s", i, test.info)
		attrs := validAttrs().Merge(test.insert).Delete(test.remove...)
		testConfig := newConfig(c, attrs)
		validatedConfig, err := gce.Provider.Validate(knownGoodConfig, testConfig)
		if test.err == "" {
			c.Check(err, gc.IsNil)
			attrs := validatedConfig.AllAttrs()
			for field, value := range validAttrs() {
				c.Check(attrs[field], gc.Equals, value)
			}
		} else {
			c.Check(validatedConfig, gc.IsNil)
			c.Check(err, gc.ErrorMatches, "invalid base config: "+test.err)
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
	info:   "can change gce-secret-field",
	insert: testing.Attrs{"gce-secret-field": "okkult"},
	expect: testing.Attrs{"gce-secret-field": "okkult"},
}, {
	info:   "can change gce-default-field",
	insert: testing.Attrs{"gce-default-field": "different"},
	expect: testing.Attrs{"gce-default-field": "different"},
}, {
	info:   "cannot change gce-immutable-field",
	insert: testing.Attrs{"gce-immutable-field": "mutant"},
	err:    "gce-immutable-field: cannot change from static to mutant",
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
		validatedConfig, err := gce.Provider.Validate(testConfig, baseConfig)
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
