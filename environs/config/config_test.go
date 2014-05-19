// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package config_test

import (
	"fmt"
	"regexp"
	stdtesting "testing"
	"time"

	"github.com/juju/loggo"
	jc "github.com/juju/testing/checkers"
	gc "launchpad.net/gocheck"

	"launchpad.net/juju-core/cert"
	"launchpad.net/juju-core/environs/config"
	"launchpad.net/juju-core/juju/osenv"
	"launchpad.net/juju-core/schema"
	"launchpad.net/juju-core/testing"
	"launchpad.net/juju-core/version"
)

func Test(t *stdtesting.T) {
	gc.TestingT(t)
}

type ConfigSuite struct {
	testing.FakeJujuHomeSuite
	home string
}

var _ = gc.Suite(&ConfigSuite{})

func (s *ConfigSuite) SetUpTest(c *gc.C) {
	s.FakeJujuHomeSuite.SetUpTest(c)
	// Make sure that the defaults are used, which
	// is <root>=WARNING
	loggo.ResetLoggers()
}

// sampleConfig holds a configuration with all required
// attributes set.
var sampleConfig = testing.Attrs{
	"type":                      "my-type",
	"name":                      "my-name",
	"authorized-keys":           testing.FakeAuthKeys,
	"firewall-mode":             config.FwInstance,
	"admin-secret":              "foo",
	"unknown":                   "my-unknown",
	"ca-cert":                   caCert,
	"ssl-hostname-verification": true,
	"development":               false,
	"state-port":                1234,
	"api-port":                  4321,
	"syslog-port":               2345,
	"default-series":            "precise",
}

type configTest struct {
	about       string
	useDefaults config.Defaulting
	attrs       testing.Attrs
	expected    testing.Attrs
	err         string
}

var configTests = []configTest{
	{
		about:       "The minimum good configuration",
		useDefaults: config.UseDefaults,
		attrs: testing.Attrs{
			"type": "my-type",
			"name": "my-name",
		},
	}, {
		about:       "Metadata URLs",
		useDefaults: config.UseDefaults,
		attrs: testing.Attrs{
			"type":               "my-type",
			"name":               "my-name",
			"image-metadata-url": "image-url",
			"tools-metadata-url": "tools-metadata-url-value",
		},
	}, {
		about:       "Deprecated tools metadata URL used",
		useDefaults: config.UseDefaults,
		attrs: testing.Attrs{
			"type":      "my-type",
			"name":      "my-name",
			"tools-url": "tools-metadata-url-value",
		},
	}, {
		about:       "Deprecated tools metadata URL ignored",
		useDefaults: config.UseDefaults,
		attrs: testing.Attrs{
			"type":               "my-type",
			"name":               "my-name",
			"tools-metadata-url": "tools-metadata-url-value",
			"tools-url":          "ignore-me",
		},
	}, {
		about:       "Explicit series",
		useDefaults: config.UseDefaults,
		attrs: testing.Attrs{
			"type":           "my-type",
			"name":           "my-name",
			"default-series": "my-series",
		},
	}, {
		about:       "Implicit series with empty value",
		useDefaults: config.UseDefaults,
		attrs: testing.Attrs{
			"type":           "my-type",
			"name":           "my-name",
			"default-series": "",
		},
	}, {
		about:       "Explicit logging",
		useDefaults: config.UseDefaults,
		attrs: testing.Attrs{
			"type":           "my-type",
			"name":           "my-name",
			"logging-config": "juju=INFO",
		},
	}, {
		about:       "Explicit authorized-keys",
		useDefaults: config.UseDefaults,
		attrs: testing.Attrs{
			"type":            "my-type",
			"name":            "my-name",
			"authorized-keys": testing.FakeAuthKeys,
		},
	}, {
		about:       "Load authorized-keys from path",
		useDefaults: config.UseDefaults,
		attrs: testing.Attrs{
			"type":                 "my-type",
			"name":                 "my-name",
			"authorized-keys-path": "~/.ssh/authorized_keys2",
		},
	}, {
		about:       "LXC clone values",
		useDefaults: config.UseDefaults,
		attrs: testing.Attrs{
			"type":           "my-type",
			"name":           "my-name",
			"default-series": "precise",
			"lxc-clone":      true,
			"lxc-clone-aufs": true,
		},
	}, {
		about:       "Deprecated lxc-use-clone used",
		useDefaults: config.UseDefaults,
		attrs: testing.Attrs{
			"type":          "my-type",
			"name":          "my-name",
			"lxc-use-clone": true,
		},
	}, {
		about:       "Deprecated lxc-use-clone ignored",
		useDefaults: config.UseDefaults,
		attrs: testing.Attrs{
			"type":          "my-type",
			"name":          "my-name",
			"lxc-use-clone": false,
			"lxc-clone":     true,
		},
	}, {
		about:       "CA cert & key from path",
		useDefaults: config.UseDefaults,
		attrs: testing.Attrs{
			"type":                "my-type",
			"name":                "my-name",
			"ca-cert-path":        "cacert2.pem",
			"ca-private-key-path": "cakey2.pem",
		},
	}, {
		about:       "CA cert & key from path; cert attribute set too",
		useDefaults: config.UseDefaults,
		attrs: testing.Attrs{
			"type":                "my-type",
			"name":                "my-name",
			"ca-cert-path":        "cacert2.pem",
			"ca-cert":             "ignored",
			"ca-private-key-path": "cakey2.pem",
		},
	}, {
		about:       "CA cert & key from ~ path",
		useDefaults: config.UseDefaults,
		attrs: testing.Attrs{
			"type":                "my-type",
			"name":                "my-name",
			"ca-cert-path":        "~/othercert.pem",
			"ca-private-key-path": "~/otherkey.pem",
		},
	}, /* {
		about: "CA cert only from ~ path",
		useDefaults: config.UseDefaults,
		attrs: testing.Attrs{
			"type":           "my-type",
			"name":           "my-name",
			"ca-cert-path":   "~/othercert.pem",
			"ca-private-key": "",
		},
	}, {
		about: "CA cert only as attribute",
		useDefaults: config.UseDefaults,
		attrs: testing.Attrs{
			"type":           "my-type",
			"name":           "my-name",
			"ca-cert":        caCert,
			"ca-private-key": "",
		},
	}, */{
		about:       "CA cert and key as attributes",
		useDefaults: config.UseDefaults,
		attrs: testing.Attrs{
			"type":           "my-type",
			"name":           "my-name",
			"ca-cert":        caCert,
			"ca-private-key": caKey,
		},
	}, {
		about:       "Mismatched CA cert and key",
		useDefaults: config.UseDefaults,
		attrs: testing.Attrs{
			"type":           "my-type",
			"name":           "my-name",
			"ca-cert":        caCert,
			"ca-private-key": caKey2,
		},
		err: "bad CA certificate/key in configuration: crypto/tls: private key does not match public key",
	}, {
		about:       "Invalid CA cert",
		useDefaults: config.UseDefaults,
		attrs: testing.Attrs{
			"type":    "my-type",
			"name":    "my-name",
			"ca-cert": invalidCACert,
		},
		err: `bad CA certificate/key in configuration: (asn1:|ASN\.1) syntax error:.*`,
	}, {
		about:       "Invalid CA key",
		useDefaults: config.UseDefaults,
		attrs: testing.Attrs{
			"type":           "my-type",
			"name":           "my-name",
			"ca-cert":        caCert,
			"ca-private-key": invalidCAKey,
		},
		err: "bad CA certificate/key in configuration: crypto/tls:.*",
	}, /* {
		about: "No CA cert or key",
		useDefaults: config.UseDefaults,
		attrs: testing.Attrs{
			"type":           "my-type",
			"name":           "my-name",
			"ca-cert":        "",
			"ca-private-key": "",
		},
	}, {
		about: "CA key but no cert",
		useDefaults: config.UseDefaults,
		attrs: testing.Attrs{
			"type":           "my-type",
			"name":           "my-name",
			"ca-cert":        "",
			"ca-private-key": caKey,
		},
		err: "bad CA certificate/key in configuration: crypto/tls:.*",
	}, {
		about: "No CA key",
		useDefaults: config.UseDefaults,
		attrs: testing.Attrs{
			"type":           "my-type",
			"name":           "my-name",
			"ca-cert":        "foo",
			"ca-private-key": "",
		},
		err: "bad CA certificate/key in configuration: no certificates found",
	}, */{
		about:       "CA cert specified as non-existent file",
		useDefaults: config.UseDefaults,
		attrs: testing.Attrs{
			"type":         "my-type",
			"name":         "my-name",
			"ca-cert-path": "no-such-file",
		},
		err: `open .*\.juju/no-such-file: .*`,
	}, {
		about:       "CA key specified as non-existent file",
		useDefaults: config.UseDefaults,
		attrs: testing.Attrs{
			"type":                "my-type",
			"name":                "my-name",
			"ca-private-key-path": "no-such-file",
		},
		err: `open .*\.juju/no-such-file: .*`,
	}, {
		about:       "Specified agent version",
		useDefaults: config.UseDefaults,
		attrs: testing.Attrs{
			"type":            "my-type",
			"name":            "my-name",
			"authorized-keys": testing.FakeAuthKeys,
			"agent-version":   "1.2.3",
		},
	}, {
		about:       "Specified development flag",
		useDefaults: config.UseDefaults,
		attrs: testing.Attrs{
			"type":            "my-type",
			"name":            "my-name",
			"authorized-keys": testing.FakeAuthKeys,
			"development":     true,
		},
	}, {
		about:       "Specified admin secret",
		useDefaults: config.UseDefaults,
		attrs: testing.Attrs{
			"type":            "my-type",
			"name":            "my-name",
			"authorized-keys": testing.FakeAuthKeys,
			"development":     false,
			"admin-secret":    "pork",
		},
	}, {
		about:       "Invalid development flag",
		useDefaults: config.UseDefaults,
		attrs: testing.Attrs{
			"type":            "my-type",
			"name":            "my-name",
			"authorized-keys": testing.FakeAuthKeys,
			"development":     "invalid",
		},
		err: `development: expected bool, got string\("invalid"\)`,
	}, {
		about:       "Invalid agent version",
		useDefaults: config.UseDefaults,
		attrs: testing.Attrs{
			"type":            "my-type",
			"name":            "my-name",
			"authorized-keys": testing.FakeAuthKeys,
			"agent-version":   "2",
		},
		err: `invalid agent version in environment configuration: "2"`,
	}, {
		about:       "Missing type",
		useDefaults: config.UseDefaults,
		attrs: testing.Attrs{
			"name": "my-name",
		},
		err: "type: expected string, got nothing",
	}, {
		about:       "Empty type",
		useDefaults: config.UseDefaults,
		attrs: testing.Attrs{
			"name": "my-name",
			"type": "",
		},
		err: "empty type in environment configuration",
	}, {
		about:       "Missing name",
		useDefaults: config.UseDefaults,
		attrs: testing.Attrs{
			"type": "my-type",
		},
		err: "name: expected string, got nothing",
	}, {
		about:       "Bad name, no slash",
		useDefaults: config.UseDefaults,
		attrs: testing.Attrs{
			"name": "foo/bar",
			"type": "my-type",
		},
		err: "environment name contains unsafe characters",
	}, {
		about:       "Bad name, no backslash",
		useDefaults: config.UseDefaults,
		attrs: testing.Attrs{
			"name": "foo\\bar",
			"type": "my-type",
		},
		err: "environment name contains unsafe characters",
	}, {
		about:       "Empty name",
		useDefaults: config.UseDefaults,
		attrs: testing.Attrs{
			"type": "my-type",
			"name": "",
		},
		err: "empty name in environment configuration",
	}, {
		about:       "Default firewall mode",
		useDefaults: config.UseDefaults,
		attrs: testing.Attrs{
			"type": "my-type",
			"name": "my-name",
		},
	}, {
		about:       "Empty firewall mode",
		useDefaults: config.UseDefaults,
		attrs: testing.Attrs{
			"type":          "my-type",
			"name":          "my-name",
			"firewall-mode": "",
		},
	}, {
		about:       "Instance firewall mode",
		useDefaults: config.UseDefaults,
		attrs: testing.Attrs{
			"type":          "my-type",
			"name":          "my-name",
			"firewall-mode": config.FwInstance,
		},
	}, {
		about:       "Global firewall mode",
		useDefaults: config.UseDefaults,
		attrs: testing.Attrs{
			"type":          "my-type",
			"name":          "my-name",
			"firewall-mode": config.FwGlobal,
		},
	}, {
		about:       "Illegal firewall mode",
		useDefaults: config.UseDefaults,
		attrs: testing.Attrs{
			"type":          "my-type",
			"name":          "my-name",
			"firewall-mode": "illegal",
		},
		err: "invalid firewall mode in environment configuration: .*",
	}, {
		about:       "ssl-hostname-verification off",
		useDefaults: config.UseDefaults,
		attrs: testing.Attrs{
			"type": "my-type",
			"name": "my-name",
			"ssl-hostname-verification": false,
		},
	}, {
		about:       "ssl-hostname-verification incorrect",
		useDefaults: config.UseDefaults,
		attrs: testing.Attrs{
			"type": "my-type",
			"name": "my-name",
			"ssl-hostname-verification": "yes please",
		},
		err: `ssl-hostname-verification: expected bool, got string\("yes please"\)`,
	}, {
		about:       "provisioner-safe-mode off",
		useDefaults: config.UseDefaults,
		attrs: testing.Attrs{
			"type":                  "my-type",
			"name":                  "my-name",
			"provisioner-safe-mode": false,
		},
	}, {
		about:       "provisioner-safe-mode on",
		useDefaults: config.UseDefaults,
		attrs: testing.Attrs{
			"type":                  "my-type",
			"name":                  "my-name",
			"provisioner-safe-mode": true,
		},
	}, {
		about:       "provisioner-safe-mode incorrect",
		useDefaults: config.UseDefaults,
		attrs: testing.Attrs{
			"type":                  "my-type",
			"name":                  "my-name",
			"provisioner-safe-mode": "yes please",
		},
		err: `provisioner-safe-mode: expected bool, got string\("yes please"\)`,
	}, {
		about:       "default image stream",
		useDefaults: config.UseDefaults,
		attrs: testing.Attrs{
			"type": "my-type",
			"name": "my-name",
		},
	}, {
		about:       "explicit image stream",
		useDefaults: config.UseDefaults,
		attrs: testing.Attrs{
			"type":         "my-type",
			"name":         "my-name",
			"image-stream": "daily",
		},
	}, {
		about:       "Explicit state port",
		useDefaults: config.UseDefaults,
		attrs: testing.Attrs{
			"type":       "my-type",
			"name":       "my-name",
			"state-port": 37042,
		},
	}, {
		about:       "Invalid state port",
		useDefaults: config.UseDefaults,
		attrs: testing.Attrs{
			"type":       "my-type",
			"name":       "my-name",
			"state-port": "illegal",
		},
		err: `state-port: expected number, got string\("illegal"\)`,
	}, {
		about:       "Explicit API port",
		useDefaults: config.UseDefaults,
		attrs: testing.Attrs{
			"type":     "my-type",
			"name":     "my-name",
			"api-port": 77042,
		},
	}, {
		about:       "Invalid API port",
		useDefaults: config.UseDefaults,
		attrs: testing.Attrs{
			"type":     "my-type",
			"name":     "my-name",
			"api-port": "illegal",
		},
		err: `api-port: expected number, got string\("illegal"\)`,
	}, {
		about:       "Explicit syslog port",
		useDefaults: config.UseDefaults,
		attrs: testing.Attrs{
			"type":        "my-type",
			"name":        "my-name",
			"syslog-port": 3456,
		},
	}, {
		about:       "Invalid syslog port",
		useDefaults: config.UseDefaults,
		attrs: testing.Attrs{
			"type":        "my-type",
			"name":        "my-name",
			"syslog-port": "illegal",
		},
		err: `syslog-port: expected number, got string\("illegal"\)`,
	}, {
		about:       "Explicit bootstrap timeout",
		useDefaults: config.UseDefaults,
		attrs: testing.Attrs{
			"type":              "my-type",
			"name":              "my-name",
			"bootstrap-timeout": 300,
		},
	}, {
		about:       "Invalid bootstrap timeout",
		useDefaults: config.UseDefaults,
		attrs: testing.Attrs{
			"type":              "my-type",
			"name":              "my-name",
			"bootstrap-timeout": "illegal",
		},
		err: `bootstrap-timeout: expected number, got string\("illegal"\)`,
	}, {
		about:       "Explicit bootstrap retry delay",
		useDefaults: config.UseDefaults,
		attrs: testing.Attrs{
			"type":                  "my-type",
			"name":                  "my-name",
			"bootstrap-retry-delay": 5,
		},
	}, {
		about:       "Invalid bootstrap retry delay",
		useDefaults: config.UseDefaults,
		attrs: testing.Attrs{
			"type":                  "my-type",
			"name":                  "my-name",
			"bootstrap-retry-delay": "illegal",
		},
		err: `bootstrap-retry-delay: expected number, got string\("illegal"\)`,
	}, {
		about:       "Explicit bootstrap addresses delay",
		useDefaults: config.UseDefaults,
		attrs: testing.Attrs{
			"type": "my-type",
			"name": "my-name",
			"bootstrap-addresses-delay": 15,
		},
	}, {
		about:       "Invalid bootstrap addresses delay",
		useDefaults: config.UseDefaults,
		attrs: testing.Attrs{
			"type": "my-type",
			"name": "my-name",
			"bootstrap-addresses-delay": "illegal",
		},
		err: `bootstrap-addresses-delay: expected number, got string\("illegal"\)`,
	}, {
		about:       "Invalid logging configuration",
		useDefaults: config.UseDefaults,
		attrs: testing.Attrs{
			"type":           "my-type",
			"name":           "my-name",
			"logging-config": "foo=bar",
		},
		err: `unknown severity level "bar"`,
	}, {
		about:       "Sample configuration",
		useDefaults: config.UseDefaults,
		attrs:       sampleConfig,
	}, {
		about:       "No defaults: sample configuration",
		useDefaults: config.NoDefaults,
		attrs:       sampleConfig,
	}, {
		about:       "No defaults: with ca-cert-path",
		useDefaults: config.NoDefaults,
		attrs:       sampleConfig.Merge(testing.Attrs{"ca-cert-path": "arble"}),
		err:         `attribute "ca-cert-path" is not allowed in configuration`,
	}, {
		about:       "No defaults: with ca-private-key-path",
		useDefaults: config.NoDefaults,
		attrs:       sampleConfig.Merge(testing.Attrs{"ca-private-key-path": "arble"}),
		err:         `attribute "ca-private-key-path" is not allowed in configuration`,
	}, {
		about:       "No defaults: with authorized-keys-path",
		useDefaults: config.NoDefaults,
		attrs:       sampleConfig.Merge(testing.Attrs{"authorized-keys-path": "arble"}),
		err:         `attribute "authorized-keys-path" is not allowed in configuration`,
	}, {
		about:       "No defaults: missing authorized-keys",
		useDefaults: config.NoDefaults,
		attrs:       sampleConfig.Delete("authorized-keys"),
		err:         `authorized-keys missing from environment configuration`,
	}, {
		about:       "Config settings from juju 1.13.3 actual installation",
		useDefaults: config.NoDefaults,
		attrs: map[string]interface{}{
			"name":                      "sample",
			"development":               false,
			"admin-secret":              "",
			"ssl-hostname-verification": true,
			"authorized-keys":           "ssh-rsa mykeys rog@rog-x220\n",
			"control-bucket":            "rog-some-control-bucket",
			"region":                    "us-east-1",
			"image-metadata-url":        "",
			"ca-private-key":            "",
			"default-series":            "precise",
			"tools-metadata-url":        "",
			"secret-key":                "a-secret-key",
			"access-key":                "an-access-key",
			"agent-version":             "1.13.2",
			"ca-cert":                   caCert,
			"firewall-mode":             "instance",
			"type":                      "ec2",
		},
	}, {
		about:       "Provider type null is replaced with manual",
		useDefaults: config.UseDefaults,
		attrs: testing.Attrs{
			"type": "null",
			"name": "my-name",
		},
	}, {
		about:       "TestMode flag specified",
		useDefaults: config.UseDefaults,
		attrs: testing.Attrs{
			"type":      "my-type",
			"name":      "my-name",
			"test-mode": true,
		},
	},
	authTokenConfigTest("token=value, tokensecret=value", true),
	authTokenConfigTest("token=value, ", true),
	authTokenConfigTest("token=value, \ttokensecret=value", true),
	authTokenConfigTest("", true),
	authTokenConfigTest("token=value, tokensecret=value, \t", true),
	authTokenConfigTest("=", false),
	authTokenConfigTest("tokenvalue", false),
	authTokenConfigTest("token=value, sometoken=", false),
	authTokenConfigTest("token==value", false),
	authTokenConfigTest(" token=value", false),
	authTokenConfigTest("=value", false),
	authTokenConfigTest("token=value, =z", false),
	authTokenConfigTest("token=value =z", false),
	authTokenConfigTest("\t", false),
	missingAttributeNoDefault("firewall-mode"),
	missingAttributeNoDefault("development"),
	missingAttributeNoDefault("ssl-hostname-verification"),
	// TODO(rog) reinstate these tests when we can lose
	// backward compatibility with pre-1.13 config.
	// missingAttributeNoDefault("state-port"),
	// missingAttributeNoDefault("api-port"),
}

// authTokenConfigTest returns a config test that checks
// that a configuration with the given auth token
// will pass or fail, depending on the value of ok.
func authTokenConfigTest(token string, ok bool) configTest {
	var testName string
	var err string

	if ok {
		testName = fmt.Sprintf("Valid auth token test: %q", token)
	} else {
		testName = fmt.Sprintf("Invalid auth token test: %q", token)
		err = fmt.Sprintf("charm store auth token needs to be a set of key-value pairs, not %q", token)
	}

	return configTest{
		about:       testName,
		useDefaults: config.UseDefaults,
		attrs:       sampleConfig.Merge(testing.Attrs{"charm-store-auth": token}),
		err:         regexp.QuoteMeta(err),
	}
}

func missingAttributeNoDefault(attrName string) configTest {
	return configTest{
		about:       fmt.Sprintf("No default: missing %s", attrName),
		useDefaults: config.NoDefaults,
		attrs:       sampleConfig.Delete(attrName),
		err:         fmt.Sprintf("%s: expected [a-z]+, got nothing", attrName),
	}
}

type testFile struct {
	name, data string
}

func (s *ConfigSuite) TestConfig(c *gc.C) {
	files := []testing.TestFile{
		{".ssh/id_dsa.pub", "dsa"},
		{".ssh/id_rsa.pub", "rsa\n"},
		{".ssh/identity.pub", "identity"},
		{".ssh/authorized_keys", "auth0\n# first\nauth1\n\n"},
		{".ssh/authorized_keys2", "auth2\nauth3\n"},

		{".juju/my-name-cert.pem", caCert},
		{".juju/my-name-private-key.pem", caKey},
		{".juju/cacert2.pem", caCert2},
		{".juju/cakey2.pem", caKey2},
		{"othercert.pem", caCert3},
		{"otherkey.pem", caKey3},
	}
	s.FakeHomeSuite.Home.AddFiles(c, files...)
	for i, test := range configTests {
		c.Logf("test %d. %s", i, test.about)
		test.check(c, s.FakeHomeSuite.Home)
	}
}

var noCertFilesTests = []configTest{
	{
		about:       "Unspecified certificate and key",
		useDefaults: config.UseDefaults,
		attrs: testing.Attrs{
			"type":            "my-type",
			"name":            "my-name",
			"authorized-keys": testing.FakeAuthKeys,
		},
	}, {
		about:       "Unspecified certificate, specified key",
		useDefaults: config.UseDefaults,
		attrs: testing.Attrs{
			"type":            "my-type",
			"name":            "my-name",
			"authorized-keys": testing.FakeAuthKeys,
			"ca-private-key":  caKey,
		},
		err: "bad CA certificate/key in configuration: crypto/tls:.*",
	},
}

func (s *ConfigSuite) TestConfigNoCertFiles(c *gc.C) {
	for i, test := range noCertFilesTests {
		c.Logf("test %d. %s", i, test.about)
		test.check(c, s.FakeHomeSuite.Home)
	}
}

var emptyCertFilesTests = []configTest{
	{
		about:       "Cert unspecified; key specified",
		useDefaults: config.UseDefaults,
		attrs: testing.Attrs{
			"type":            "my-type",
			"name":            "my-name",
			"authorized-keys": testing.FakeAuthKeys,
			"ca-private-key":  caKey,
		},
		err: `file ".*/my-name-cert.pem" is empty`,
	}, {
		about:       "Cert and key unspecified",
		useDefaults: config.UseDefaults,
		attrs: testing.Attrs{
			"type":            "my-type",
			"name":            "my-name",
			"authorized-keys": testing.FakeAuthKeys,
		},
		err: `file ".*/my-name-cert.pem" is empty`,
	}, {
		about:       "Cert specified, key unspecified",
		useDefaults: config.UseDefaults,
		attrs: testing.Attrs{
			"type":            "my-type",
			"name":            "my-name",
			"authorized-keys": testing.FakeAuthKeys,
			"ca-cert":         caCert,
		},
		err: `file ".*/my-name-private-key.pem" is empty`,
	}, /* {
		about: "Cert and key specified as absent",
		useDefaults: config.UseDefaults,
		attrs: testing.Attrs{
			"type":            "my-type",
			"name":            "my-name",
			"authorized-keys": testing.FakeAuthKeys,
			"ca-cert":         "",
			"ca-private-key":  "",
		},
	}, {
		about: "Cert specified as absent",
		useDefaults: config.UseDefaults,
		attrs: testing.Attrs{
			"type":            "my-type",
			"name":            "my-name",
			"authorized-keys": testing.FakeAuthKeys,
			"ca-cert":         "",
		},
		err: "bad CA certificate/key in configuration: crypto/tls: .*",
	}, */
}

func (s *ConfigSuite) TestConfigEmptyCertFiles(c *gc.C) {
	files := []testing.TestFile{
		{".juju/my-name-cert.pem", ""},
		{".juju/my-name-private-key.pem", ""},
	}
	s.FakeHomeSuite.Home.AddFiles(c, files...)

	for i, test := range emptyCertFilesTests {
		c.Logf("test %d. %s", i, test.about)
		test.check(c, s.FakeHomeSuite.Home)
	}
}

func (test configTest) check(c *gc.C, home *testing.FakeHome) {
	cfg, err := config.New(test.useDefaults, test.attrs)
	if test.err != "" {
		c.Check(cfg, gc.IsNil)
		c.Assert(err, gc.ErrorMatches, test.err)
		return
	}
	c.Assert(err, gc.IsNil)

	typ, _ := test.attrs["type"].(string)
	// "null" has been deprecated in favour of "manual",
	// and is automatically switched.
	if typ == "null" {
		typ = "manual"
	}
	name, _ := test.attrs["name"].(string)
	c.Assert(cfg.Type(), gc.Equals, typ)
	c.Assert(cfg.Name(), gc.Equals, name)
	agentVersion, ok := cfg.AgentVersion()
	if s := test.attrs["agent-version"]; s != nil {
		c.Assert(ok, jc.IsTrue)
		c.Assert(agentVersion, gc.Equals, version.MustParse(s.(string)))
	} else {
		c.Assert(ok, jc.IsFalse)
		c.Assert(agentVersion, gc.Equals, version.Zero)
	}

	if statePort, ok := test.attrs["state-port"]; ok {
		c.Assert(cfg.StatePort(), gc.Equals, statePort)
	}
	if apiPort, ok := test.attrs["api-port"]; ok {
		c.Assert(cfg.APIPort(), gc.Equals, apiPort)
	}
	if syslogPort, ok := test.attrs["syslog-port"]; ok {
		c.Assert(cfg.SyslogPort(), gc.Equals, syslogPort)
	}

	dev, _ := test.attrs["development"].(bool)
	c.Assert(cfg.Development(), gc.Equals, dev)

	testmode, _ := test.attrs["test-mode"].(bool)
	c.Assert(cfg.TestMode(), gc.Equals, testmode)

	series, _ := test.attrs["default-series"].(string)
	if defaultSeries, ok := cfg.DefaultSeries(); ok {
		c.Assert(defaultSeries, gc.Equals, series)
	} else {
		c.Assert(series, gc.Equals, "")
		c.Assert(defaultSeries, gc.Equals, "")
	}

	if m, _ := test.attrs["firewall-mode"].(string); m != "" {
		c.Assert(cfg.FirewallMode(), gc.Equals, m)
	}
	if secret, _ := test.attrs["admin-secret"].(string); secret != "" {
		c.Assert(cfg.AdminSecret(), gc.Equals, secret)
	}

	if path, _ := test.attrs["authorized-keys-path"].(string); path != "" {
		c.Assert(cfg.AuthorizedKeys(), gc.Equals, home.FileContents(c, path))
		c.Assert(cfg.AllAttrs()["authorized-keys-path"], gc.IsNil)
	} else if keys, _ := test.attrs["authorized-keys"].(string); keys != "" {
		c.Assert(cfg.AuthorizedKeys(), gc.Equals, keys)
	} else {
		// Content of all the files that are read by default.
		c.Assert(cfg.AuthorizedKeys(), gc.Equals, "dsa\nrsa\nidentity\n")
	}

	cert, certPresent := cfg.CACert()
	if path, _ := test.attrs["ca-cert-path"].(string); path != "" {
		c.Assert(certPresent, jc.IsTrue)
		c.Assert(string(cert), gc.Equals, home.FileContents(c, path))
	} else if v, ok := test.attrs["ca-cert"].(string); v != "" {
		c.Assert(certPresent, jc.IsTrue)
		c.Assert(string(cert), gc.Equals, v)
	} else if ok {
		c.Check(cert, gc.HasLen, 0)
		c.Assert(certPresent, jc.IsFalse)
	} else if bool(test.useDefaults) && home.FileExists(".juju/my-name-cert.pem") {
		c.Assert(certPresent, jc.IsTrue)
		c.Assert(string(cert), gc.Equals, home.FileContents(c, "my-name-cert.pem"))
	} else {
		c.Check(cert, gc.HasLen, 0)
		c.Assert(certPresent, jc.IsFalse)
	}

	key, keyPresent := cfg.CAPrivateKey()
	if path, _ := test.attrs["ca-private-key-path"].(string); path != "" {
		c.Assert(keyPresent, jc.IsTrue)
		c.Assert(string(key), gc.Equals, home.FileContents(c, path))
	} else if v, ok := test.attrs["ca-private-key"].(string); v != "" {
		c.Assert(keyPresent, jc.IsTrue)
		c.Assert(string(key), gc.Equals, v)
	} else if ok {
		c.Check(key, gc.HasLen, 0)
		c.Assert(keyPresent, jc.IsFalse)
	} else if bool(test.useDefaults) && home.FileExists(".juju/my-name-private-key.pem") {
		c.Assert(keyPresent, jc.IsTrue)
		c.Assert(string(key), gc.Equals, home.FileContents(c, "my-name-private-key.pem"))
	} else {
		c.Check(key, gc.HasLen, 0)
		c.Assert(keyPresent, jc.IsFalse)
	}

	if v, ok := test.attrs["ssl-hostname-verification"]; ok {
		c.Assert(cfg.SSLHostnameVerification(), gc.Equals, v)
	}

	if v, ok := test.attrs["provisioner-safe-mode"]; ok {
		c.Assert(cfg.ProvisionerSafeMode(), gc.Equals, v)
	} else {
		c.Assert(cfg.ProvisionerSafeMode(), gc.Equals, false)
	}
	sshOpts := cfg.BootstrapSSHOpts()
	test.assertDuration(
		c,
		"bootstrap-timeout",
		sshOpts.Timeout,
		config.DefaultBootstrapSSHTimeout,
	)
	test.assertDuration(
		c,
		"bootstrap-retry-delay",
		sshOpts.RetryDelay,
		config.DefaultBootstrapSSHRetryDelay,
	)
	test.assertDuration(
		c,
		"bootstrap-addresses-delay",
		sshOpts.AddressesDelay,
		config.DefaultBootstrapSSHAddressesDelay,
	)

	if v, ok := test.attrs["image-stream"]; ok {
		c.Assert(cfg.ImageStream(), gc.Equals, v)
	} else {
		c.Assert(cfg.ImageStream(), gc.Equals, "released")
	}

	url, urlPresent := cfg.ImageMetadataURL()
	if v, _ := test.attrs["image-metadata-url"].(string); v != "" {
		c.Assert(url, gc.Equals, v)
		c.Assert(urlPresent, jc.IsTrue)
	} else {
		c.Assert(urlPresent, jc.IsFalse)
	}
	toolsURL, urlPresent := cfg.ToolsURL()
	oldToolsURL, oldURLPresent := cfg.AllAttrs()["tools-url"]
	oldToolsURLAttrValue, oldURLAttrPresent := test.attrs["tools-url"]
	expectedToolsURLValue := test.attrs["tools-metadata-url"]
	if expectedToolsURLValue == nil {
		expectedToolsURLValue = oldToolsURLAttrValue
	}
	if expectedToolsURLValue != nil && expectedToolsURLValue != "" {
		c.Assert(expectedToolsURLValue, gc.Equals, "tools-metadata-url-value")
		c.Assert(toolsURL, gc.Equals, expectedToolsURLValue)
		c.Assert(urlPresent, jc.IsTrue)
		c.Assert(oldToolsURL, gc.Equals, expectedToolsURLValue)
		c.Assert(oldURLPresent, jc.IsTrue)
	} else {
		c.Assert(urlPresent, jc.IsFalse)
		c.Assert(oldURLAttrPresent, jc.IsFalse)
		c.Assert(oldToolsURL, gc.Equals, "")
	}

	useLxcClone, useLxcClonePresent := cfg.LXCUseClone()
	oldUseClone, oldUseClonePresent := cfg.AllAttrs()["lxc-use-clone"]
	if v, ok := test.attrs["lxc-clone"]; ok {
		c.Assert(useLxcClone, gc.Equals, v)
		c.Assert(useLxcClonePresent, jc.IsTrue)
	} else {
		if oldUseClonePresent {
			c.Assert(useLxcClonePresent, jc.IsTrue)
			c.Assert(useLxcClone, gc.Equals, oldUseClone)
		} else {
			c.Assert(useLxcClonePresent, jc.IsFalse)
			c.Assert(useLxcClone, gc.Equals, false)
		}
	}
	useLxcCloneAufs, ok := cfg.LXCUseCloneAUFS()
	if v, ok := test.attrs["lxc-clone-aufs"]; ok {
		c.Assert(useLxcCloneAufs, gc.Equals, v)
	} else {
		c.Assert(useLxcCloneAufs, gc.Equals, false)
	}
}

func (test configTest) assertDuration(c *gc.C, name string, actual time.Duration, defaultInSeconds int) {
	value, ok := test.attrs[name].(int)
	if !ok || value == 0 {
		c.Assert(actual, gc.Equals, time.Duration(defaultInSeconds)*time.Second)
	} else {
		c.Assert(actual, gc.Equals, time.Duration(value)*time.Second)
	}
}

func (s *ConfigSuite) TestConfigAttrs(c *gc.C) {
	// Normally this is handled by testing.FakeHome
	s.PatchEnvironment(osenv.JujuLoggingConfigEnvKey, "")
	attrs := map[string]interface{}{
		"type":                      "my-type",
		"name":                      "my-name",
		"authorized-keys":           testing.FakeAuthKeys,
		"firewall-mode":             config.FwInstance,
		"admin-secret":              "foo",
		"unknown":                   "my-unknown",
		"ca-cert":                   caCert,
		"ssl-hostname-verification": true,
		"development":               false,
		"provisioner-safe-mode":     false,
		"state-port":                1234,
		"api-port":                  4321,
		"syslog-port":               2345,
		"bootstrap-timeout":         3600,
		"bootstrap-retry-delay":     30,
		"bootstrap-addresses-delay": 10,
		"default-series":            testing.FakeDefaultSeries,
		"charm-store-auth":          "token=auth",
		"test-mode":                 false,
	}
	cfg, err := config.New(config.NoDefaults, attrs)
	c.Assert(err, gc.IsNil)

	// These attributes are added if not set.
	attrs["development"] = false
	attrs["logging-config"] = "<root>=WARNING;unit=DEBUG"
	attrs["ca-private-key"] = ""
	attrs["image-metadata-url"] = ""
	attrs["tools-metadata-url"] = ""
	attrs["tools-url"] = ""
	attrs["image-stream"] = ""
	attrs["proxy-ssh"] = false
	attrs["lxc-clone-aufs"] = false

	// Default firewall mode is instance
	attrs["firewall-mode"] = string(config.FwInstance)
	c.Assert(cfg.AllAttrs(), jc.DeepEquals, attrs)
	c.Assert(cfg.UnknownAttrs(), jc.DeepEquals, map[string]interface{}{"unknown": "my-unknown"})

	newcfg, err := cfg.Apply(map[string]interface{}{
		"name":        "new-name",
		"new-unknown": "my-new-unknown",
	})
	c.Assert(err, gc.IsNil)

	attrs["name"] = "new-name"
	attrs["new-unknown"] = "my-new-unknown"
	c.Assert(newcfg.AllAttrs(), jc.DeepEquals, attrs)
}

type validationTest struct {
	about string
	new   testing.Attrs
	old   testing.Attrs
	err   string
}

var validationTests = []validationTest{{
	about: "Can't change the type",
	new:   testing.Attrs{"type": "new-type"},
	err:   `cannot change type from "my-type" to "new-type"`,
}, {
	about: "Can't change the name",
	new:   testing.Attrs{"name": "new-name"},
	err:   `cannot change name from "my-name" to "new-name"`,
}, {
	about: "Can set agent version",
	new:   testing.Attrs{"agent-version": "1.9.13"},
}, {
	about: "Can change agent version",
	old:   testing.Attrs{"agent-version": "1.9.13"},
	new:   testing.Attrs{"agent-version": "1.9.27"},
}, {
	about: "Can't clear agent version",
	old:   testing.Attrs{"agent-version": "1.9.27"},
	err:   `cannot clear agent-version`,
}, {
	about: "Can't change the firewall-mode",
	old:   testing.Attrs{"firewall-mode": config.FwGlobal},
	new:   testing.Attrs{"firewall-mode": config.FwInstance},
	err:   `cannot change firewall-mode from "global" to "instance"`,
}, {
	about: "Cannot change the state-port",
	old:   testing.Attrs{"state-port": config.DefaultStatePort},
	new:   testing.Attrs{"state-port": 42},
	err:   `cannot change state-port from 37017 to 42`,
}, {
	about: "Cannot change the api-port",
	old:   testing.Attrs{"api-port": config.DefaultAPIPort},
	new:   testing.Attrs{"api-port": 42},
	err:   `cannot change api-port from 17070 to 42`,
}, {
	about: "Can change the state-port from explicit-default to implicit-default",
	old:   testing.Attrs{"state-port": config.DefaultStatePort},
}, {
	about: "Can change the api-port from explicit-default to implicit-default",
	old:   testing.Attrs{"api-port": config.DefaultAPIPort},
}, {
	about: "Can change the state-port from implicit-default to explicit-default",
	new:   testing.Attrs{"state-port": config.DefaultStatePort},
}, {
	about: "Can change the api-port from implicit-default to explicit-default",
	new:   testing.Attrs{"api-port": config.DefaultAPIPort},
}, {
	about: "Cannot change the state-port from implicit-default to different value",
	new:   testing.Attrs{"state-port": 42},
	err:   `cannot change state-port from 37017 to 42`,
}, {
	about: "Cannot change the api-port from implicit-default to different value",
	new:   testing.Attrs{"api-port": 42},
	err:   `cannot change api-port from 17070 to 42`,
}, {
	about: "Cannot change the bootstrap-timeout from implicit-default to different value",
	new:   testing.Attrs{"bootstrap-timeout": 5},
	err:   `cannot change bootstrap-timeout from 600 to 5`,
}, {
	about: "Cannot change lxc-clone",
	old:   testing.Attrs{"lxc-clone": false},
	new:   testing.Attrs{"lxc-clone": true},
	err:   `cannot change lxc-clone from false to true`,
}, {
	about: "Cannot change lxc-clone-aufs",
	old:   testing.Attrs{"lxc-clone-aufs": false},
	new:   testing.Attrs{"lxc-clone-aufs": true},
	err:   `cannot change lxc-clone-aufs from false to true`,
}}

func (s *ConfigSuite) TestValidateChange(c *gc.C) {
	files := []testing.TestFile{
		{".ssh/identity.pub", "identity"},
	}
	s.FakeHomeSuite.Home.AddFiles(c, files...)

	for i, test := range validationTests {
		c.Logf("test %d: %s", i, test.about)
		newConfig := newTestConfig(c, test.new)
		oldConfig := newTestConfig(c, test.old)
		err := config.Validate(newConfig, oldConfig)
		if test.err == "" {
			c.Check(err, gc.IsNil)
		} else {
			c.Check(err, gc.ErrorMatches, test.err)
		}
	}
}

func (s *ConfigSuite) addJujuFiles(c *gc.C) {
	s.FakeHomeSuite.Home.AddFiles(c, []testing.TestFile{
		{".ssh/id_rsa.pub", "rsa\n"},
		{".juju/myenv-cert.pem", caCert},
		{".juju/myenv-private-key.pem", caKey},
	}...)
}

func (s *ConfigSuite) TestValidateUnknownAttrs(c *gc.C) {
	s.addJujuFiles(c)
	cfg, err := config.New(config.UseDefaults, map[string]interface{}{
		"name":    "myenv",
		"type":    "other",
		"known":   "this",
		"unknown": "that",
	})

	// No fields: all attrs passed through.
	attrs, err := cfg.ValidateUnknownAttrs(nil, nil)
	c.Assert(err, gc.IsNil)
	c.Assert(attrs, gc.DeepEquals, map[string]interface{}{
		"known":   "this",
		"unknown": "that",
	})

	// Valid field: that and other attrs passed through.
	fields := schema.Fields{"known": schema.String()}
	attrs, err = cfg.ValidateUnknownAttrs(fields, nil)
	c.Assert(err, gc.IsNil)
	c.Assert(attrs, gc.DeepEquals, map[string]interface{}{
		"known":   "this",
		"unknown": "that",
	})

	// Default field: inserted.
	fields["default"] = schema.String()
	defaults := schema.Defaults{"default": "the other"}
	attrs, err = cfg.ValidateUnknownAttrs(fields, defaults)
	c.Assert(err, gc.IsNil)
	c.Assert(attrs, gc.DeepEquals, map[string]interface{}{
		"known":   "this",
		"unknown": "that",
		"default": "the other",
	})

	// Invalid field: failure.
	fields["known"] = schema.Int()
	_, err = cfg.ValidateUnknownAttrs(fields, defaults)
	c.Assert(err, gc.ErrorMatches, `known: expected int, got string\("this"\)`)
}

func newTestConfig(c *gc.C, explicit testing.Attrs) *config.Config {
	final := testing.Attrs{"type": "my-type", "name": "my-name"}
	for key, value := range explicit {
		final[key] = value
	}
	result, err := config.New(config.UseDefaults, final)
	c.Assert(err, gc.IsNil)
	return result
}

func (s *ConfigSuite) TestLoggingConfig(c *gc.C) {
	s.addJujuFiles(c)
	config := newTestConfig(c, testing.Attrs{
		"logging-config": "<root>=WARNING;juju=DEBUG"})
	c.Assert(config.LoggingConfig(), gc.Equals, "<root>=WARNING;juju=DEBUG;unit=DEBUG")
}

func (s *ConfigSuite) TestLoggingConfigWithUnit(c *gc.C) {
	s.addJujuFiles(c)
	config := newTestConfig(c, testing.Attrs{
		"logging-config": "<root>=WARNING;unit=INFO"})
	c.Assert(config.LoggingConfig(), gc.Equals, "<root>=WARNING;unit=INFO")
}

func (s *ConfigSuite) TestLoggingConfigFromEnvironment(c *gc.C) {
	s.addJujuFiles(c)
	s.PatchEnvironment(osenv.JujuLoggingConfigEnvKey, "<root>=INFO")

	config := newTestConfig(c, nil)
	c.Assert(config.LoggingConfig(), gc.Equals, "<root>=INFO;unit=DEBUG")
}

func (s *ConfigSuite) TestProxyValuesWithFallback(c *gc.C) {
	s.addJujuFiles(c)

	config := newTestConfig(c, testing.Attrs{
		"http-proxy":  "http://user@10.0.0.1",
		"https-proxy": "https://user@10.0.0.1",
		"ftp-proxy":   "ftp://user@10.0.0.1",
		"no-proxy":    "localhost,10.0.3.1",
	})
	c.Assert(config.HttpProxy(), gc.Equals, "http://user@10.0.0.1")
	c.Assert(config.AptHttpProxy(), gc.Equals, "http://user@10.0.0.1")
	c.Assert(config.HttpsProxy(), gc.Equals, "https://user@10.0.0.1")
	c.Assert(config.AptHttpsProxy(), gc.Equals, "https://user@10.0.0.1")
	c.Assert(config.FtpProxy(), gc.Equals, "ftp://user@10.0.0.1")
	c.Assert(config.AptFtpProxy(), gc.Equals, "ftp://user@10.0.0.1")
	c.Assert(config.NoProxy(), gc.Equals, "localhost,10.0.3.1")
}

func (s *ConfigSuite) TestProxyValues(c *gc.C) {
	s.addJujuFiles(c)
	config := newTestConfig(c, testing.Attrs{
		"http-proxy":      "http://user@10.0.0.1",
		"https-proxy":     "https://user@10.0.0.1",
		"ftp-proxy":       "ftp://user@10.0.0.1",
		"apt-http-proxy":  "http://user@10.0.0.2",
		"apt-https-proxy": "https://user@10.0.0.2",
		"apt-ftp-proxy":   "ftp://user@10.0.0.2",
	})
	c.Assert(config.HttpProxy(), gc.Equals, "http://user@10.0.0.1")
	c.Assert(config.AptHttpProxy(), gc.Equals, "http://user@10.0.0.2")
	c.Assert(config.HttpsProxy(), gc.Equals, "https://user@10.0.0.1")
	c.Assert(config.AptHttpsProxy(), gc.Equals, "https://user@10.0.0.2")
	c.Assert(config.FtpProxy(), gc.Equals, "ftp://user@10.0.0.1")
	c.Assert(config.AptFtpProxy(), gc.Equals, "ftp://user@10.0.0.2")
}

func (s *ConfigSuite) TestProxyValuesNotSet(c *gc.C) {
	s.addJujuFiles(c)
	config := newTestConfig(c, testing.Attrs{})
	c.Assert(config.HttpProxy(), gc.Equals, "")
	c.Assert(config.AptHttpProxy(), gc.Equals, "")
	c.Assert(config.HttpsProxy(), gc.Equals, "")
	c.Assert(config.AptHttpsProxy(), gc.Equals, "")
	c.Assert(config.FtpProxy(), gc.Equals, "")
	c.Assert(config.AptFtpProxy(), gc.Equals, "")
	c.Assert(config.NoProxy(), gc.Equals, "")
}

func (s *ConfigSuite) TestProxyConfigMap(c *gc.C) {
	s.addJujuFiles(c)
	cfg := newTestConfig(c, testing.Attrs{})
	proxy := osenv.ProxySettings{
		Http:    "http proxy",
		Https:   "https proxy",
		Ftp:     "ftp proxy",
		NoProxy: "no proxy",
	}
	cfg, err := cfg.Apply(config.ProxyConfigMap(proxy))
	c.Assert(err, gc.IsNil)
	c.Assert(cfg.ProxySettings(), gc.DeepEquals, proxy)
	// Apt proxy and proxy differ by the content of the no-proxy values.
	proxy.NoProxy = ""
	c.Assert(cfg.AptProxySettings(), gc.DeepEquals, proxy)
}

func (s *ConfigSuite) TestAptProxyConfigMap(c *gc.C) {
	s.addJujuFiles(c)
	cfg := newTestConfig(c, testing.Attrs{})
	proxy := osenv.ProxySettings{
		Http:  "http proxy",
		Https: "https proxy",
		Ftp:   "ftp proxy",
	}
	cfg, err := cfg.Apply(config.AptProxyConfigMap(proxy))
	c.Assert(err, gc.IsNil)
	// The default proxy settings should still be empty.
	c.Assert(cfg.ProxySettings(), gc.DeepEquals, osenv.ProxySettings{})
	c.Assert(cfg.AptProxySettings(), gc.DeepEquals, proxy)
}

func (s *ConfigSuite) TestGenerateStateServerCertAndKey(c *gc.C) {
	// Add a cert.
	s.FakeHomeSuite.Home.AddFiles(c, testing.TestFile{".ssh/id_rsa.pub", "rsa\n"})

	for _, test := range []struct {
		configValues map[string]interface{}
		errMatch     string
	}{{
		configValues: map[string]interface{}{
			"name": "test-no-certs",
			"type": "dummy",
		},
		errMatch: "environment configuration has no ca-cert",
	}, {
		configValues: map[string]interface{}{
			"name":    "test-no-certs",
			"type":    "dummy",
			"ca-cert": testing.CACert,
		},
		errMatch: "environment configuration has no ca-private-key",
	}, {
		configValues: map[string]interface{}{
			"name":           "test-no-certs",
			"type":           "dummy",
			"ca-cert":        testing.CACert,
			"ca-private-key": testing.CAKey,
		},
	}} {
		cfg, err := config.New(config.UseDefaults, test.configValues)
		c.Assert(err, gc.IsNil)
		certPEM, keyPEM, err := cfg.GenerateStateServerCertAndKey()
		if test.errMatch == "" {
			c.Assert(err, gc.IsNil)

			_, _, err = cert.ParseCertAndKey(certPEM, keyPEM)
			c.Check(err, gc.IsNil)

			err = cert.Verify(certPEM, testing.CACert, time.Now())
			c.Assert(err, gc.IsNil)
			err = cert.Verify(certPEM, testing.CACert, time.Now().AddDate(9, 0, 0))
			c.Assert(err, gc.IsNil)
			err = cert.Verify(certPEM, testing.CACert, time.Now().AddDate(10, 0, 1))
			c.Assert(err, gc.NotNil)
		} else {
			c.Assert(err, gc.ErrorMatches, test.errMatch)
			c.Assert(certPEM, gc.Equals, "")
			c.Assert(keyPEM, gc.Equals, "")
		}
	}
}

var caCert = `
-----BEGIN CERTIFICATE-----
MIIBjDCCATigAwIBAgIBADALBgkqhkiG9w0BAQUwHjENMAsGA1UEChMEanVqdTEN
MAsGA1UEAxMEcm9vdDAeFw0xMjExMDkxNjQwMjhaFw0yMjExMDkxNjQ1MjhaMB4x
DTALBgNVBAoTBGp1anUxDTALBgNVBAMTBHJvb3QwWTALBgkqhkiG9w0BAQEDSgAw
RwJAduA1Gnb2VJLxNGfG4St0Qy48Y3q5Z5HheGtTGmti/FjlvQvScCFGCnJG7fKA
Knd7ia3vWg7lxYkIvMPVP88LAQIDAQABo2YwZDAOBgNVHQ8BAf8EBAMCAKQwEgYD
VR0TAQH/BAgwBgEB/wIBATAdBgNVHQ4EFgQUlvKX8vwp0o+VdhdhoA9O6KlOm00w
HwYDVR0jBBgwFoAUlvKX8vwp0o+VdhdhoA9O6KlOm00wCwYJKoZIhvcNAQEFA0EA
LlNpevtFr8gngjAFFAO/FXc7KiZcCrA5rBfb/rEy297lIqmKt5++aVbLEPyxCIFC
r71Sj63TUTFWtRZAxvn9qQ==
-----END CERTIFICATE-----
`[1:]

var caKey = `
-----BEGIN RSA PRIVATE KEY-----
MIIBOQIBAAJAduA1Gnb2VJLxNGfG4St0Qy48Y3q5Z5HheGtTGmti/FjlvQvScCFG
CnJG7fKAKnd7ia3vWg7lxYkIvMPVP88LAQIDAQABAkEAsFOdMSYn+AcF1M/iBfjo
uQWJ+Zz+CgwuvumjGNsUtmwxjA+hh0fCn0Ah2nAt4Ma81vKOKOdQ8W6bapvsVDH0
6QIhAJOkLmEKm4H5POQV7qunRbRsLbft/n/SHlOBz165WFvPAiEAzh9fMf70std1
sVCHJRQWKK+vw3oaEvPKvkPiV5ui0C8CIGNsvybuo8ald5IKCw5huRlFeIxSo36k
m3OVCXc6zfwVAiBnTUe7WcivPNZqOC6TAZ8dYvdWo4Ifz3jjpEfymjid1wIgBIJv
ERPyv2NQqIFQZIyzUP7LVRIWfpFFOo9/Ww/7s5Y=
-----END RSA PRIVATE KEY-----
`[1:]

var caCert2 = `
-----BEGIN CERTIFICATE-----
MIIBjTCCATmgAwIBAgIBADALBgkqhkiG9w0BAQUwHjENMAsGA1UEChMEanVqdTEN
MAsGA1UEAxMEcm9vdDAeFw0xMjExMDkxNjQxMDhaFw0yMjExMDkxNjQ2MDhaMB4x
DTALBgNVBAoTBGp1anUxDTALBgNVBAMTBHJvb3QwWjALBgkqhkiG9w0BAQEDSwAw
SAJBAJkSWRrr81y8pY4dbNgt+8miSKg4z6glp2KO2NnxxAhyyNtQHKvC+fJALJj+
C2NhuvOv9xImxOl3Hg8fFPCXCtcCAwEAAaNmMGQwDgYDVR0PAQH/BAQDAgCkMBIG
A1UdEwEB/wQIMAYBAf8CAQEwHQYDVR0OBBYEFOsX/ZCqKzWCAaTTVcWsWKT5Msow
MB8GA1UdIwQYMBaAFOsX/ZCqKzWCAaTTVcWsWKT5MsowMAsGCSqGSIb3DQEBBQNB
AAVV57jetEzJQnjgBzhvx/UwauFn78jGhXfV5BrQmxIb4SF4DgSCFstPwUQOAr8h
XXzJqBQH92KYmp+y3YXDoMQ=
-----END CERTIFICATE-----
`[1:]

var caKey2 = `
-----BEGIN RSA PRIVATE KEY-----
MIIBOQIBAAJBAJkSWRrr81y8pY4dbNgt+8miSKg4z6glp2KO2NnxxAhyyNtQHKvC
+fJALJj+C2NhuvOv9xImxOl3Hg8fFPCXCtcCAwEAAQJATQNzO11NQvJS5U6eraFt
FgSFQ8XZjILtVWQDbJv8AjdbEgKMHEy33icsAKIUAx8jL9kjq6K9kTdAKXZi9grF
UQIhAPD7jccIDUVm785E5eR9eisq0+xpgUIa24Jkn8cAlst5AiEAopxVFl1auer3
GP2In3pjdL4ydzU/gcRcYisoJqwHpM8CIHtqmaXBPeq5WT9ukb5/dL3+5SJCtmxA
jQMuvZWRe6khAiBvMztYtPSDKXRbCZ4xeQ+kWSDHtok8Y5zNoTeu4nvDrwIgb3Al
fikzPveC5g6S6OvEQmyDz59tYBubm2XHgvxqww0=
-----END RSA PRIVATE KEY-----
`[1:]

var caCert3 = `
-----BEGIN CERTIFICATE-----
MIIBjTCCATmgAwIBAgIBADALBgkqhkiG9w0BAQUwHjENMAsGA1UEChMEanVqdTEN
MAsGA1UEAxMEcm9vdDAeFw0xMjExMDkxNjQxMjlaFw0yMjExMDkxNjQ2MjlaMB4x
DTALBgNVBAoTBGp1anUxDTALBgNVBAMTBHJvb3QwWjALBgkqhkiG9w0BAQEDSwAw
SAJBAIW7CbHFJivvV9V6mO8AGzJS9lqjUf6MdEPsdF6wx2Cpzr/lSFIggCwRA138
9MuFxflxb/3U8Nq+rd8rVtTgFMECAwEAAaNmMGQwDgYDVR0PAQH/BAQDAgCkMBIG
A1UdEwEB/wQIMAYBAf8CAQEwHQYDVR0OBBYEFJafrxqByMN9BwGfcmuF0Lw/1QII
MB8GA1UdIwQYMBaAFJafrxqByMN9BwGfcmuF0Lw/1QIIMAsGCSqGSIb3DQEBBQNB
AHq3vqNhxya3s33DlQfSj9whsnqM0Nm+u8mBX/T76TF5rV7+B33XmYzSyfA3yBi/
zHaUR/dbHuiNTO+KXs3/+Y4=
-----END CERTIFICATE-----
`[1:]

var caKey3 = `
-----BEGIN RSA PRIVATE KEY-----
MIIBOgIBAAJBAIW7CbHFJivvV9V6mO8AGzJS9lqjUf6MdEPsdF6wx2Cpzr/lSFIg
gCwRA1389MuFxflxb/3U8Nq+rd8rVtTgFMECAwEAAQJAaivPi4qJPrJb2onl50H/
VZnWKqmljGF4YQDWduMEt7GTPk+76x9SpO7W4gfY490Ivd9DEXfbr/KZqhwWikNw
LQIhALlLfRXLF2ZfToMfB1v1v+jith5onAu24O68mkdRc5PLAiEAuMJ/6U07hggr
Ckf9OT93wh84DK66h780HJ/FUHKcoCMCIDsPZaJBpoa50BOZG0ZjcTTwti3BGCPf
uZg+w0oCGz27AiEAsUCYKqEXy/ymHhT2kSecozYENdajyXvcaOG3EPkD3nUCICOP
zatzs7c/4mx4a0JBG6Za0oEPUcm2I34is50KSohz
-----END RSA PRIVATE KEY-----
`[1:]

var invalidCAKey = `
-----BEGIN RSA PRIVATE KEY-----
MIIBOgIBAAJAZabKgKInuOxj5vDWLwHHQtK3/45KB+32D15w94Nt83BmuGxo90lw
-----END RSA PRIVATE KEY-----
`[1:]

var invalidCACert = `
-----BEGIN CERTIFICATE-----
MIIBOgIBAAJAZabKgKInuOxj5vDWLwHHQtK3/45KB+32D15w94Nt83BmuGxo90lw
-----END CERTIFICATE-----
`[1:]
