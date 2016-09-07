// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

// +build go1.3

package lxd_test

import (
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
	"gopkg.in/juju/environschema.v1"

	"github.com/juju/juju/environs"
	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/provider/lxd"
	"github.com/juju/juju/testing"
)

type configSuite struct {
	lxd.BaseSuite

	config *config.Config
}

var _ = gc.Suite(&configSuite{})

func (s *configSuite) SetUpTest(c *gc.C) {
	s.BaseSuite.SetUpTest(c)

	cfg, err := testing.ModelConfig(c).Apply(lxd.ConfigAttrs)
	c.Assert(err, jc.ErrorIsNil)
	s.config = cfg
}

func (s *configSuite) TestDefaults(c *gc.C) {
	cfg := lxd.NewBaseConfig(c)
	ecfg := lxd.NewConfig(cfg)

	values, extras := ecfg.Values(c)
	c.Assert(extras, gc.HasLen, 0)

	c.Check(values, jc.DeepEquals, lxd.ConfigValues{})
}

// TODO(ericsnow) Each test only deals with a single field, so having
// multiple values in insert and remove (in configTestSpec) is a little
// misleading and unecessary.

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
	for field, expected := range cfg.UnknownAttrs() {
		value := attrs[field]
		c.Check(value, gc.Equals, expected)
	}
}

func (ts configTestSpec) attrs() testing.Attrs {
	attrs := lxd.ConfigAttrs
	return attrs.Merge(ts.insert).Delete(ts.remove...)
}

func (ts configTestSpec) newConfig(c *gc.C) *config.Config {
	attrs := ts.attrs()
	cfg, err := testing.ModelConfig(c).Apply(attrs)
	c.Assert(err, jc.ErrorIsNil)
	return cfg
}

func (ts configTestSpec) fixCfg(c *gc.C, cfg *config.Config) *config.Config {
	fixes := make(map[string]interface{})

	// Set changed values.
	fixes = updateAttrs(fixes, ts.insert)

	newCfg, err := cfg.Apply(fixes)
	c.Assert(err, jc.ErrorIsNil)
	return newCfg
}

func updateAttrs(attrs, updates testing.Attrs) testing.Attrs {
	updated := make(testing.Attrs, len(attrs))
	for k, v := range attrs {
		updated[k] = v
	}
	for k, v := range updates {
		updated[k] = v
	}
	return updated
}

var newConfigTests = []configTestSpec{{
	info:   "unknown field is not touched",
	insert: testing.Attrs{"unknown-field": 12345},
	expect: testing.Attrs{"unknown-field": 12345},
}}

func (s *configSuite) TestNewModelConfig(c *gc.C) {
	// TODO(ericsnow) Move to a functional suite.
	if !s.IsRunningLocally(c) {
		c.Skip("LXD not running locally")
	}

	// TODO(redir): Remove after wily or in yakkety.
	skipIfWily(c)

	for i, test := range newConfigTests {
		c.Logf("test %d: %s", i, test.info)

		testConfig := test.newConfig(c)
		environ, err := environs.New(environs.OpenParams{
			Cloud:  lxdCloudSpec(),
			Config: testConfig,
		})

		// Check the result
		if test.err != "" {
			test.checkFailure(c, err, "invalid config")
		} else {
			test.checkSuccess(c, environ, err)
		}
	}
}

// TODO(wwitzel3) refactor to provider_test file
func (s *configSuite) TestValidateNewConfig(c *gc.C) {
	for i, test := range newConfigTests {
		c.Logf("test %d: %s", i, test.info)

		testConfig := test.newConfig(c)
		validatedConfig, err := lxd.Provider.Validate(testConfig, nil)

		// Check the result
		if test.err != "" {
			test.checkFailure(c, err, "invalid config")
		} else {
			c.Check(validatedConfig, gc.NotNil)
			test.checkSuccess(c, validatedConfig, err)
		}
	}
}

// TODO(wwitzel3) refactor to the provider_test file
func (s *configSuite) TestValidateOldConfig(c *gc.C) {
	for i, test := range newConfigTests {
		c.Logf("test %d: %s", i, test.info)

		oldcfg := test.newConfig(c)
		var err error
		oldcfg, err = lxd.Provider.Validate(oldcfg, nil)
		c.Assert(err, jc.ErrorIsNil)
		newcfg := test.fixCfg(c, s.config)
		expected := updateAttrs(lxd.ConfigAttrs, test.insert)

		// Validate the new config (relative to the old one) using the
		// provider.
		validatedConfig, err := lxd.Provider.Validate(newcfg, oldcfg)

		// Check the result.
		if test.err != "" {
			test.checkFailure(c, err, "invalid base config")
		} else {
			if !c.Check(err, jc.ErrorIsNil) {
				continue
			}
			// We verify that Validate filled in the defaults
			// appropriately.
			c.Check(validatedConfig, gc.NotNil)
			test.checkAttrs(c, expected, validatedConfig)
		}
	}
}

var changeConfigTests = []configTestSpec{{
	info:   "no change, no error",
	expect: lxd.ConfigAttrs,
}, {
	info:   "can insert unknown field",
	insert: testing.Attrs{"unknown": "ignoti"},
	expect: testing.Attrs{"unknown": "ignoti"},
}}

// TODO(wwitzel3) refactor this to the provider_test file.
func (s *configSuite) TestValidateChange(c *gc.C) {
	for i, test := range changeConfigTests {
		c.Logf("test %d: %s", i, test.info)

		testConfig := test.newConfig(c)
		validatedConfig, err := lxd.Provider.Validate(testConfig, s.config)

		// Check the result.
		if test.err != "" {
			test.checkFailure(c, err, "invalid config change")
		} else {
			test.checkSuccess(c, validatedConfig, err)
		}
	}
}

func (s *configSuite) TestSetConfig(c *gc.C) {
	// TODO(ericsnow) Move to a functional suite.
	if !s.IsRunningLocally(c) {
		c.Skip("LXD not running locally")
	}

	// TODO(redir): Remove after wily or in yakkety.
	skipIfWily(c)

	for i, test := range changeConfigTests {
		c.Logf("test %d: %s", i, test.info)

		environ, err := environs.New(environs.OpenParams{
			Cloud:  lxdCloudSpec(),
			Config: s.config,
		})
		c.Assert(err, jc.ErrorIsNil)

		testConfig := test.newConfig(c)
		err = environ.SetConfig(testConfig)

		// Check the result.
		if test.err != "" {
			test.checkFailure(c, err, "invalid config change")
			expected, err := lxd.Provider.Validate(s.config, nil)
			c.Assert(err, jc.ErrorIsNil)
			test.checkAttrs(c, environ.Config().AllAttrs(), expected)
		} else {
			test.checkSuccess(c, environ.Config(), err)
		}
	}
}

func (*configSuite) TestSchema(c *gc.C) {
	fields := lxd.Provider.(interface {
		Schema() environschema.Fields
	}).Schema()
	// Check that all the fields defined in environs/config
	// are in the returned schema.
	globalFields, err := config.Schema(nil)
	c.Assert(err, gc.IsNil)
	for name, field := range globalFields {
		c.Check(fields[name], jc.DeepEquals, field)
	}
}

func lxdCloudSpec() environs.CloudSpec {
	return environs.CloudSpec{
		Type: "lxd",
		Name: "localhost",
	}
}
