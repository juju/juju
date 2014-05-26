// Copyright 2013 Joyent Inc.
// Licensed under the AGPLv3, see LICENCE file for details.

package joyent_test

import (
	"fmt"
	"io/ioutil"
	"os"

	"github.com/juju/testing"
	gc "launchpad.net/gocheck"

	"launchpad.net/juju-core/environs"
	"launchpad.net/juju-core/environs/config"
	jp "launchpad.net/juju-core/provider/joyent"
	coretesting "launchpad.net/juju-core/testing"
	"launchpad.net/juju-core/utils"
	"launchpad.net/juju-core/utils/ssh"
)

func newConfig(c *gc.C, attrs coretesting.Attrs) *config.Config {
	attrs = coretesting.FakeConfig().Merge(attrs)
	cfg, err := config.New(config.UseDefaults, attrs)
	c.Assert(err, gc.IsNil)
	return cfg
}

func validAttrs() coretesting.Attrs {
	return coretesting.FakeConfig().Merge(coretesting.Attrs{
		"type":             "joyent",
		"sdc-user":         "juju-test",
		"sdc-key-id":       "00:11:22:33:44:55:66:77:88:99:aa:bb:cc:dd:ee:ff",
		"sdc-url":          "https://test.api.joyentcloud.com",
		"manta-user":       "juju-test",
		"manta-key-id":     "00:11:22:33:44:55:66:77:88:99:aa:bb:cc:dd:ee:ff",
		"manta-url":        "https://test.manta.joyent.com",
		"private-key-path": "~/.ssh/provider_id_rsa",
		"algorithm":        "rsa-sha256",
		"control-dir":      "juju-test",
	})
}

type ConfigSuite struct {
	coretesting.FakeJujuHomeSuite
	originalValues map[string]testing.Restorer
	privateKeyData string
}

var _ = gc.Suite(&ConfigSuite{})

func (s *ConfigSuite) SetUpSuite(c *gc.C) {
	s.FakeJujuHomeSuite.SetUpSuite(c)
	restoreSdcAccount := testing.PatchEnvironment(jp.SdcAccount, "tester")
	s.AddSuiteCleanup(func(*gc.C) { restoreSdcAccount() })
	restoreSdcKeyId := testing.PatchEnvironment(jp.SdcKeyId, "ff:ee:dd:cc:bb:aa:99:88:77:66:55:44:33:22:11:00")
	s.AddSuiteCleanup(func(*gc.C) { restoreSdcKeyId() })
	restoreMantaUser := testing.PatchEnvironment(jp.MantaUser, "tester")
	s.AddSuiteCleanup(func(*gc.C) { restoreMantaUser() })
	restoreMantaKeyId := testing.PatchEnvironment(jp.MantaKeyId, "ff:ee:dd:cc:bb:aa:99:88:77:66:55:44:33:22:11:00")
	s.AddSuiteCleanup(func(*gc.C) { restoreMantaKeyId() })
	s.privateKeyData = generatePrivateKey(c)
}

func generatePrivateKey(c *gc.C) string {
	oldBits := ssh.KeyBits
	defer func() {
		ssh.KeyBits = oldBits
	}()
	ssh.KeyBits = 32
	private, _, err := ssh.GenerateKey("test-client")
	c.Assert(err, gc.IsNil)
	return private
}

func (s *ConfigSuite) SetUpTest(c *gc.C) {
	s.FakeJujuHomeSuite.SetUpTest(c)
	s.AddCleanup(CreateTestKey(c))
	for _, envVar := range jp.EnvironmentVariables {
		s.PatchEnvironment(envVar, "")
	}
}

var newConfigTests = []struct {
	info    string
	insert  coretesting.Attrs
	remove  []string
	envVars map[string]string
	expect  coretesting.Attrs
	err     string
}{{
	info:   "sdc-user is required",
	remove: []string{"sdc-user"},
	err:    ".* cannot get sdc-user value from environment variable .*",
}, {
	info:   "sdc-user cannot be empty",
	insert: coretesting.Attrs{"sdc-user": ""},
	err:    ".* cannot get sdc-user value from environment variable .*",
}, {
	info:   "can get sdc-user from env variable",
	insert: coretesting.Attrs{"sdc-user": ""},
	expect: coretesting.Attrs{"sdc-user": "tester"},
	envVars: map[string]string{
		"SDC_ACCOUNT": "tester",
	},
}, {
	info:   "can get sdc-user from env variable, missing from config",
	remove: []string{"sdc-user"},
	expect: coretesting.Attrs{"sdc-user": "tester"},
	envVars: map[string]string{
		"SDC_ACCOUNT": "tester",
	},
}, {
	info:   "sdc-key-id is required",
	remove: []string{"sdc-key-id"},
	err:    ".* cannot get sdc-key-id value from environment variable .*",
}, {
	info:   "sdc-key-id cannot be empty",
	insert: coretesting.Attrs{"sdc-key-id": ""},
	err:    ".* cannot get sdc-key-id value from environment variable .*",
}, {
	info:   "can get sdc-key-id from env variable",
	insert: coretesting.Attrs{"sdc-key-id": ""},
	expect: coretesting.Attrs{"sdc-key-id": "key"},
	envVars: map[string]string{
		"SDC_KEY_ID": "key",
	},
}, {
	info:   "can get sdc-key-id from env variable, missing from config",
	remove: []string{"sdc-key-id"},
	expect: coretesting.Attrs{"sdc-key-id": "key"},
	envVars: map[string]string{
		"SDC_KEY_ID": "key",
	},
}, {
	info:   "sdc-url is inserted if missing",
	expect: coretesting.Attrs{"sdc-url": "https://test.api.joyentcloud.com"},
}, {
	info:   "sdc-url cannot be empty",
	insert: coretesting.Attrs{"sdc-url": ""},
	err:    ".* cannot get sdc-url value from environment variable .*",
}, {
	info:   "sdc-url is untouched if present",
	insert: coretesting.Attrs{"sdc-url": "https://test.api.joyentcloud.com"},
	expect: coretesting.Attrs{"sdc-url": "https://test.api.joyentcloud.com"},
}, {
	info:   "manta-user is required",
	remove: []string{"manta-user"},
	err:    ".* cannot get manta-user value from environment variable .*",
}, {
	info:   "manta-user cannot be empty",
	insert: coretesting.Attrs{"manta-user": ""},
	err:    ".* cannot get manta-user value from environment variable .*",
}, {
	info:   "can get manta-user from env variable",
	insert: coretesting.Attrs{"manta-user": ""},
	expect: coretesting.Attrs{"manta-user": "tester"},
	envVars: map[string]string{
		"MANTA_USER": "tester",
	},
}, {
	info:   "can get manta-user from env variable, missing from config",
	remove: []string{"manta-user"},
	expect: coretesting.Attrs{"manta-user": "tester"},
	envVars: map[string]string{
		"MANTA_USER": "tester",
	},
}, {
	info:   "manta-key-id is required",
	remove: []string{"manta-key-id"},
	err:    ".* cannot get manta-key-id value from environment variable .*",
}, {
	info:   "manta-key-id cannot be empty",
	insert: coretesting.Attrs{"manta-key-id": ""},
	err:    ".* cannot get manta-key-id value from environment variable .*",
}, {
	info:   "can get manta-key-id from env variable",
	insert: coretesting.Attrs{"manta-key-id": ""},
	expect: coretesting.Attrs{"manta-key-id": "key"},
	envVars: map[string]string{
		"MANTA_KEY_ID": "key",
	},
}, {
	info:   "can get manta-key-id from env variable, missing from config",
	remove: []string{"manta-key-id"},
	expect: coretesting.Attrs{"manta-key-id": "key"},
	envVars: map[string]string{
		"MANTA_KEY_ID": "key",
	},
}, {
	info:   "manta-url is inserted if missing",
	expect: coretesting.Attrs{"manta-url": "https://test.manta.joyent.com"},
}, {
	info:   "manta-url cannot be empty",
	insert: coretesting.Attrs{"manta-url": ""},
	err:    ".* cannot get manta-url value from environment variable .*",
}, {
	info:   "manta-url is untouched if present",
	insert: coretesting.Attrs{"manta-url": "https://test.manta.joyent.com"},
	expect: coretesting.Attrs{"manta-url": "https://test.manta.joyent.com"},
}, {
	info:   "private-key-path is inserted if missing",
	remove: []string{"private-key-path"},
	expect: coretesting.Attrs{"private-key-path": "~/.ssh/id_rsa"},
}, {
	info:   "can get private-key-path from env variable",
	insert: coretesting.Attrs{"private-key-path": ""},
	expect: coretesting.Attrs{"private-key-path": "some-file"},
	envVars: map[string]string{
		"MANTA_PRIVATE_KEY_FILE": "some-file",
	},
}, {
	info:   "can get private-key-path from env variable, missing from config",
	remove: []string{"private-key-path"},
	expect: coretesting.Attrs{"private-key-path": "some-file"},
	envVars: map[string]string{
		"MANTA_PRIVATE_KEY_FILE": "some-file",
	},
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

func (s *ConfigSuite) TestNewEnvironConfig(c *gc.C) {
	for i, test := range newConfigTests {
		c.Logf("test %d: %s", i, test.info)
		for k, v := range test.envVars {
			os.Setenv(k, v)
		}
		attrs := validAttrs().Merge(test.insert).Delete(test.remove...)
		attrs["private-key"] = s.privateKeyData
		testConfig := newConfig(c, attrs)
		environ, err := environs.New(testConfig)
		if test.err == "" {
			c.Check(err, gc.IsNil)
			if err != nil {
				continue
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
	insert: coretesting.Attrs{"sdc-url": "https://test.api.joyentcloud.com"},
	expect: coretesting.Attrs{"sdc-url": "https://test.api.joyentcloud.com"},
}, {
	info:   "can change manta-user",
	insert: coretesting.Attrs{"manta-user": "manta_user"},
	expect: coretesting.Attrs{"manta-user": "manta_user"},
}, {
	info:   "can change manta-key-id",
	insert: coretesting.Attrs{"manta-key-id": "ff:ee:dd:cc:bb:aa:99:88:77:66:55:44:33:22:11:00"},
	expect: coretesting.Attrs{"manta-key-id": "ff:ee:dd:cc:bb:aa:99:88:77:66:55:44:33:22:11:00"},
}, {
	info:   "can change manta-url",
	insert: coretesting.Attrs{"manta-url": "https://test.manta.joyent.com"},
	expect: coretesting.Attrs{"manta-url": "https://test.manta.joyent.com"},
}, {
	info:   "can insert unknown field",
	insert: coretesting.Attrs{"unknown": "ignoti"},
	expect: coretesting.Attrs{"unknown": "ignoti"},
}}

func (s *ConfigSuite) TestValidateChange(c *gc.C) {
	attrs := validAttrs()
	attrs["private-key"] = s.privateKeyData
	baseConfig := newConfig(c, attrs)
	for i, test := range changeConfigTests {
		c.Logf("test %d: %s", i, test.info)
		attrs := validAttrs().Merge(test.insert).Delete(test.remove...)
		attrs["private-key"] = s.privateKeyData
		testConfig := newConfig(c, attrs)
		validatedConfig, err := jp.Provider.Validate(testConfig, baseConfig)
		if test.err == "" {
			c.Check(err, gc.IsNil)
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

func validPrepareAttrs() coretesting.Attrs {
	return validAttrs().Delete("private-key")
}

var prepareConfigTests = []struct {
	info   string
	insert coretesting.Attrs
	remove []string
	expect coretesting.Attrs
	err    string
}{{
	info:   "All value provided, nothig to do",
	expect: validPrepareAttrs(),
}, {
	info:   "private key is loaded from key file",
	insert: coretesting.Attrs{"private-key-path": fmt.Sprintf("~/.ssh/%s", testKeyFileName)},
	expect: coretesting.Attrs{"private-key": testPrivateKey},
}, {
	info:   "bad private-key-path errors, not panics",
	insert: coretesting.Attrs{"private-key-path": "~/.ssh/no-such-file"},
	err:    "invalid Joyent provider config: open .*: no such file or directory",
}}

func (s *ConfigSuite) TestPrepare(c *gc.C) {
	ctx := coretesting.Context(c)
	for i, test := range prepareConfigTests {
		c.Logf("test %d: %s", i, test.info)
		attrs := validPrepareAttrs().Merge(test.insert).Delete(test.remove...)
		testConfig := newConfig(c, attrs)
		preparedConfig, err := jp.Provider.Prepare(ctx, testConfig)
		if test.err == "" {
			c.Check(err, gc.IsNil)
			attrs := preparedConfig.Config().AllAttrs()
			for field, value := range test.expect {
				c.Check(attrs[field], gc.Equals, value)
			}
		} else {
			c.Check(preparedConfig, gc.IsNil)
			c.Check(err, gc.ErrorMatches, test.err)
		}
	}
}

func (s *ConfigSuite) TestPrepareWithDefaultKeyFile(c *gc.C) {
	ctx := coretesting.Context(c)
	// By default "private-key-path isn't set until after validateConfig has been called.
	attrs := validAttrs().Delete("private-key-path", "private-key")
	keyFilePath, err := utils.NormalizePath(jp.DefaultPrivateKey)
	c.Assert(err, gc.IsNil)
	err = ioutil.WriteFile(keyFilePath, []byte(testPrivateKey), 400)
	c.Assert(err, gc.IsNil)
	defer os.Remove(keyFilePath)
	testConfig := newConfig(c, attrs)
	preparedConfig, err := jp.Provider.Prepare(ctx, testConfig)
	c.Assert(err, gc.IsNil)
	attrs = preparedConfig.Config().AllAttrs()
	c.Check(attrs["private-key-path"], gc.Equals, jp.DefaultPrivateKey)
	c.Check(attrs["private-key"], gc.Equals, testPrivateKey)
}
