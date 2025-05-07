// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package vsphere_test

import (
	"context"

	"github.com/juju/tc"
	jc "github.com/juju/testing/checkers"

	"github.com/juju/juju/cloud"
	"github.com/juju/juju/environs"
	environscloudspec "github.com/juju/juju/environs/cloudspec"
	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/internal/provider/vsphere"
	"github.com/juju/juju/internal/testing"
)

func fakeConfig(c *tc.C, attrs ...testing.Attrs) *config.Config {
	cfg, err := testing.ModelConfig(c).Apply(fakeConfigAttrs(attrs...))
	c.Assert(err, jc.ErrorIsNil)
	return cfg
}

func fakeConfigAttrs(attrs ...testing.Attrs) testing.Attrs {
	merged := testing.FakeConfig().Merge(testing.Attrs{
		"type":                      "vsphere",
		"uuid":                      "2d02eeac-9dbb-11e4-89d3-123b93f75cba",
		"external-network":          "",
		"enable-disk-uuid":          true,
		"force-vm-hardware-version": 0,
		"disk-provisioning-type":    "",
	})
	for _, attrs := range attrs {
		merged = merged.Merge(attrs)
	}
	return merged
}

func fakeCloudSpec() environscloudspec.CloudSpec {
	cred := fakeCredential()
	return environscloudspec.CloudSpec{
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

var _ = tc.Suite(&ConfigSuite{})

func (s *ConfigSuite) SetUpTest(c *tc.C) {
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

func (ts configTestSpec) checkSuccess(c *tc.C, value interface{}, err error) {
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
		c.Check(attrs[field], tc.Equals, value)
	}
}

func (ts configTestSpec) checkFailure(c *tc.C, err error, msg string) {
	c.Check(err, tc.ErrorMatches, msg+": "+ts.err)
}

func (ts configTestSpec) checkAttrs(c *tc.C, attrs map[string]interface{}, cfg *config.Config) {
	for field, value := range cfg.UnknownAttrs() {
		c.Check(attrs[field], tc.Equals, value)
	}
}

func (ts configTestSpec) attrs() testing.Attrs {
	return fakeConfigAttrs().Merge(ts.insert).Delete(ts.remove...)
}

func (ts configTestSpec) newConfig(c *tc.C) *config.Config {
	attrs := ts.attrs()
	cfg, err := testing.ModelConfig(c).Apply(attrs)
	c.Assert(err, jc.ErrorIsNil)
	return cfg
}

var newConfigTests = []configTestSpec{
	{
		info:   "unknown field is not touched",
		insert: testing.Attrs{"unknown-field": "12345"},
		expect: testing.Attrs{"unknown-field": "12345"},
	},
	{
		info:   "use thick disk provisioning",
		insert: testing.Attrs{"disk-provisioning-type": "thick"},
		expect: testing.Attrs{"disk-provisioning-type": "thick"},
	},
	{
		info:   "set invalid disk provisioning",
		insert: testing.Attrs{"disk-provisioning-type": "eroneous"},
		err:    "\"disk-provisioning-type\" must be one of.*",
	},
}

func (*ConfigSuite) TestNewModelConfig(c *tc.C) {
	for i, test := range newConfigTests {
		c.Logf("test %d: %s", i, test.info)

		fakeConfig := test.newConfig(c)
		environ, err := environs.New(context.Background(), environs.OpenParams{
			Cloud:  fakeCloudSpec(),
			Config: fakeConfig,
		}, environs.NoopCredentialInvalidator())

		// Check the result
		if test.err != "" {
			test.checkFailure(c, err, "invalid config")
		} else {
			test.checkSuccess(c, environ, err)
		}
	}
}

func (s *ConfigSuite) TestValidateNewConfig(c *tc.C) {
	for i, test := range newConfigTests {
		c.Logf("test %d: %s", i, test.info)

		fakeConfig := test.newConfig(c)
		validatedConfig, err := s.provider.Validate(context.Background(), fakeConfig, nil)

		// Check the result
		if test.err != "" {
			test.checkFailure(c, err, "invalid config")
		} else {
			c.Check(validatedConfig, tc.NotNil)
			test.checkSuccess(c, validatedConfig, err)
		}
	}
}

func (s *ConfigSuite) TestValidateOldConfig(c *tc.C) {
	for i, test := range newConfigTests {
		c.Logf("test %d: %s", i, test.info)

		oldcfg := test.newConfig(c)
		newcfg := s.config
		expected := fakeConfigAttrs()

		// Validate the new config (relative to the old one) using the
		// provider.
		validatedConfig, err := s.provider.Validate(context.Background(), newcfg, oldcfg)

		// Check the result.
		if test.err != "" {
			test.checkFailure(c, err, "invalid base config")
		} else {
			if test.remove != nil {
				// No defaults are set on the old config.
				c.Check(err, tc.ErrorMatches, "invalid base config: .*")
				continue
			}

			c.Assert(err, jc.ErrorIsNil)
			// We verify that Validate filled in the defaults
			// appropriately.
			c.Check(validatedConfig, tc.NotNil)
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

func (s *ConfigSuite) TestValidateChange(c *tc.C) {
	for i, test := range changeConfigTests {
		c.Logf("test %d: %s", i, test.info)

		fakeConfig := test.newConfig(c)
		validatedConfig, err := s.provider.Validate(context.Background(), fakeConfig, s.config)

		// Check the result.
		if test.err != "" {
			test.checkFailure(c, err, "invalid config change")
		} else {
			test.checkSuccess(c, validatedConfig, err)
		}
	}
}

func (s *ConfigSuite) TestSetConfig(c *tc.C) {
	for i, test := range changeConfigTests {
		c.Logf("test %d: %s", i, test.info)

		environ, err := environs.New(context.Background(), environs.OpenParams{
			Cloud:  fakeCloudSpec(),
			Config: s.config,
		}, environs.NoopCredentialInvalidator())
		c.Assert(err, jc.ErrorIsNil)

		fakeConfig := test.newConfig(c)
		err = environ.SetConfig(context.Background(), fakeConfig)

		// Check the result.
		if test.err != "" {
			test.checkFailure(c, err, "invalid config change")
			test.checkAttrs(c, environ.Config().AllAttrs(), s.config)
		} else {
			test.checkSuccess(c, environ.Config(), err)
		}
	}
}

func (s *ConfigSuite) TestSchema(c *tc.C) {
	ps, ok := s.provider.(environs.ProviderSchema)
	c.Assert(ok, jc.IsTrue)

	fields := ps.Schema()

	globalFields, err := config.Schema(nil)
	c.Assert(err, tc.IsNil)
	for name, field := range globalFields {
		c.Check(fields[name], jc.DeepEquals, field)
	}
}
