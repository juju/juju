// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package vsphere_test

import (
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/cloud"
	"github.com/juju/juju/environs"
	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/provider/vsphere"
	"github.com/juju/juju/testing"
)

func fakeConfig(c *gc.C, attrs ...testing.Attrs) *config.Config {
	cfg, err := testing.ModelConfig(c).Apply(fakeConfigAttrs(attrs...))
	c.Assert(err, jc.ErrorIsNil)
	return cfg
}

func fakeConfigAttrs(attrs ...testing.Attrs) testing.Attrs {
	merged := testing.FakeConfig().Merge(testing.Attrs{
		"type":             "vsphere",
		"uuid":             "2d02eeac-9dbb-11e4-89d3-123b93f75cba",
		"external-network": "",
	})
	for _, attrs := range attrs {
		merged = merged.Merge(attrs)
	}
	return merged
}

func fakeCloudSpec() environs.CloudSpec {
	cred := fakeCredential()
	return environs.CloudSpec{
		Type:       "vsphere",
		Name:       "vsphere",
		Region:     "/datacenter1",
		Endpoint:   "host1",
		Credential: &cred,
	}
}

func fakeCredential() cloud.Credential {
	return cloud.NewCredential(cloud.UserPassAuthType, map[string]string{
		"user":     "user1",
		"password": "password1",
	})
}

type ConfigSuite struct {
	testing.BaseSuite
	config   *config.Config
	provider environs.EnvironProvider
}

var _ = gc.Suite(&ConfigSuite{})

func (s *ConfigSuite) SetUpTest(c *gc.C) {
	s.BaseSuite.SetUpTest(c)
	s.config = fakeConfig(c)
	s.provider = vsphere.NewEnvironProvider(vsphere.EnvironProviderConfig{})
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
	return fakeConfigAttrs().Merge(ts.insert).Delete(ts.remove...)
}

func (ts configTestSpec) newConfig(c *gc.C) *config.Config {
	attrs := ts.attrs()
	cfg, err := testing.ModelConfig(c).Apply(attrs)
	c.Assert(err, jc.ErrorIsNil)
	return cfg
}

var newConfigTests = []configTestSpec{{
	info:   "unknown field is not touched",
	insert: testing.Attrs{"unknown-field": "12345"},
	expect: testing.Attrs{"unknown-field": "12345"},
}}

func (*ConfigSuite) TestNewModelConfig(c *gc.C) {
	for i, test := range newConfigTests {
		c.Logf("test %d: %s", i, test.info)

		fakeConfig := test.newConfig(c)
		environ, err := environs.New(environs.OpenParams{
			Cloud:  fakeCloudSpec(),
			Config: fakeConfig,
		})

		// Check the result
		if test.err != "" {
			test.checkFailure(c, err, "invalid config")
		} else {
			test.checkSuccess(c, environ, err)
		}
	}
}

func (s *ConfigSuite) TestValidateNewConfig(c *gc.C) {
	for i, test := range newConfigTests {
		c.Logf("test %d: %s", i, test.info)

		fakeConfig := test.newConfig(c)
		validatedConfig, err := s.provider.Validate(fakeConfig, nil)

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
		expected := fakeConfigAttrs()

		// Validate the new config (relative to the old one) using the
		// provider.
		validatedConfig, err := s.provider.Validate(newcfg, oldcfg)

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
	expect: fakeConfigAttrs(),
}, {
	info:   "can insert unknown field",
	insert: testing.Attrs{"unknown": "ignoti"},
	expect: testing.Attrs{"unknown": "ignoti"},
}}

func (s *ConfigSuite) TestValidateChange(c *gc.C) {
	for i, test := range changeConfigTests {
		c.Logf("test %d: %s", i, test.info)

		fakeConfig := test.newConfig(c)
		validatedConfig, err := s.provider.Validate(fakeConfig, s.config)

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

		environ, err := environs.New(environs.OpenParams{
			Cloud:  fakeCloudSpec(),
			Config: s.config,
		})
		c.Assert(err, jc.ErrorIsNil)

		fakeConfig := test.newConfig(c)
		err = environ.SetConfig(fakeConfig)

		// Check the result.
		if test.err != "" {
			test.checkFailure(c, err, "invalid config change")
			test.checkAttrs(c, environ.Config().AllAttrs(), s.config)
		} else {
			test.checkSuccess(c, environ.Config(), err)
		}
	}
}
