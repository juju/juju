// Copyright 2013 Joyent Inc.
// Licensed under the AGPLv3, see LICENCE file for details.

package joyent_test

import (
	"fmt"
	"os"

	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/utils/ssh"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/cloud"
	"github.com/juju/juju/environs"
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
		"type":        "joyent",
		"sdc-user":    "test",
		"sdc-key-id":  "00:11:22:33:44:55:66:77:88:99:aa:bb:cc:dd:ee:ff",
		"sdc-url":     "test://test.api.joyentcloud.com",
		"private-key": testPrivateKey,
		"algorithm":   "rsa-sha256",
	})
}

type ConfigSuite struct {
	coretesting.FakeJujuXDGDataHomeSuite
	originalValues map[string]testing.Restorer
	privateKeyData string
}

var _ = gc.Suite(&ConfigSuite{})

func (s *ConfigSuite) SetUpSuite(c *gc.C) {
	s.FakeJujuXDGDataHomeSuite.SetUpSuite(c)
	restoreSdcAccount := testing.PatchEnvironment(jp.SdcAccount, "tester")
	s.AddCleanup(func(*gc.C) { restoreSdcAccount() })
	restoreSdcKeyId := testing.PatchEnvironment(jp.SdcKeyId, "ff:ee:dd:cc:bb:aa:99:88:77:66:55:44:33:22:11:00")
	s.AddCleanup(func(*gc.C) { restoreSdcKeyId() })
	s.privateKeyData = generatePrivateKey(c)
	jp.RegisterMachinesEndpoint()
	s.AddCleanup(func(*gc.C) { jp.UnregisterMachinesEndpoint() })
}

func generatePrivateKey(c *gc.C) string {
	oldBits := ssh.KeyBits
	defer func() {
		ssh.KeyBits = oldBits
	}()
	ssh.KeyBits = 32
	private, _, err := ssh.GenerateKey("test-client")
	c.Assert(err, jc.ErrorIsNil)
	return private
}

func (s *ConfigSuite) SetUpTest(c *gc.C) {
	s.FakeJujuXDGDataHomeSuite.SetUpTest(c)
	for _, envVar := range jp.EnvironmentVariables {
		s.PatchEnvironment(envVar, "")
	}
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
	info:   "sdc-user is required",
	remove: []string{"sdc-user"},
	err:    ".* cannot get sdc-user value from model variable .*",
}, {
	info:   "sdc-user cannot be empty",
	insert: coretesting.Attrs{"sdc-user": ""},
	err:    ".* cannot get sdc-user value from model variable .*",
}, {
	info:   "can get sdc-user from model variable",
	insert: coretesting.Attrs{"sdc-user": ""},
	expect: coretesting.Attrs{"sdc-user": "tester"},
	envVars: map[string]string{
		"SDC_ACCOUNT": "tester",
	},
}, {
	info:   "can get sdc-user from model variable, missing from config",
	remove: []string{"sdc-user"},
	expect: coretesting.Attrs{"sdc-user": "tester"},
	envVars: map[string]string{
		"SDC_ACCOUNT": "tester",
	},
}, {
	info:   "sdc-key-id is required",
	remove: []string{"sdc-key-id"},
	err:    ".* cannot get sdc-key-id value from model variable .*",
}, {
	info:   "sdc-key-id cannot be empty",
	insert: coretesting.Attrs{"sdc-key-id": ""},
	err:    ".* cannot get sdc-key-id value from model variable .*",
}, {
	info:   "can get sdc-key-id from model variable",
	insert: coretesting.Attrs{"sdc-key-id": ""},
	expect: coretesting.Attrs{"sdc-key-id": "key"},
	envVars: map[string]string{
		"SDC_KEY_ID": "key",
	},
}, {
	info:   "can get sdc-key-id from model variable, missing from config",
	remove: []string{"sdc-key-id"},
	expect: coretesting.Attrs{"sdc-key-id": "key"},
	envVars: map[string]string{
		"SDC_KEY_ID": "key",
	},
}, {
	info:   "sdc-url is inserted if missing",
	expect: coretesting.Attrs{"sdc-url": "test://test.api.joyentcloud.com"},
}, {
	info:   "sdc-url cannot be empty",
	insert: coretesting.Attrs{"sdc-url": ""},
	err:    ".* cannot get sdc-url value from model variable .*",
}, {
	info:   "sdc-url is untouched if present",
	insert: coretesting.Attrs{"sdc-url": "test://test.api.joyentcloud.com"},
	expect: coretesting.Attrs{"sdc-url": "test://test.api.joyentcloud.com"},
}, {
	info:   "algorithm is inserted if missing",
	expect: coretesting.Attrs{"algorithm": "rsa-sha256"},
}, {
	info:   "algorithm cannot be empty",
	insert: coretesting.Attrs{"algorithm": ""},
	err:    ".* algorithm: must not be empty",
}, {
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
	attrs["private-key"] = s.privateKeyData
	testConfig := newConfig(c, attrs)
	environ, err := environs.New(testConfig)
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
	info:   "can change sdc-user",
	insert: coretesting.Attrs{"sdc-user": "joyent_user"},
	expect: coretesting.Attrs{"sdc-user": "joyent_user"},
}, {
	info:   "can change sdc-key-id",
	insert: coretesting.Attrs{"sdc-key-id": "ff:ee:dd:cc:bb:aa:99:88:77:66:55:44:33:22:11:00"},
	expect: coretesting.Attrs{"sdc-key-id": "ff:ee:dd:cc:bb:aa:99:88:77:66:55:44:33:22:11:00"},
}, {
	info:   "can change sdc-url",
	insert: coretesting.Attrs{"sdc-url": "test://test.api.joyentcloud.com"},
	expect: coretesting.Attrs{"sdc-url": "test://test.api.joyentcloud.com"},
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
		environ, err := environs.New(baseConfig)
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

func (s *ConfigSuite) TestBootstrapConfig(c *gc.C) {
	for i, test := range bootstrapConfigTests {
		c.Logf("test %d: %s", i, test.info)
		attrs := validAttrs().Merge(test.insert).Delete(test.remove...)
		credentialAttrs := make(map[string]string, len(attrs))
		for k, v := range attrs.Delete("type") {
			credentialAttrs[k] = fmt.Sprintf("%v", v)
		}
		testConfig := newConfig(c, attrs)
		preparedConfig, err := jp.Provider.BootstrapConfig(environs.BootstrapConfigParams{
			Config: testConfig,
			Credentials: cloud.NewCredential(
				cloud.UserPassAuthType,
				credentialAttrs,
			),
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
