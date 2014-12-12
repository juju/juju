// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package gce_test

import (
	gitjujutesting "github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/environs"
	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/provider/gce"
	"github.com/juju/juju/testing"
)

func newConfig(c *gc.C, attrs testing.Attrs) *config.Config {
	attrs = testing.FakeConfig().Merge(attrs)
	cfg, err := config.New(config.NoDefaults, attrs)
	c.Assert(err, jc.ErrorIsNil)
	return cfg
}

func validAttrs() testing.Attrs {
	return testing.FakeConfig().Merge(testing.Attrs{
		"type":         "gce",
		"private-key":  "seekrit",
		"client-id":    "static",
		"client-email": "joe@mail.com",
		"region":       "home",
		"project-id":   "my-juju",
	})
}

type ConfigSuite struct {
	gitjujutesting.IsolationSuite
}

var _ = gc.Suite(&ConfigSuite{})

func (s *ConfigSuite) SetUpTest(c *gc.C) {
	s.IsolationSuite.SetUpTest(c)

	s.PatchValue(gce.NewToken, gce.DummyNewToken)
	s.PatchValue(gce.NewService, gce.DummyNewService)
}

var newConfigTests = []struct {
	info   string
	insert testing.Attrs
	remove []string
	expect testing.Attrs
	err    string
}{{
	info:   "client-id is required",
	remove: []string{"client-id"},
	err:    "client-id: expected string, got nothing",
}, {
	info:   "client-id cannot be empty",
	insert: testing.Attrs{"client-id": ""},
	err:    "client-id: must not be empty",
}, {
	info:   "private-key is required",
	remove: []string{"private-key"},
	err:    "private-key: expected string, got nothing",
}, {
	info:   "private-key cannot be empty",
	insert: testing.Attrs{"private-key": ""},
	err:    "private-key: must not be empty",
}, {
	info:   "client-email is required",
	remove: []string{"client-email"},
	err:    "client-email: expected string, got nothing",
}, {
	info:   "client-email cannot be empty",
	insert: testing.Attrs{"client-email": ""},
	err:    "client-email: must not be empty",
}, {
	info:   "region is required",
	remove: []string{"region"},
	err:    "region: expected string, got nothing",
}, {
	info:   "region cannot be empty",
	insert: testing.Attrs{"region": ""},
	err:    "region: must not be empty",
}, {
	info:   "project-id is required",
	remove: []string{"project-id"},
	err:    "project-id: expected string, got nothing",
}, {
	info:   "project-id cannot be empty",
	insert: testing.Attrs{"project-id": ""},
	err:    "project-id: must not be empty",
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
			if !c.Check(err, jc.ErrorIsNil) {
				continue
			}
			attrs := environ.Config().AllAttrs()
			for field, value := range test.expect {
				c.Check(attrs[field], gc.Equals, value)
			}
		} else {
			if v, ok := attrs["private-key"].(string); ok && v == "" {
				//panic("here")
			}
			if err != nil {
				c.Check(environ, gc.IsNil)
			}
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
			if !c.Check(err, jc.ErrorIsNil) {
				continue
			}
			attrs := validatedConfig.AllAttrs()
			for field, value := range test.expect {
				c.Check(attrs[field], gc.Equals, value)
			}
		} else {
			if err != nil {
				c.Check(validatedConfig, gc.IsNil)
			}
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
			if !c.Check(err, jc.ErrorIsNil) {
				continue
			}
			attrs := validatedConfig.AllAttrs()
			for field, value := range validAttrs() {
				c.Check(attrs[field], gc.Equals, value)
			}
		} else {
			if err != nil {
				c.Check(validatedConfig, gc.IsNil)
			}
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
	info:   "cannot change private-key",
	insert: testing.Attrs{"private-key": "okkult"},
	err:    "private-key: cannot change from seekrit to okkult",
}, {
	info:   "cannot change client-id",
	insert: testing.Attrs{"client-id": "mutant"},
	err:    "client-id: cannot change from static to mutant",
}, {
	info:   "cannot change client-email",
	insert: testing.Attrs{"client-email": "spam@eggs.com"},
	err:    "client-email: cannot change from joe@mail.com to spam@eggs.com",
}, {
	info:   "cannot change region",
	insert: testing.Attrs{"region": "not home"},
	err:    "region: cannot change from home to not home",
}, {
	info:   "cannot change project-id",
	insert: testing.Attrs{"project-id": "your-juju"},
	err:    "project-id: cannot change from my-juju to your-juju",
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
			if !c.Check(err, jc.ErrorIsNil) {
				continue
			}
			attrs := validatedConfig.AllAttrs()
			for field, value := range test.expect {
				c.Check(attrs[field], gc.Equals, value)
			}
		} else {
			if err != nil {
				c.Check(validatedConfig, gc.IsNil)
			}
			c.Check(err, gc.ErrorMatches, "invalid config change: "+test.err)
		}
	}
}

func (s *ConfigSuite) TestSetConfig(c *gc.C) {
	baseConfig := newConfig(c, validAttrs())
	for i, test := range changeConfigTests {
		c.Logf("test %d: %s", i, test.info)
		environ, err := environs.New(baseConfig)
		c.Assert(err, jc.ErrorIsNil)
		attrs := validAttrs().Merge(test.insert).Delete(test.remove...)
		testConfig := newConfig(c, attrs)
		err = environ.SetConfig(testConfig)
		newAttrs := environ.Config().AllAttrs()
		if test.err == "" {
			if !c.Check(err, jc.ErrorIsNil) {
				continue
			}
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
