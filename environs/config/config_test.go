// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package config_test

import (
	"fmt"
	"strings"
	stdtesting "testing"
	"time"

	"github.com/juju/loggo"
	"github.com/juju/schema"
	gitjujutesting "github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/utils/proxy"
	"github.com/juju/utils/series"
	"github.com/juju/version"
	gc "gopkg.in/check.v1"
	"gopkg.in/juju/charmrepo.v2-unstable"
	"gopkg.in/juju/environschema.v1"

	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/juju/osenv"
	"github.com/juju/juju/testing"
)

func Test(t *stdtesting.T) {
	gc.TestingT(t)
}

type ConfigSuite struct {
	testing.FakeJujuXDGDataHomeSuite
	home string
}

var _ = gc.Suite(&ConfigSuite{})

func (s *ConfigSuite) SetUpTest(c *gc.C) {
	s.FakeJujuXDGDataHomeSuite.SetUpTest(c)
	// Make sure that the defaults are used, which
	// is <root>=WARNING
	loggo.DefaultContext().ResetLoggerLevels()
}

// sampleConfig holds a configuration with all required
// attributes set.
var sampleConfig = testing.Attrs{
	"type":                       "my-type",
	"name":                       "my-name",
	"uuid":                       testing.ModelTag.Id(),
	"authorized-keys":            testing.FakeAuthKeys,
	"firewall-mode":              config.FwInstance,
	"unknown":                    "my-unknown",
	"ssl-hostname-verification":  true,
	"development":                false,
	"default-series":             series.LatestLts(),
	"disable-network-management": false,
	"ignore-machine-addresses":   false,
	"automatically-retry-hooks":  true,
	"proxy-ssh":                  false,
	"resource-tags":              []string{},
}

type configTest struct {
	about       string
	useDefaults config.Defaulting
	attrs       testing.Attrs
	expected    testing.Attrs
	err         string
}

var testResourceTags = []string{"a=b", "c=", "d=e"}
var testResourceTagsMap = map[string]string{
	"a": "b", "c": "", "d": "e",
}

var minimalConfigAttrs = testing.Attrs{
	"type": "my-type",
	"name": "my-name",
	"uuid": testing.ModelTag.Id(),
}

var modelNameErr = "%q is not a valid name: model names may only contain lowercase letters, digits and hyphens"

var configTests = []configTest{
	{
		about:       "The minimum good configuration",
		useDefaults: config.UseDefaults,
		attrs:       minimalConfigAttrs,
	}, {
		about:       "Agent Stream",
		useDefaults: config.UseDefaults,
		attrs: minimalConfigAttrs.Merge(testing.Attrs{
			"image-metadata-url": "image-url",
			"agent-stream":       "released",
		}),
	}, {
		about:       "Metadata URLs",
		useDefaults: config.UseDefaults,
		attrs: minimalConfigAttrs.Merge(testing.Attrs{
			"image-metadata-url": "image-url",
			"agent-metadata-url": "agent-metadata-url-value",
		}),
	}, {
		about:       "Explicit series",
		useDefaults: config.UseDefaults,
		attrs: minimalConfigAttrs.Merge(testing.Attrs{
			"default-series": "my-series",
		}),
	}, {
		about:       "Explicit logging",
		useDefaults: config.UseDefaults,
		attrs: minimalConfigAttrs.Merge(testing.Attrs{
			"logging-config": "juju=INFO",
		}),
	}, {
		about:       "Explicit authorized-keys",
		useDefaults: config.UseDefaults,
		attrs: minimalConfigAttrs.Merge(testing.Attrs{
			"authorized-keys": testing.FakeAuthKeys,
		}),
	}, {
		about:       "Specified agent version",
		useDefaults: config.UseDefaults,
		attrs: minimalConfigAttrs.Merge(testing.Attrs{
			"agent-version": "1.2.3",
		}),
	}, {
		about:       "Specified development flag",
		useDefaults: config.UseDefaults,
		attrs: minimalConfigAttrs.Merge(testing.Attrs{
			"development": true,
		}),
	}, {
		about:       "Invalid development flag",
		useDefaults: config.UseDefaults,
		attrs: minimalConfigAttrs.Merge(testing.Attrs{
			"development": "invalid",
		}),
		err: `development: expected bool, got string\("invalid"\)`,
	}, {
		about:       "Invalid disable-network-management flag",
		useDefaults: config.UseDefaults,
		attrs: minimalConfigAttrs.Merge(testing.Attrs{
			"disable-network-management": "invalid",
		}),
		err: `disable-network-management: expected bool, got string\("invalid"\)`,
	}, {
		about:       "disable-network-management off",
		useDefaults: config.UseDefaults,
		attrs: minimalConfigAttrs.Merge(testing.Attrs{
			"disable-network-management": false,
		}),
	}, {
		about:       "disable-network-management on",
		useDefaults: config.UseDefaults,
		attrs: minimalConfigAttrs.Merge(testing.Attrs{
			"disable-network-management": true,
		}),
	}, {
		about:       "Invalid ignore-machine-addresses flag",
		useDefaults: config.UseDefaults,
		attrs: minimalConfigAttrs.Merge(testing.Attrs{
			"ignore-machine-addresses": "invalid",
		}),
		err: `ignore-machine-addresses: expected bool, got string\("invalid"\)`,
	}, {
		about:       "ignore-machine-addresses off",
		useDefaults: config.UseDefaults,
		attrs: minimalConfigAttrs.Merge(testing.Attrs{
			"ignore-machine-addresses": false,
		}),
	}, {
		about:       "ignore-machine-addresses on",
		useDefaults: config.UseDefaults,
		attrs: minimalConfigAttrs.Merge(testing.Attrs{
			"ignore-machine-addresses": true,
		}),
	}, {
		about:       "set-numa-control-policy on",
		useDefaults: config.UseDefaults,
		attrs: minimalConfigAttrs.Merge(testing.Attrs{
			"set-numa-control-policy": true,
		}),
	}, {
		about:       "set-numa-control-policy off",
		useDefaults: config.UseDefaults,
		attrs: minimalConfigAttrs.Merge(testing.Attrs{
			"set-numa-control-policy": false,
		}),
	}, {
		about:       "Invalid agent version",
		useDefaults: config.UseDefaults,
		attrs: minimalConfigAttrs.Merge(testing.Attrs{
			"agent-version": "2",
		}),
		err: `invalid agent version in model configuration: "2"`,
	}, {
		about:       "Missing type",
		useDefaults: config.UseDefaults,
		attrs:       minimalConfigAttrs.Delete("type"),
		err:         "type: expected string, got nothing",
	}, {
		about:       "Empty type",
		useDefaults: config.UseDefaults,
		attrs: minimalConfigAttrs.Merge(testing.Attrs{
			"type": "",
		}),
		err: "empty type in model configuration",
	}, {
		about:       "Missing name",
		useDefaults: config.UseDefaults,
		attrs:       minimalConfigAttrs.Delete("name"),
		err:         "name: expected string, got nothing",
	}, {
		about:       "Bad name, no slash",
		useDefaults: config.UseDefaults,
		attrs: minimalConfigAttrs.Merge(testing.Attrs{
			"name": "foo/bar",
		}),
		err: fmt.Sprintf(modelNameErr, "foo/bar"),
	}, {
		about:       "Bad name, no backslash",
		useDefaults: config.UseDefaults,
		attrs: minimalConfigAttrs.Merge(testing.Attrs{
			"name": "foo\\bar",
		}),
		// Double escape to keep the safe quote in the format string happy
		err: fmt.Sprintf(modelNameErr, "foo\\\\bar"),
	}, {
		about:       "Bad name, no space",
		useDefaults: config.UseDefaults,
		attrs: minimalConfigAttrs.Merge(testing.Attrs{
			"name": "foo bar",
		}),
		err: fmt.Sprintf(modelNameErr, "foo bar"),
	}, {
		about:       "Bad name, no capital",
		useDefaults: config.UseDefaults,
		attrs: minimalConfigAttrs.Merge(testing.Attrs{
			"name": "fooBar",
		}),
		err: fmt.Sprintf(modelNameErr, "fooBar"),
	}, {
		about:       "Empty name",
		useDefaults: config.UseDefaults,
		attrs: minimalConfigAttrs.Merge(testing.Attrs{
			"name": "",
		}),
		err: "empty name in model configuration",
	}, {
		about:       "Default firewall mode",
		useDefaults: config.UseDefaults,
		attrs:       minimalConfigAttrs,
	}, {
		about:       "Empty firewall mode",
		useDefaults: config.UseDefaults,
		attrs: minimalConfigAttrs.Merge(testing.Attrs{
			"firewall-mode": "",
		}),
		err: `firewall-mode: expected one of \[instance global none\], got ""`,
	}, {
		about:       "Instance firewall mode",
		useDefaults: config.UseDefaults,
		attrs: minimalConfigAttrs.Merge(testing.Attrs{
			"firewall-mode": config.FwInstance,
		}),
	}, {
		about:       "Global firewall mode",
		useDefaults: config.UseDefaults,
		attrs: minimalConfigAttrs.Merge(testing.Attrs{
			"firewall-mode": config.FwGlobal,
		}),
	}, {
		about:       "None firewall mode",
		useDefaults: config.UseDefaults,
		attrs: minimalConfigAttrs.Merge(testing.Attrs{
			"firewall-mode": config.FwNone,
		}),
	}, {
		about:       "Illegal firewall mode",
		useDefaults: config.UseDefaults,
		attrs: minimalConfigAttrs.Merge(testing.Attrs{
			"firewall-mode": "illegal",
		}),
		err: `firewall-mode: expected one of \[instance global none\], got "illegal"`,
	}, {
		about:       "ssl-hostname-verification off",
		useDefaults: config.UseDefaults,
		attrs: minimalConfigAttrs.Merge(testing.Attrs{
			"ssl-hostname-verification": false,
		}),
	}, {
		about:       "ssl-hostname-verification incorrect",
		useDefaults: config.UseDefaults,
		attrs: minimalConfigAttrs.Merge(testing.Attrs{
			"ssl-hostname-verification": "yes please",
		}),
		err: `ssl-hostname-verification: expected bool, got string\("yes please"\)`,
	}, {
		about: fmt.Sprintf(
			"%s: %s",
			"provisioner-harvest-mode",
			config.HarvestAll.String(),
		),
		useDefaults: config.UseDefaults,
		attrs: minimalConfigAttrs.Merge(testing.Attrs{
			"provisioner-harvest-mode": config.HarvestAll.String(),
		}),
	}, {
		about: fmt.Sprintf(
			"%s: %s",
			"provisioner-harvest-mode",
			config.HarvestDestroyed.String(),
		),
		useDefaults: config.UseDefaults,
		attrs: minimalConfigAttrs.Merge(testing.Attrs{
			"provisioner-harvest-mode": config.HarvestDestroyed.String(),
		}),
	}, {
		about: fmt.Sprintf(
			"%s: %s",
			"provisioner-harvest-mode",
			config.HarvestUnknown.String(),
		),
		useDefaults: config.UseDefaults,
		attrs: minimalConfigAttrs.Merge(testing.Attrs{
			"provisioner-harvest-mode": config.HarvestUnknown.String(),
		}),
	}, {
		about: fmt.Sprintf(
			"%s: %s",
			"provisioner-harvest-mode",
			config.HarvestNone.String(),
		),
		useDefaults: config.UseDefaults,
		attrs: minimalConfigAttrs.Merge(testing.Attrs{
			"provisioner-harvest-mode": config.HarvestNone.String(),
		}),
	}, {
		about:       "provisioner-harvest-mode: incorrect",
		useDefaults: config.UseDefaults,
		attrs: minimalConfigAttrs.Merge(testing.Attrs{
			"provisioner-harvest-mode": "yes please",
		}),
		err: `provisioner-harvest-mode: expected one of \[all none unknown destroyed], got "yes please"`,
	}, {
		about:       "default image stream",
		useDefaults: config.UseDefaults,
		attrs:       minimalConfigAttrs,
	}, {
		about:       "explicit image stream",
		useDefaults: config.UseDefaults,
		attrs: minimalConfigAttrs.Merge(testing.Attrs{
			"image-stream": "daily",
		}),
	}, {
		about:       "explicit tools stream",
		useDefaults: config.UseDefaults,
		attrs: minimalConfigAttrs.Merge(testing.Attrs{
			"agent-stream": "proposed",
		}),
	}, {
		about:       "Invalid logging configuration",
		useDefaults: config.UseDefaults,
		attrs: minimalConfigAttrs.Merge(testing.Attrs{
			"logging-config": "foo=bar",
		}),
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
		about:       "Config settings from juju actual installation",
		useDefaults: config.NoDefaults,
		attrs: map[string]interface{}{
			"name":                       "sample",
			"development":                false,
			"ssl-hostname-verification":  true,
			"authorized-keys":            "ssh-rsa mykeys rog@rog-x220\n",
			"region":                     "us-east-1",
			"default-series":             "precise",
			"secret-key":                 "a-secret-key",
			"access-key":                 "an-access-key",
			"agent-version":              "1.13.2",
			"firewall-mode":              "instance",
			"disable-network-management": false,
			"ignore-machine-addresses":   false,
			"automatically-retry-hooks":  true,
			"proxy-ssh":                  false,
			"resource-tags":              []string{},
			"type":                       "ec2",
			"uuid":                       testing.ModelTag.Id(),
		},
	}, {
		about:       "TestMode flag specified",
		useDefaults: config.UseDefaults,
		attrs: minimalConfigAttrs.Merge(testing.Attrs{
			"test-mode": true,
		}),
	}, {
		about:       "valid uuid",
		useDefaults: config.UseDefaults,
		attrs: minimalConfigAttrs.Merge(testing.Attrs{
			"uuid": "dcfbdb4a-bca2-49ad-aa7c-f011424e0fe4",
		}),
	}, {
		about:       "invalid uuid 1",
		useDefaults: config.UseDefaults,
		attrs: minimalConfigAttrs.Merge(testing.Attrs{
			"uuid": "dcfbdb4abca249adaa7cf011424e0fe4",
		}),
		err: `uuid: expected UUID, got string\("dcfbdb4abca249adaa7cf011424e0fe4"\)`,
	}, {
		about:       "invalid uuid 2",
		useDefaults: config.UseDefaults,
		attrs: minimalConfigAttrs.Merge(testing.Attrs{
			"uuid": "uuid",
		}),
		err: `uuid: expected UUID, got string\("uuid"\)`,
	}, {
		about:       "blank uuid",
		useDefaults: config.UseDefaults,
		attrs: minimalConfigAttrs.Merge(testing.Attrs{
			"uuid": "",
		}),
		err: `empty uuid in model configuration`,
	},
	{
		about:       "Explicit apt-mirror",
		useDefaults: config.UseDefaults,
		attrs: minimalConfigAttrs.Merge(testing.Attrs{
			"apt-mirror": "http://my.archive.ubuntu.com",
		}),
	},
	{
		about:       "Resource tags as space-separated string",
		useDefaults: config.UseDefaults,
		attrs: minimalConfigAttrs.Merge(testing.Attrs{
			"resource-tags": strings.Join(testResourceTags, " "),
		}),
	},
	{
		about:       "Resource tags as list of strings",
		useDefaults: config.UseDefaults,
		attrs: minimalConfigAttrs.Merge(testing.Attrs{
			"resource-tags": testResourceTags,
		}),
	},
	{
		about:       "Resource tags contains non-keyvalues",
		useDefaults: config.UseDefaults,
		attrs: minimalConfigAttrs.Merge(testing.Attrs{
			"resource-tags": []string{"a"},
		}),
		err: `resource-tags: expected "key=value", got "a"`,
	}, {
		about:       "Invalid syslog ca cert format",
		useDefaults: config.UseDefaults,
		attrs: minimalConfigAttrs.Merge(testing.Attrs{
			"type":               "my-type",
			"name":               "my-name",
			"logforward-enabled": true,
			"syslog-host":        "localhost:1234",
			"syslog-ca-cert":     "abc",
			"syslog-client-cert": caCert,
			"syslog-client-key":  caKey,
		}),
		err: `invalid syslog forwarding config: validating TLS config: parsing CA certificate: no certificates found`,
	}, {
		about:       "Invalid syslog ca cert",
		useDefaults: config.UseDefaults,
		attrs: minimalConfigAttrs.Merge(testing.Attrs{
			"type":               "my-type",
			"name":               "my-name",
			"logforward-enabled": true,
			"syslog-host":        "localhost:1234",
			"syslog-ca-cert":     invalidCACert,
			"syslog-client-cert": caCert,
			"syslog-client-key":  caKey,
		}),
		err: `invalid syslog forwarding config: validating TLS config: parsing CA certificate: asn1: syntax error: data truncated`,
	}, {
		about:       "invalid syslog cert",
		useDefaults: config.UseDefaults,
		attrs: minimalConfigAttrs.Merge(testing.Attrs{
			"logforward-enabled": true,
			"syslog-host":        "10.0.0.1:12345",
			"syslog-ca-cert":     caCert,
			"syslog-client-cert": invalidCACert,
			"syslog-client-key":  caKey,
		}),
		err: `invalid syslog forwarding config: validating TLS config: parsing client key pair: asn1: syntax error: data truncated`,
	}, {
		about:       "invalid syslog key",
		useDefaults: config.UseDefaults,
		attrs: minimalConfigAttrs.Merge(testing.Attrs{
			"logforward-enabled": true,
			"syslog-host":        "10.0.0.1:12345",
			"syslog-ca-cert":     caCert,
			"syslog-client-cert": caCert,
			"syslog-client-key":  invalidCAKey,
		}),
		err: `invalid syslog forwarding config: validating TLS config: parsing client key pair: (crypto/)?tls: failed to parse private key`,
	}, {
		about:       "Mismatched syslog cert and key",
		useDefaults: config.UseDefaults,
		attrs: minimalConfigAttrs.Merge(testing.Attrs{
			"logforward-enabled": true,
			"syslog-host":        "10.0.0.1:12345",
			"syslog-ca-cert":     caCert,
			"syslog-client-cert": caCert,
			"syslog-client-key":  caKey2,
		}),
		err: `invalid syslog forwarding config: validating TLS config: parsing client key pair: (crypto/)?tls: private key does not match public key`,
	}, {
		about:       "transmit-vendor-metrics asserted with default value",
		useDefaults: config.UseDefaults,
		attrs: minimalConfigAttrs.Merge(testing.Attrs{
			"transmit-vendor-metrics": true,
		}),
	}, {
		about:       "transmit-vendor-metrics asserted false",
		useDefaults: config.UseDefaults,
		attrs: minimalConfigAttrs.Merge(testing.Attrs{
			"transmit-vendor-metrics": false,
		}),
	}, {
		about:       "Valid syslog config values",
		useDefaults: config.UseDefaults,
		attrs: minimalConfigAttrs.Merge(testing.Attrs{
			"type":               "my-type",
			"name":               "my-name",
			"logforward-enabled": true,
			"syslog-host":        "localhost:1234",
			"syslog-ca-cert":     testing.CACert,
			"syslog-client-cert": testing.ServerCert,
			"syslog-client-key":  testing.ServerKey,
		}),
	},
}

type testFile struct {
	name, data string
}

func (s *ConfigSuite) TestConfig(c *gc.C) {
	files := []gitjujutesting.TestFile{
		{".ssh/id_dsa.pub", "dsa"},
		{".ssh/id_rsa.pub", "rsa\n"},
		{".ssh/identity.pub", "identity"},
		{".ssh/authorized_keys", "auth0\n# first\nauth1\n\n"},
		{".ssh/authorized_keys2", "auth2\nauth3\n"},
	}
	s.FakeHomeSuite.Home.AddFiles(c, files...)
	for i, test := range configTests {
		c.Logf("test %d. %s", i, test.about)
		test.check(c, s.FakeHomeSuite.Home)
	}
}

func (test configTest) check(c *gc.C, home *gitjujutesting.FakeHome) {
	cfg, err := config.New(test.useDefaults, test.attrs)
	if test.err != "" {
		c.Check(cfg, gc.IsNil)
		c.Assert(err, gc.ErrorMatches, test.err)
		return
	}
	c.Assert(err, jc.ErrorIsNil)

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

	if expected, ok := test.attrs["uuid"]; ok {
		c.Assert(cfg.UUID(), gc.Equals, expected)
	}

	dev, _ := test.attrs["development"].(bool)
	c.Assert(cfg.Development(), gc.Equals, dev)

	testmode, _ := test.attrs["test-mode"].(bool)
	c.Assert(cfg.TestMode(), gc.Equals, testmode)

	seriesAttr, _ := test.attrs["default-series"].(string)
	defaultSeries, ok := cfg.DefaultSeries()
	c.Assert(ok, jc.IsTrue)
	if seriesAttr != "" {
		c.Assert(defaultSeries, gc.Equals, seriesAttr)
	} else {
		c.Assert(defaultSeries, gc.Equals, series.LatestLts())
	}

	if m, _ := test.attrs["firewall-mode"].(string); m != "" {
		c.Assert(cfg.FirewallMode(), gc.Equals, m)
	}

	keys, _ := test.attrs["authorized-keys"].(string)
	c.Assert(cfg.AuthorizedKeys(), gc.Equals, keys)

	lfCfg, hasLogCfg := cfg.LogFwdSyslog()
	if v, ok := test.attrs["logforward-enabled"].(bool); ok {
		c.Assert(hasLogCfg, jc.IsTrue)
		c.Assert(lfCfg.Enabled, gc.Equals, v)
	}
	if v, ok := test.attrs["syslog-ca-cert"].(string); v != "" {
		c.Assert(hasLogCfg, jc.IsTrue)
		c.Assert(lfCfg.CACert, gc.Equals, v)
	} else if ok {
		c.Assert(hasLogCfg, jc.IsTrue)
		c.Check(lfCfg.CACert, gc.Equals, "")
	}
	if v, ok := test.attrs["syslog-client-cert"].(string); v != "" {
		c.Assert(hasLogCfg, jc.IsTrue)
		c.Assert(lfCfg.ClientCert, gc.Equals, v)
	} else if ok {
		c.Assert(hasLogCfg, jc.IsTrue)
		c.Check(lfCfg.ClientCert, gc.Equals, "")
	}
	if v, ok := test.attrs["syslog-client-key"].(string); v != "" {
		c.Assert(hasLogCfg, jc.IsTrue)
		c.Assert(lfCfg.ClientKey, gc.Equals, v)
	} else if ok {
		c.Assert(hasLogCfg, jc.IsTrue)
		c.Check(lfCfg.ClientKey, gc.Equals, "")
	}

	if v, ok := test.attrs["ssl-hostname-verification"]; ok {
		c.Assert(cfg.SSLHostnameVerification(), gc.Equals, v)
	}

	if v, ok := test.attrs["provisioner-harvest-mode"]; ok {
		hvstMeth, err := config.ParseHarvestMode(v.(string))
		c.Assert(err, jc.ErrorIsNil)
		c.Assert(cfg.ProvisionerHarvestMode(), gc.Equals, hvstMeth)
	} else {
		c.Assert(cfg.ProvisionerHarvestMode(), gc.Equals, config.HarvestDestroyed)
	}

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

	agentURL, urlPresent := cfg.AgentMetadataURL()
	expectedToolsURLValue := test.attrs["agent-metadata-url"]
	if urlPresent {
		c.Assert(agentURL, gc.Equals, expectedToolsURLValue)
	} else {
		c.Assert(agentURL, gc.Equals, "")
	}

	// assertions for deprecated tools-stream attribute used with new agent-stream
	agentStreamValue := cfg.AgentStream()
	oldTstToolsStreamAttr, oldTstOk := test.attrs["tools-stream"]
	expectedAgentStreamAttr := test.attrs["agent-stream"]

	// When no agent-stream provided, look for tools-stream
	if expectedAgentStreamAttr == nil {
		if oldTstOk {
			expectedAgentStreamAttr = oldTstToolsStreamAttr
		} else {
			// If it's still nil, then hard-coded default is used
			expectedAgentStreamAttr = "released"
		}
	}
	c.Assert(agentStreamValue, gc.Equals, expectedAgentStreamAttr)

	resourceTags, cfgHasResourceTags := cfg.ResourceTags()
	c.Assert(cfgHasResourceTags, jc.IsTrue)
	if tags, ok := test.attrs["resource-tags"]; ok {
		switch tags := tags.(type) {
		case []string:
			if len(tags) > 0 {
				c.Assert(resourceTags, jc.DeepEquals, testResourceTagsMap)
			}
		case string:
			if tags != "" {
				c.Assert(resourceTags, jc.DeepEquals, testResourceTagsMap)
			}
		}
	} else {
		c.Assert(resourceTags, gc.HasLen, 0)
	}

	xmit := cfg.TransmitVendorMetrics()
	expectedXmit, xmitAsserted := test.attrs["transmit-vendor-metrics"]
	if xmitAsserted {
		c.Check(xmit, gc.Equals, expectedXmit)
	} else {
		c.Check(xmit, jc.IsTrue)
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
	// Normally this is handled by gitjujutesting.FakeHome
	s.PatchEnvironment(osenv.JujuLoggingConfigEnvKey, "")
	attrs := map[string]interface{}{
		"type":                       "my-type",
		"name":                       "my-name",
		"uuid":                       "90168e4c-2f10-4e9c-83c2-1fb55a58e5a9",
		"authorized-keys":            testing.FakeAuthKeys,
		"firewall-mode":              config.FwInstance,
		"unknown":                    "my-unknown",
		"ssl-hostname-verification":  true,
		"default-series":             series.LatestLts(),
		"disable-network-management": false,
		"ignore-machine-addresses":   false,
		"automatically-retry-hooks":  true,
		"proxy-ssh":                  false,
		"development":                false,
		"test-mode":                  false,
	}
	cfg, err := config.New(config.NoDefaults, attrs)
	c.Assert(err, jc.ErrorIsNil)

	// These attributes are added if not set.
	attrs["logging-config"] = "<root>=WARNING;unit=DEBUG"

	// Default firewall mode is instance
	attrs["firewall-mode"] = string(config.FwInstance)
	c.Assert(cfg.AllAttrs(), jc.DeepEquals, attrs)
	c.Assert(cfg.UnknownAttrs(), jc.DeepEquals, map[string]interface{}{"unknown": "my-unknown"})

	// Verify that default provisioner-harvest-mode is good.
	c.Assert(cfg.ProvisionerHarvestMode(), gc.Equals, config.HarvestDestroyed)

	newcfg, err := cfg.Apply(map[string]interface{}{
		"name":        "new-name",
		"uuid":        "6216dfc3-6e82-408f-9f74-8565e63e6158",
		"new-unknown": "my-new-unknown",
	})
	c.Assert(err, jc.ErrorIsNil)

	attrs["name"] = "new-name"
	attrs["uuid"] = "6216dfc3-6e82-408f-9f74-8565e63e6158"
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
	about: "Can't change the firewall-mode (global->instance)",
	old:   testing.Attrs{"firewall-mode": config.FwGlobal},
	new:   testing.Attrs{"firewall-mode": config.FwInstance},
	err:   `cannot change firewall-mode from "global" to "instance"`,
}, {
	about: "Can't change the firewall-mode (global->none)",
	old:   testing.Attrs{"firewall-mode": config.FwGlobal},
	new:   testing.Attrs{"firewall-mode": config.FwNone},
	err:   `cannot change firewall-mode from "global" to "none"`,
}, {
	about: "Cannot change uuid",
	old:   testing.Attrs{"uuid": "90168e4c-2f10-4e9c-83c2-1fb55a58e5a9"},
	new:   testing.Attrs{"uuid": "dcfbdb4a-bca2-49ad-aa7c-f011424e0fe4"},
	err:   "cannot change uuid from \"90168e4c-2f10-4e9c-83c2-1fb55a58e5a9\" to \"dcfbdb4a-bca2-49ad-aa7c-f011424e0fe4\"",
}}

func (s *ConfigSuite) TestValidateChange(c *gc.C) {
	files := []gitjujutesting.TestFile{
		{".ssh/identity.pub", "identity"},
	}
	s.FakeHomeSuite.Home.AddFiles(c, files...)

	for i, test := range validationTests {
		c.Logf("test %d: %s", i, test.about)
		newConfig := newTestConfig(c, test.new)
		oldConfig := newTestConfig(c, test.old)
		err := config.Validate(newConfig, oldConfig)
		if test.err == "" {
			c.Check(err, jc.ErrorIsNil)
		} else {
			c.Check(err, gc.ErrorMatches, test.err)
		}
	}
}

func (s *ConfigSuite) addJujuFiles(c *gc.C) {
	s.FakeHomeSuite.Home.AddFiles(c, []gitjujutesting.TestFile{
		{".ssh/id_rsa.pub", "rsa\n"},
	}...)
}

func (s *ConfigSuite) TestValidateUnknownAttrs(c *gc.C) {
	s.addJujuFiles(c)
	cfg, err := config.New(config.UseDefaults, map[string]interface{}{
		"name":    "myenv",
		"type":    "other",
		"uuid":    testing.ModelTag.Id(),
		"known":   "this",
		"unknown": "that",
	})
	c.Assert(err, jc.ErrorIsNil)

	// No fields: all attrs passed through.
	attrs, err := cfg.ValidateUnknownAttrs(nil, nil)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(attrs, gc.DeepEquals, map[string]interface{}{
		"known":   "this",
		"unknown": "that",
	})

	// Valid field: that and other attrs passed through.
	fields := schema.Fields{"known": schema.String()}
	attrs, err = cfg.ValidateUnknownAttrs(fields, nil)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(attrs, gc.DeepEquals, map[string]interface{}{
		"known":   "this",
		"unknown": "that",
	})

	// Default field: inserted.
	fields["default"] = schema.String()
	defaults := schema.Defaults{"default": "the other"}
	attrs, err = cfg.ValidateUnknownAttrs(fields, defaults)
	c.Assert(err, jc.ErrorIsNil)
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

type testAttr struct {
	message string
	aKey    string
	aValue  string
	checker gc.Checker
}

var emptyAttributeTests = []testAttr{
	{
		message: "Warning message about unknown attribute (%v) is expected because attribute value exists",
		aKey:    "unknown",
		aValue:  "unknown value",
		checker: gc.Matches,
	}, {
		message: "Warning message about unknown attribute (%v) is unexpected because attribute value is empty",
		aKey:    "unknown-empty",
		aValue:  "",
		checker: gc.Not(gc.Matches),
	},
}

func (s *ConfigSuite) TestValidateUnknownEmptyAttr(c *gc.C) {
	s.addJujuFiles(c)
	cfg, err := config.New(config.UseDefaults, map[string]interface{}{
		"name": "myenv",
		"type": "other",
		"uuid": testing.ModelTag.Id(),
	})
	c.Assert(err, jc.ErrorIsNil)
	warningTxt := `.* unknown config field %q.*`

	for i, test := range emptyAttributeTests {
		c.Logf("test %d: %v\n", i, fmt.Sprintf(test.message, test.aKey))
		testCfg, err := cfg.Apply(map[string]interface{}{test.aKey: test.aValue})
		c.Assert(err, jc.ErrorIsNil)
		attrs, err := testCfg.ValidateUnknownAttrs(nil, nil)
		c.Assert(err, jc.ErrorIsNil)
		// all attrs passed through
		c.Assert(attrs, gc.DeepEquals, map[string]interface{}{test.aKey: test.aValue})
		expectedWarning := fmt.Sprintf(warningTxt, test.aKey)
		logOutputText := strings.Replace(c.GetTestLog(), "\n", "", -1)
		// warning displayed or not based on test expectation
		c.Assert(logOutputText, test.checker, expectedWarning, gc.Commentf(test.message, test.aKey))
	}
}

func newTestConfig(c *gc.C, explicit testing.Attrs) *config.Config {
	final := testing.Attrs{
		"type": "my-type", "name": "my-name",
		"uuid": testing.ModelTag.Id(),
	}
	for key, value := range explicit {
		final[key] = value
	}
	result, err := config.New(config.UseDefaults, final)
	c.Assert(err, jc.ErrorIsNil)
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

func (s *ConfigSuite) TestAutoHookRetryDefault(c *gc.C) {
	config := newTestConfig(c, testing.Attrs{})
	c.Assert(config.AutomaticallyRetryHooks(), gc.Equals, true)
}

func (s *ConfigSuite) TestAutoHookRetryFalseEnv(c *gc.C) {
	config := newTestConfig(c, testing.Attrs{
		"automatically-retry-hooks": "false"})
	c.Assert(config.AutomaticallyRetryHooks(), gc.Equals, false)
}

func (s *ConfigSuite) TestAutoHookRetryTrueEnv(c *gc.C) {
	config := newTestConfig(c, testing.Attrs{
		"automatically-retry-hooks": "true"})
	c.Assert(config.AutomaticallyRetryHooks(), gc.Equals, true)
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

func (s *ConfigSuite) TestProxyValuesWithFallbackNoScheme(c *gc.C) {
	s.addJujuFiles(c)

	config := newTestConfig(c, testing.Attrs{
		"http-proxy":  "user@10.0.0.1",
		"https-proxy": "user@10.0.0.1",
		"ftp-proxy":   "user@10.0.0.1",
		"no-proxy":    "localhost,10.0.3.1",
	})
	c.Assert(config.HttpProxy(), gc.Equals, "user@10.0.0.1")
	c.Assert(config.AptHttpProxy(), gc.Equals, "http://user@10.0.0.1")
	c.Assert(config.HttpsProxy(), gc.Equals, "user@10.0.0.1")
	c.Assert(config.AptHttpsProxy(), gc.Equals, "https://user@10.0.0.1")
	c.Assert(config.FtpProxy(), gc.Equals, "user@10.0.0.1")
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
	proxySettings := proxy.Settings{
		Http:    "http proxy",
		Https:   "https proxy",
		Ftp:     "ftp proxy",
		NoProxy: "no proxy",
	}
	expectedProxySettings := proxy.Settings{
		Http:    "http://http proxy",
		Https:   "https://https proxy",
		Ftp:     "ftp://ftp proxy",
		NoProxy: "",
	}
	cfg, err := cfg.Apply(config.ProxyConfigMap(proxySettings))
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(cfg.ProxySettings(), gc.DeepEquals, proxySettings)
	// Apt proxy settings always include the scheme. NoProxy is empty.
	c.Assert(cfg.AptProxySettings(), gc.DeepEquals, expectedProxySettings)
}

func (s *ConfigSuite) TestAptProxyConfigMap(c *gc.C) {
	s.addJujuFiles(c)
	cfg := newTestConfig(c, testing.Attrs{})
	proxySettings := proxy.Settings{
		Http:  "http://httpproxy",
		Https: "https://httpsproxy",
		Ftp:   "ftp://ftpproxy",
	}
	cfg, err := cfg.Apply(config.AptProxyConfigMap(proxySettings))
	c.Assert(err, jc.ErrorIsNil)
	// The default proxy settings should still be empty.
	c.Assert(cfg.ProxySettings(), gc.DeepEquals, proxy.Settings{})
	c.Assert(cfg.AptProxySettings(), gc.DeepEquals, proxySettings)
}

func (s *ConfigSuite) TestSchemaNoExtra(c *gc.C) {
	schema, err := config.Schema(nil)
	c.Assert(err, gc.IsNil)
	orig := make(environschema.Fields)
	for name, field := range config.ConfigSchema {
		orig[name] = field
	}
	c.Assert(schema, jc.DeepEquals, orig)
	// Check that we actually returned a copy, not the original.
	schema["foo"] = environschema.Attr{}
	_, ok := orig["foo"]
	c.Assert(ok, jc.IsFalse)
}

func (s *ConfigSuite) TestSchemaWithExtraFields(c *gc.C) {
	extraField := environschema.Attr{
		Description: "fooish",
		Type:        environschema.Tstring,
	}
	schema, err := config.Schema(environschema.Fields{
		"foo": extraField,
	})
	c.Assert(err, gc.IsNil)
	c.Assert(schema["foo"], gc.DeepEquals, extraField)
	delete(schema, "foo")
	orig := make(environschema.Fields)
	for name, field := range config.ConfigSchema {
		orig[name] = field
	}
	c.Assert(schema, jc.DeepEquals, orig)
}

func (s *ConfigSuite) TestSchemaWithExtraOverlap(c *gc.C) {
	schema, err := config.Schema(environschema.Fields{
		"type": environschema.Attr{
			Description: "duplicate",
			Type:        environschema.Tstring,
		},
	})
	c.Assert(err, gc.ErrorMatches, `config field "type" clashes with global config`)
	c.Assert(schema, gc.IsNil)
}

func (s *ConfigSuite) TestCoerceForStorage(c *gc.C) {
	cfg := newTestConfig(c, testing.Attrs{
		"resource-tags": "a=b c=d"})
	tags, ok := cfg.ResourceTags()
	c.Assert(ok, jc.IsTrue)
	expectedTags := map[string]string{"a": "b", "c": "d"}
	c.Assert(tags, gc.DeepEquals, expectedTags)
	tagsStr := config.CoerceForStorage(cfg.AllAttrs())["resource-tags"].(string)
	tagItems := strings.Split(tagsStr, " ")
	tagsMap := make(map[string]string)
	for _, kv := range tagItems {
		parts := strings.Split(kv, "=")
		tagsMap[parts[0]] = parts[1]
	}
	c.Assert(tagsMap, gc.DeepEquals, expectedTags)
}

var specializeCharmRepoTests = []struct {
	about    string
	testMode bool
	repo     charmrepo.Interface
}{{
	about: "test mode disabled, charm store",
	repo:  &specializedCharmRepo{},
}, {
	about:    "test mode enabled, charm store",
	testMode: true,
	repo:     &specializedCharmRepo{},
}}

func (s *ConfigSuite) TestSpecializeCharmRepo(c *gc.C) {
	for i, test := range specializeCharmRepoTests {
		c.Logf("test %d: %s", i, test.about)
		cfg := newTestConfig(c, testing.Attrs{"test-mode": test.testMode})
		repo := config.SpecializeCharmRepo(test.repo, cfg)
		store := repo.(*specializedCharmRepo)
		c.Assert(store.testMode, gc.Equals, test.testMode)
	}
}

type specializedCharmRepo struct {
	*charmrepo.CharmStore
	testMode bool
}

func (s *specializedCharmRepo) WithTestMode() charmrepo.Interface {
	s.testMode = true
	return s
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
