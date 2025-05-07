// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package gce_test

import (
	"context"

	"github.com/juju/tc"

	"github.com/juju/juju/environs"
	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/internal/provider/gce"
	"github.com/juju/juju/internal/testing"
)

type ConfigSuite struct {
	gce.BaseSuite

	config *config.Config
}

var _ = tc.Suite(&ConfigSuite{})

func (s *ConfigSuite) SetUpTest(c *tc.C) {
	s.BaseSuite.SetUpTest(c)

	cfg, err := testing.ModelConfig(c).Apply(gce.ConfigAttrs)
	c.Assert(err, tc.ErrorIsNil)
	s.config = cfg
}

// TODO(ericsnow) Each test only deals with a single field, so having
// multiple values in insert and remove (in configTestSpec) is a little
// misleading and unnecessary.

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

func (ts configTestSpec) checkSuccess(c *tc.C, value interface{}, err error) {
	if !c.Check(err, tc.ErrorIsNil) {
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
		c.Check(attrs[field], tc.Equals, value)
	}
}

func (ts configTestSpec) checkFailure(c *tc.C, err error, msg string) {
	c.Check(err, tc.ErrorMatches, msg+": "+ts.err)
}

func (ts configTestSpec) checkAttrs(c *tc.C, attrs map[string]interface{}, cfg *config.Config) {
	for field, expected := range cfg.UnknownAttrs() {
		value := attrs[field]
		c.Check(value, tc.Equals, expected)
	}
}

func (ts configTestSpec) attrs() testing.Attrs {
	return gce.ConfigAttrs.Merge(ts.insert).Delete(ts.remove...)
}

func (ts configTestSpec) newConfig(c *tc.C) *config.Config {
	attrs := ts.attrs()
	cfg, err := testing.ModelConfig(c).Apply(attrs)
	c.Assert(err, tc.ErrorIsNil)
	return cfg
}

var newConfigTests = []configTestSpec{{
	info:   "unknown field is not touched",
	insert: testing.Attrs{"unknown-field": 12345},
	expect: testing.Attrs{"unknown-field": 12345},
}}

func (s *ConfigSuite) TestNewModelConfig(c *tc.C) {
	for i, test := range newConfigTests {
		c.Logf("test %d: %s", i, test.info)

		testConfig := test.newConfig(c)
		environ, err := environs.New(context.Background(), environs.OpenParams{
			Cloud:  gce.MakeTestCloudSpec(),
			Config: testConfig,
		}, environs.NoopCredentialInvalidator())

		// Check the result
		if test.err != "" {
			test.checkFailure(c, err, "invalid config")
		} else {
			test.checkSuccess(c, environ, err)
		}
	}
}

// TODO(wwitzel3) refactor to provider_test file
func (s *ConfigSuite) TestValidateNewConfig(c *tc.C) {
	for i, test := range newConfigTests {
		c.Logf("test %d: %s", i, test.info)

		testConfig := test.newConfig(c)
		validatedConfig, err := gce.Provider.Validate(context.Background(), testConfig, nil)

		// Check the result
		if test.err != "" {
			test.checkFailure(c, err, "invalid config")
		} else {
			c.Check(validatedConfig, tc.NotNil)
			test.checkSuccess(c, validatedConfig, err)
		}
	}
}

// TODO(wwitzel3) refactor to the provider_test file
func (s *ConfigSuite) TestValidateOldConfig(c *tc.C) {
	for i, test := range newConfigTests {
		c.Logf("test %d: %s", i, test.info)

		// Validate the new config (relative to the old one) using the
		// provider.
		_, err := gce.Provider.Validate(context.Background(), s.config, test.newConfig(c))
		if test.err != "" {
			// In this test, we case only about the validating
			// the old configuration, not about the success cases.
			test.checkFailure(c, err, "invalid config: invalid base config")
		}
	}
}

var changeConfigTests = []configTestSpec{{
	info:   "no change, no error",
	expect: gce.ConfigAttrs,
}, {
	info:   "can insert unknown field",
	insert: testing.Attrs{"unknown": "ignoti"},
	expect: testing.Attrs{"unknown": "ignoti"},
}}

// TODO(wwitzel3) refactor this to the provider_test file.
func (s *ConfigSuite) TestValidateChange(c *tc.C) {
	for i, test := range changeConfigTests {
		c.Logf("test %d: %s", i, test.info)

		testConfig := test.newConfig(c)
		validatedConfig, err := gce.Provider.Validate(context.Background(), testConfig, s.config)

		// Check the result.
		if test.err != "" {
			test.checkFailure(c, err, "invalid config")
		} else {
			test.checkSuccess(c, validatedConfig, err)
		}
	}
}

func (s *ConfigSuite) TestSetConfig(c *tc.C) {
	for i, test := range changeConfigTests {
		c.Logf("test %d: %s", i, test.info)

		environ, err := environs.New(context.Background(), environs.OpenParams{
			Cloud:  gce.MakeTestCloudSpec(),
			Config: s.config,
		}, environs.NoopCredentialInvalidator())
		c.Assert(err, tc.ErrorIsNil)

		testConfig := test.newConfig(c)
		err = environ.SetConfig(context.Background(), testConfig)

		// Check the result.
		if test.err != "" {
			test.checkFailure(c, err, "invalid config change")
			test.checkAttrs(c, environ.Config().AllAttrs(), s.config)
		} else {
			test.checkSuccess(c, environ.Config(), err)
		}
	}
}

func (*ConfigSuite) TestSchema(c *tc.C) {
	fields := gce.Provider.(environs.ProviderSchema).Schema()
	// Check that all the fields defined in environs/config
	// are in the returned schema.
	globalFields, err := config.Schema(nil)
	c.Assert(err, tc.IsNil)
	for name, field := range globalFields {
		c.Check(fields[name], tc.DeepEquals, field)
	}
}
