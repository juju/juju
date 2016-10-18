// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package cloudsigma

import (
	gc "gopkg.in/check.v1"

	"github.com/altoros/gosigma/mock"
	"github.com/juju/juju/cloud"
	"github.com/juju/juju/environs"
	"github.com/juju/juju/environs/config"
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
		"type": "cloudsigma",
		"uuid": "f54aac3a-9dcd-4a0c-86b5-24091478478c",
	})
}

func fakeCloudSpec() environs.CloudSpec {
	cred := fakeCredential()
	return environs.CloudSpec{
		Type:       "cloudsigma",
		Name:       "cloudsigma",
		Region:     "testregion",
		Endpoint:   "https://0.1.2.3:2000/api/2.0/",
		Credential: &cred,
	}
}

func fakeCredential() cloud.Credential {
	return cloud.NewCredential(cloud.UserPassAuthType, map[string]string{
		"username": mock.TestUser,
		"password": mock.TestPassword,
	})
}

type configSuite struct {
	testing.BaseSuite
}

func (s *configSuite) SetUpSuite(c *gc.C) {
	s.BaseSuite.SetUpSuite(c)
}

func (s *configSuite) SetUpTest(c *gc.C) {
	s.BaseSuite.SetUpTest(c)
	// speed up tests, do not create heavy stuff inside providers created withing this test suite
	s.PatchValue(&newClient, func(environs.CloudSpec, string) (*environClient, error) {
		return nil, nil
	})
}

var _ = gc.Suite(&configSuite{})

func (s *configSuite) TestNewModelConfig(c *gc.C) {

	type checker struct {
		checker gc.Checker
		value   interface{}
	}

	var newConfigTests = []struct {
		info   string
		insert testing.Attrs
		remove []string
		expect testing.Attrs
		err    string
	}{}

	for i, test := range newConfigTests {
		c.Logf("test %d: %s", i, test.info)
		attrs := validAttrs().Merge(test.insert).Delete(test.remove...)
		testConfig := newConfig(c, attrs)
		environ, err := environs.New(environs.OpenParams{
			Cloud:  fakeCloudSpec(),
			Config: testConfig,
		})
		if test.err == "" {
			c.Check(err, gc.IsNil)
			attrs := environ.Config().AllAttrs()
			for field, value := range test.expect {
				if chk, ok := value.(checker); ok {
					c.Check(attrs[field], chk.checker, chk.value)
				} else {
					c.Check(attrs[field], gc.Equals, value)
				}
			}
		} else {
			c.Check(environ, gc.IsNil)
			c.Check(err, gc.ErrorMatches, test.err)
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
}}

func (s *configSuite) TestValidateChange(c *gc.C) {
	baseConfig := newConfig(c, validAttrs())
	for i, test := range changeConfigTests {
		c.Logf("test %d: %s", i, test.info)
		attrs := validAttrs().Merge(test.insert).Delete(test.remove...)
		testConfig := newConfig(c, attrs)
		validatedConfig, err := providerInstance.Validate(testConfig, baseConfig)
		if test.err == "" {
			c.Check(err, gc.IsNil)
			attrs := validatedConfig.AllAttrs()
			for field, value := range test.expect {
				c.Check(attrs[field], gc.Equals, value)
			}
		} else {
			c.Check(validatedConfig, gc.IsNil)
			c.Check(err, gc.ErrorMatches, "invalid config.*: "+test.err)
		}

		// reverse change
		validatedConfig, err = providerInstance.Validate(baseConfig, testConfig)
		if test.err == "" {
			c.Check(err, gc.IsNil)
			attrs := validatedConfig.AllAttrs()
			for field, value := range validAttrs() {
				c.Check(attrs[field], gc.Equals, value)
			}
		} else {
			c.Check(validatedConfig, gc.IsNil)
			c.Check(err, gc.ErrorMatches, "invalid .*config.*: "+test.err)
		}
	}
}

func (s *configSuite) TestSetConfig(c *gc.C) {
	baseConfig := newConfig(c, validAttrs())
	for i, test := range changeConfigTests {
		c.Logf("test %d: %s", i, test.info)
		environ, err := environs.New(environs.OpenParams{
			Cloud:  fakeCloudSpec(),
			Config: baseConfig,
		})
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

func (s *configSuite) TestModelConfig(c *gc.C) {
	testConfig := newConfig(c, validAttrs())
	ecfg, err := validateConfig(testConfig, nil)
	c.Assert(ecfg, gc.NotNil)
	c.Assert(err, gc.IsNil)
}

func (s *configSuite) TestInvalidConfigChange(c *gc.C) {
	oldAttrs := validAttrs().Merge(testing.Attrs{"name": "123"})
	oldConfig := newConfig(c, oldAttrs)
	newAttrs := validAttrs().Merge(testing.Attrs{"name": "321"})
	newConfig := newConfig(c, newAttrs)

	oldecfg, _ := providerInstance.Validate(oldConfig, nil)
	c.Assert(oldecfg, gc.NotNil)

	newecfg, err := providerInstance.Validate(newConfig, oldecfg)
	c.Assert(newecfg, gc.IsNil)
	c.Assert(err, gc.NotNil)
}
