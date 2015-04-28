// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

// +build !gccgo

package vsphere_test

import (
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/environs"
	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/provider/vsphere"
	"github.com/juju/juju/testing"
)

type ConfigSuite struct {
	vsphere.BaseSuite

	config *config.Config
}

var _ = gc.Suite(&ConfigSuite{})

func (s *ConfigSuite) SetUpTest(c *gc.C) {
	s.BaseSuite.SetUpTest(c)

	cfg, err := testing.EnvironConfig(c).Apply(vsphere.ConfigAttrs)
	c.Assert(err, jc.ErrorIsNil)
	s.config = cfg
}

// configTestSpec defines a subtest to run in a table driven test.
type configTestSpec struct {
	// info describes the subtest.
	info string
	// insert holds attrs that should be merged into the config.
	insert testing.Attrs
	// remove has the names of attrs that should be removed.
	remove []string
	// expect defines the expected attributes in a success case.
	expect testing.Attrs
	// err is the error message to expect in a failure case.
	err string
}

func (ts configTestSpec) checkSuccess(c *gc.C, value interface{}, err error) {
	if !c.Check(err, jc.ErrorIsNil) {
		return
	}

	var cfg *config.Config
	switch typed := value.(type) {
	case *config.Config:
		cfg = typed
	case environs.Environ:
		cfg = typed.Config()
	}

	attrs := cfg.AllAttrs()
	for field, value := range ts.expect {
		c.Check(attrs[field], gc.Equals, value)
	}
}

func (ts configTestSpec) checkFailure(c *gc.C, err error, msg string) {
	c.Check(err, gc.ErrorMatches, msg+": "+ts.err)
}

func (ts configTestSpec) checkAttrs(c *gc.C, attrs map[string]interface{}, cfg *config.Config) {
	for field, value := range cfg.UnknownAttrs() {
		c.Check(attrs[field], gc.Equals, value)
	}
}

func (ts configTestSpec) attrs() testing.Attrs {
	return vsphere.ConfigAttrs.Merge(ts.insert).Delete(ts.remove...)
}

func (ts configTestSpec) newConfig(c *gc.C) *config.Config {
	attrs := ts.attrs()
	cfg, err := testing.EnvironConfig(c).Apply(attrs)
	c.Assert(err, jc.ErrorIsNil)
	return cfg
}

var newConfigTests = []configTestSpec{{
	info:   "datacenter is required",
	remove: []string{"datacenter"},
	err:    "datacenter: expected string, got nothing",
}, {
	info:   "datacenter cannot be empty",
	insert: testing.Attrs{"datacenter": ""},
	err:    "datacenter: must not be empty",
}, {
	info:   "host is required",
	remove: []string{"host"},
	err:    "host: expected string, got nothing",
}, {
	info:   "host cannot be empty",
	insert: testing.Attrs{"host": ""},
	err:    "host: must not be empty",
}, {
	info:   "user is required",
	remove: []string{"user"},
	err:    "user: expected string, got nothing",
}, {
	info:   "user cannot be empty",
	insert: testing.Attrs{"user": ""},
	err:    "user: must not be empty",
}, {
	info:   "password is required",
	remove: []string{"password"},
	err:    "password: expected string, got nothing",
}, {
	info:   "password cannot be empty",
	insert: testing.Attrs{"password": ""},
	err:    "password: must not be empty",
}, {
	info:   "unknown field is not touched",
	insert: testing.Attrs{"unknown-field": "12345"},
	expect: testing.Attrs{"unknown-field": "12345"},
}}

func (*ConfigSuite) TestNewEnvironConfig(c *gc.C) {
	for i, test := range newConfigTests {
		c.Logf("test %d: %s", i, test.info)

		testConfig := test.newConfig(c)
		environ, err := environs.New(testConfig)

		// Check the result
		if test.err != "" {
			test.checkFailure(c, err, "invalid config")
		} else {
			test.checkSuccess(c, environ, err)
		}
	}
}

func (*ConfigSuite) TestValidateNewConfig(c *gc.C) {
	for i, test := range newConfigTests {
		c.Logf("test %d: %s", i, test.info)

		testConfig := test.newConfig(c)
		validatedConfig, err := vsphere.Provider.Validate(testConfig, nil)

		// Check the result
		if test.err != "" {
			test.checkFailure(c, err, "invalid config")
		} else {
			c.Check(validatedConfig, gc.NotNil)
			test.checkSuccess(c, validatedConfig, err)
		}
	}
}

func (s *ConfigSuite) TestValidateOldConfig(c *gc.C) {
	for i, test := range newConfigTests {
		c.Logf("test %d: %s", i, test.info)

		oldcfg := test.newConfig(c)
		newcfg := s.config
		expected := vsphere.ConfigAttrs

		// Validate the new config (relative to the old one) using the
		// provider.
		validatedConfig, err := vsphere.Provider.Validate(newcfg, oldcfg)

		// Check the result.
		if test.err != "" {
			test.checkFailure(c, err, "invalid base config")
		} else {
			if test.remove != nil {
				// No defaults are set on the old config.
				c.Check(err, gc.ErrorMatches, "invalid base config: .*")
				continue
			}

			c.Check(err, jc.ErrorIsNil)
			// We verify that Validate filled in the defaults
			// appropriately.
			c.Check(validatedConfig, gc.NotNil)
			test.checkAttrs(c, expected, validatedConfig)
		}
	}
}

var changeConfigTests = []configTestSpec{{
	info:   "no change, no error",
	expect: vsphere.ConfigAttrs,
}, {
	info:   "cannot change datacenter",
	insert: testing.Attrs{"datacenter": "/datacenter2"},
	err:    "datacenter: cannot change from /datacenter1 to /datacenter2",
}, {
	info:   "cannot change host",
	insert: testing.Attrs{"host": "host2"},
	err:    "host: cannot change from host1 to host2",
}, {
	info:   "cannot change user",
	insert: testing.Attrs{"user": "user2"},
	expect: testing.Attrs{"user": "user2"},
}, {
	info:   "cannot change password",
	insert: testing.Attrs{"password": "password2"},
	expect: testing.Attrs{"password": "password2"},
}, {
	info:   "can insert unknown field",
	insert: testing.Attrs{"unknown": "ignoti"},
	expect: testing.Attrs{"unknown": "ignoti"},
}}

func (s *ConfigSuite) TestValidateChange(c *gc.C) {
	for i, test := range changeConfigTests {
		c.Logf("test %d: %s", i, test.info)

		testConfig := test.newConfig(c)
		validatedConfig, err := vsphere.Provider.Validate(testConfig, s.config)

		// Check the result.
		if test.err != "" {
			test.checkFailure(c, err, "invalid config change")
		} else {
			test.checkSuccess(c, validatedConfig, err)
		}
	}
}

func (s *ConfigSuite) TestSetConfig(c *gc.C) {
	for i, test := range changeConfigTests {
		c.Logf("test %d: %s", i, test.info)

		environ, err := environs.New(s.config)
		c.Assert(err, jc.ErrorIsNil)

		testConfig := test.newConfig(c)
		err = environ.SetConfig(testConfig)

		// Check the result.
		if test.err != "" {
			test.checkFailure(c, err, "invalid config change")
			test.checkAttrs(c, environ.Config().AllAttrs(), s.config)
		} else {
			test.checkSuccess(c, environ.Config(), err)
		}
	}
}
