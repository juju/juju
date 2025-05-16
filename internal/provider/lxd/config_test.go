// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package lxd_test

import (
	stdtesting "testing"

	"github.com/juju/tc"

	"github.com/juju/juju/cloud"
	"github.com/juju/juju/environs"
	environscloudspec "github.com/juju/juju/environs/cloudspec"
	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/internal/configschema"
	"github.com/juju/juju/internal/provider/lxd"
	"github.com/juju/juju/internal/testing"
)

type configSuite struct {
	lxd.BaseSuite

	provider environs.EnvironProvider
	config   *config.Config
}

func TestConfigSuite(t *stdtesting.T) { tc.Run(t, &configSuite{}) }
func (s *configSuite) SetUpTest(c *tc.C) {
	s.BaseSuite.SetUpTest(c)

	s.provider = lxd.NewProvider()

	cfg, err := testing.ModelConfig(c).Apply(lxd.ConfigAttrs)
	c.Assert(err, tc.ErrorIsNil)
	s.config = cfg
}

func (s *configSuite) TestDefaults(c *tc.C) {
	cfg := lxd.NewBaseConfig(c)
	ecfg := lxd.NewConfig(cfg)

	values, extras := ecfg.Values(c)
	c.Assert(extras, tc.HasLen, 0)

	c.Check(values, tc.DeepEquals, lxd.ConfigValues{})
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
	attrs := lxd.ConfigAttrs
	return attrs.Merge(ts.insert).Delete(ts.remove...)
}

func (ts configTestSpec) newConfig(c *tc.C) *config.Config {
	attrs := ts.attrs()
	cfg, err := testing.ModelConfig(c).Apply(attrs)
	c.Assert(err, tc.ErrorIsNil)
	return cfg
}

func (ts configTestSpec) fixCfg(c *tc.C, cfg *config.Config) *config.Config {
	fixes := make(map[string]interface{})

	// Set changed values.
	fixes = updateAttrs(fixes, ts.insert)

	newCfg, err := cfg.Apply(fixes)
	c.Assert(err, tc.ErrorIsNil)
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

func (s *configSuite) TestNewModelConfig(c *tc.C) {
	for i, test := range newConfigTests {
		c.Logf("test %d: %s", i, test.info)

		testConfig := test.newConfig(c)
		environ, err := environs.New(c.Context(), environs.OpenParams{
			Cloud:  lxdCloudSpec(),
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

func (s *configSuite) TestValidateNewConfig(c *tc.C) {
	for i, test := range newConfigTests {
		c.Logf("test %d: %s", i, test.info)

		testConfig := test.newConfig(c)
		validatedConfig, err := s.provider.Validate(c.Context(), testConfig, nil)

		// Check the result
		if test.err != "" {
			test.checkFailure(c, err, "invalid config")
		} else {
			c.Check(validatedConfig, tc.NotNil)
			test.checkSuccess(c, validatedConfig, err)
		}
	}
}

func (s *configSuite) TestValidateOldConfig(c *tc.C) {
	for i, test := range newConfigTests {
		c.Logf("test %d: %s", i, test.info)

		oldcfg := test.newConfig(c)
		var err error
		oldcfg, err = s.provider.Validate(c.Context(), oldcfg, nil)
		c.Assert(err, tc.ErrorIsNil)
		newcfg := test.fixCfg(c, s.config)
		expected := updateAttrs(lxd.ConfigAttrs, test.insert)

		// Validate the new config (relative to the old one) using the
		// provider.
		validatedConfig, err := s.provider.Validate(c.Context(), newcfg, oldcfg)

		// Check the result.
		if test.err != "" {
			test.checkFailure(c, err, "invalid base config")
		} else {
			if !c.Check(err, tc.ErrorIsNil) {
				continue
			}
			// We verify that Validate filled in the defaults
			// appropriately.
			c.Check(validatedConfig, tc.NotNil)
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

func (s *configSuite) TestValidateChange(c *tc.C) {
	for i, test := range changeConfigTests {
		c.Logf("test %d: %s", i, test.info)

		testConfig := test.newConfig(c)
		validatedConfig, err := s.provider.Validate(c.Context(), testConfig, s.config)

		// Check the result.
		if test.err != "" {
			test.checkFailure(c, err, "invalid config change")
		} else {
			test.checkSuccess(c, validatedConfig, err)
		}
	}
}

func (s *configSuite) TestSetConfig(c *tc.C) {
	for i, test := range changeConfigTests {
		c.Logf("test %d: %s", i, test.info)

		environ, err := environs.New(c.Context(), environs.OpenParams{
			Cloud:  lxdCloudSpec(),
			Config: s.config,
		}, environs.NoopCredentialInvalidator())
		c.Assert(err, tc.ErrorIsNil)

		testConfig := test.newConfig(c)
		err = environ.SetConfig(c.Context(), testConfig)

		// Check the result.
		if test.err != "" {
			test.checkFailure(c, err, "invalid config change")
			expected, err := s.provider.Validate(c.Context(), s.config, nil)
			c.Assert(err, tc.ErrorIsNil)
			test.checkAttrs(c, environ.Config().AllAttrs(), expected)
		} else {
			test.checkSuccess(c, environ.Config(), err)
		}
	}
}

func (s *configSuite) TestSchema(c *tc.C) {
	fields := s.provider.(interface {
		Schema() configschema.Fields
	}).Schema()
	// Check that all the fields defined in environs/config
	// are in the returned schema.
	globalFields, err := config.Schema(nil)
	c.Assert(err, tc.IsNil)
	for name, field := range globalFields {
		c.Check(fields[name], tc.DeepEquals, field)
	}
}

func lxdCloudSpec() environscloudspec.CloudSpec {
	cred := cloud.NewCredential(cloud.CertificateAuthType, map[string]string{
		"client-cert": "client.crt",
		"client-key":  "client.key",
		"server-cert": "servert.crt",
	})
	return environscloudspec.CloudSpec{
		Type:       "lxd",
		Name:       "localhost",
		Credential: &cred,
	}
}
