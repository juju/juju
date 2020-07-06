// Copyright 2013 Joyent Inc.
// Licensed under the AGPLv3, see LICENCE file for details.

package joyent_test

import (
	"os"

	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/cloud"
	"github.com/juju/juju/environs"
	environscloudspec "github.com/juju/juju/environs/cloudspec"
	"github.com/juju/juju/environs/config"
	jp "github.com/juju/juju/provider/joyent"
	coretesting "github.com/juju/juju/testing"
)

func newConfig(c *gc.C, attrs coretesting.Attrs) *config.Config {
	attrs = coretesting.FakeConfig().Merge(attrs)
	cfg, err := config.New(config.UseDefaults, attrs)
	c.Assert(err, jc.ErrorIsNil)
	return cfg
}

func validAttrs() coretesting.Attrs {
	return coretesting.FakeConfig().Merge(coretesting.Attrs{
		"type": "joyent",
	})
}

func fakeCloudSpec() environscloudspec.CloudSpec {
	cred := fakeCredential()
	return environscloudspec.CloudSpec{
		Type:       "joyent",
		Name:       "joyent",
		Region:     "whatever",
		Endpoint:   "test://test.api.joyentcloud.com",
		Credential: &cred,
	}
}

func fakeCredential() cloud.Credential {
	return cloud.NewCredential(cloud.UserPassAuthType, map[string]string{
		"sdc-user":    "test",
		"sdc-key-id":  "00:11:22:33:44:55:66:77:88:99:aa:bb:cc:dd:ee:ff",
		"private-key": testPrivateKey,
		"algorithm":   "rsa-sha256",
	})
}

type ConfigSuite struct {
	coretesting.FakeJujuXDGDataHomeSuite
}

var _ = gc.Suite(&ConfigSuite{})

func (s *ConfigSuite) SetUpSuite(c *gc.C) {
	s.FakeJujuXDGDataHomeSuite.SetUpSuite(c)
	jp.RegisterMachinesEndpoint()
	s.AddCleanup(func(*gc.C) { jp.UnregisterMachinesEndpoint() })
}

type configtest struct {
	info    string
	insert  coretesting.Attrs
	remove  []string
	envVars map[string]string
	expect  coretesting.Attrs
	err     string
}

var newConfigTests = []configtest{{
	info:   "unknown field is not touched",
	insert: coretesting.Attrs{"unknown-field": 12345},
	expect: coretesting.Attrs{"unknown-field": 12345},
}}

func (s *ConfigSuite) TestNewModelConfig(c *gc.C) {
	for i, test := range newConfigTests {
		doTest(s, i, test, c)
	}
}

func doTest(s *ConfigSuite, i int, test configtest, c *gc.C) {
	c.Logf("test %d: %s", i, test.info)
	for k, v := range test.envVars {
		os.Setenv(k, v)
		defer os.Setenv(k, "")
	}
	attrs := validAttrs().Merge(test.insert).Delete(test.remove...)
	testConfig := newConfig(c, attrs)
	environ, err := environs.New(environs.OpenParams{
		Cloud:  fakeCloudSpec(),
		Config: testConfig,
	})
	if test.err == "" {
		c.Check(err, jc.ErrorIsNil)
		if err != nil {
			return
		}
		attrs := environ.Config().AllAttrs()
		for field, value := range test.expect {
			c.Check(attrs[field], gc.Equals, value)
		}
	} else {
		c.Check(environ, gc.IsNil)
		c.Check(err, gc.ErrorMatches, test.err)
	}
}

var changeConfigTests = []struct {
	info   string
	insert coretesting.Attrs
	remove []string
	expect coretesting.Attrs
	err    string
}{{
	info:   "no change, no error",
	expect: validAttrs(),
}, {
	info:   "can insert unknown field",
	insert: coretesting.Attrs{"unknown": "ignoti"},
	expect: coretesting.Attrs{"unknown": "ignoti"},
}}

func (s *ConfigSuite) TestValidateChange(c *gc.C) {
	attrs := validAttrs()
	baseConfig := newConfig(c, attrs)
	for i, test := range changeConfigTests {
		c.Logf("test %d: %s", i, test.info)
		attrs := validAttrs().Merge(test.insert).Delete(test.remove...)
		testConfig := newConfig(c, attrs)
		validatedConfig, err := jp.Provider.Validate(testConfig, baseConfig)
		if test.err == "" {
			c.Check(err, jc.ErrorIsNil)
			if err != nil {
				continue
			}
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
		environ, err := environs.New(environs.OpenParams{
			Cloud:  fakeCloudSpec(),
			Config: baseConfig,
		})
		c.Assert(err, jc.ErrorIsNil)
		attrs := validAttrs().Merge(test.insert).Delete(test.remove...)
		testConfig := newConfig(c, attrs)
		err = environ.SetConfig(testConfig)
		newAttrs := environ.Config().AllAttrs()
		if test.err == "" {
			c.Check(err, jc.ErrorIsNil)
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

// TODO(wallyworld) - add tests for cloud endpoint passed in via bootstrap args
var bootstrapConfigTests = []struct {
	info   string
	insert coretesting.Attrs
	remove []string
	expect coretesting.Attrs
	err    string
}{{
	info:   "All value provided, nothing to do",
	expect: validAttrs(),
}}

func (s *ConfigSuite) TestPrepareConfig(c *gc.C) {
	for i, test := range bootstrapConfigTests {
		c.Logf("test %d: %s", i, test.info)
		attrs := validAttrs().Merge(test.insert).Delete(test.remove...)
		testConfig := newConfig(c, attrs)
		preparedConfig, err := jp.Provider.PrepareConfig(environs.PrepareConfigParams{
			Config: testConfig,
			Cloud:  fakeCloudSpec(),
		})
		if test.err == "" {
			c.Check(err, jc.ErrorIsNil)
			attrs := preparedConfig.AllAttrs()
			for field, value := range test.expect {
				c.Check(attrs[field], gc.Equals, value)
			}
		} else {
			c.Check(preparedConfig, gc.IsNil)
			c.Check(err, gc.ErrorMatches, test.err)
		}
	}
}
