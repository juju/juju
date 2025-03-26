// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package config_test

import (
	"context"
	"fmt"
	"strings"
	stdtesting "testing"
	"time"

	"github.com/juju/collections/set"
	"github.com/juju/loggo/v2"
	"github.com/juju/proxy"
	"github.com/juju/schema"
	jujutesting "github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	jujuversion "github.com/juju/juju/core/version"
	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/internal/charmhub"
	"github.com/juju/juju/internal/configschema"
	"github.com/juju/juju/internal/featureflag"
	"github.com/juju/juju/internal/testing"
	"github.com/juju/juju/internal/version"
	"github.com/juju/juju/juju/osenv"
)

func Test(t *stdtesting.T) {
	gc.TestingT(t)
}

type ConfigSuite struct {
	testing.FakeJujuXDGDataHomeSuite
}

var _ = gc.Suite(&ConfigSuite{})

func (s *ConfigSuite) SetUpTest(c *gc.C) {
	s.SetInitialFeatureFlags(featureflag.DeveloperMode)
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
	"firewall-mode":              config.FwInstance,
	"unknown":                    "my-unknown",
	"ssl-hostname-verification":  true,
	"development":                false,
	"default-base":               jujuversion.DefaultSupportedLTSBase().String(),
	"disable-network-management": false,
	"ignore-machine-addresses":   false,
	"automatically-retry-hooks":  true,
	"proxy-ssh":                  false,
	"resource-tags":              []string{},
	"secret-backend":             "auto",
}

type configTest struct {
	about       string
	useDefaults config.Defaulting
	attrs       testing.Attrs
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
		about:       "Streams",
		useDefaults: config.UseDefaults,
		attrs: minimalConfigAttrs.Merge(testing.Attrs{
			"image-stream":           "released",
			"agent-stream":           "released",
			"container-image-stream": "daily",
		}),
	}, {
		about:       "Metadata URLs",
		useDefaults: config.UseDefaults,
		attrs: minimalConfigAttrs.Merge(testing.Attrs{
			"image-metadata-url":           "image-url",
			"agent-metadata-url":           "agent-metadata-url-value",
			"container-image-metadata-url": "container-image-metadata-url-value",
		}),
	}, {
		about:       "Metadata Defaults Disabled",
		useDefaults: config.UseDefaults,
		attrs: minimalConfigAttrs.Merge(testing.Attrs{
			"image-metadata-defaults-disabled":           true,
			"container-image-metadata-defaults-disabled": true,
		}),
	}, {
		about:       "Explicit base",
		useDefaults: config.UseDefaults,
		attrs: minimalConfigAttrs.Merge(testing.Attrs{
			"default-base": "ubuntu@20.04",
		}),
	}, {
		about:       "old base",
		useDefaults: config.UseDefaults,
		attrs: minimalConfigAttrs.Merge(testing.Attrs{
			"default-base": "ubuntu@18.04",
		}),
		err: `base "ubuntu@18.04" not supported`,
	}, {
		about:       "bad base",
		useDefaults: config.UseDefaults,
		attrs: minimalConfigAttrs.Merge(testing.Attrs{
			"default-base": "my-series",
		}),
		err: `invalid default base "my-series": expected base string to contain os and channel separated by '@'`,
	}, {
		about:       "Explicit logging",
		useDefaults: config.UseDefaults,
		attrs: minimalConfigAttrs.Merge(testing.Attrs{
			"logging-config": "juju=INFO",
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
		about:       "num-provision-workers: 42",
		useDefaults: config.UseDefaults,
		attrs: minimalConfigAttrs.Merge(testing.Attrs{
			"num-provision-workers": 42,
		}),
	}, {
		about:       "num-provision-workers: over max",
		useDefaults: config.UseDefaults,
		attrs: minimalConfigAttrs.Merge(testing.Attrs{
			"num-provision-workers": 101,
		}),
		err: `num-provision-workers: must be less than 100`,
	}, {
		about:       "num-container-provision-workers: 17",
		useDefaults: config.UseDefaults,
		attrs: minimalConfigAttrs.Merge(testing.Attrs{
			"num-container-provision-workers": 17,
		}),
	}, {
		about:       "num-container-provision-workers: over max",
		useDefaults: config.UseDefaults,
		attrs: minimalConfigAttrs.Merge(testing.Attrs{
			"num-container-provision-workers": 26,
		}),
		err: `num-container-provision-workers: must be less than 25`,
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
		about:       "compatible with older empty, now mandatory",
		useDefaults: config.NoDefaults,
		attrs: sampleConfig.Merge(testing.Attrs{
			"secret-backend": "",
		}),
	}, {
		about:       "compatible with older missing, now mandatory",
		useDefaults: config.NoDefaults,
		attrs: minimalConfigAttrs.Merge(testing.Attrs{
			"resource-tags": []string{},
		}),
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
			"region":                     "us-east-1",
			"default-series":             "focal",
			"default-base":               "ubuntu@20.04",
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
			"secret-backend":             "auto",
		},
	}, {
		about:       "TestMode flag specified",
		useDefaults: config.UseDefaults,
		attrs: minimalConfigAttrs.Merge(testing.Attrs{
			"test-mode": true,
		}),
	}, {
		about:       "Mode flag specified",
		useDefaults: config.UseDefaults,
		attrs: minimalConfigAttrs.Merge(testing.Attrs{
			"mode": "strict",
		}),
	}, {
		about:       "Mode flag includes requires-prompts",
		useDefaults: config.UseDefaults,
		attrs: minimalConfigAttrs.Merge(testing.Attrs{
			"mode": "strict,requires-prompts",
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
		about:       "net-bond-reconfigure-delay value",
		useDefaults: config.UseDefaults,
		attrs: minimalConfigAttrs.Merge(testing.Attrs{
			config.NetBondReconfigureDelayKey: 1234,
		}),
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
		about:       "Valid container-inherit-properties",
		useDefaults: config.UseDefaults,
		attrs: minimalConfigAttrs.Merge(testing.Attrs{
			"container-inherit-properties": "apt-primary, ca-certs",
		}),
	}, {
		about:       "Invalid container-inherit-properties",
		useDefaults: config.UseDefaults,
		attrs: minimalConfigAttrs.Merge(testing.Attrs{
			"container-inherit-properties": "apt-security, write_files,users,apt-sources",
		}),
		err: `container-inherit-properties: users, write_files not allowed`,
	}, {
		about:       "String as valid value",
		useDefaults: config.UseDefaults,
		attrs: minimalConfigAttrs.Merge(testing.Attrs{
			"backup-dir": "/foo/bar",
		}),
	}, {
		about:       "Default-space takes a space name as valid value",
		useDefaults: config.UseDefaults,
		attrs: minimalConfigAttrs.Merge(testing.Attrs{
			"default-space": "bar",
		}),
	}, {
		about:       "Valid charm-hub api url",
		useDefaults: config.UseDefaults,
		attrs: minimalConfigAttrs.Merge(testing.Attrs{
			"charmhub-url": "http://test.com",
		}),
	}, {
		about:       "Malformed charm-hub api url",
		useDefaults: config.UseDefaults,
		attrs: minimalConfigAttrs.Merge(testing.Attrs{
			"charmhub-url": "http://t est.com",
		}),
		err: `charm-hub url "http://t est.com" not valid`,
	}, {
		about:       "Invalid charm-hub api url",
		useDefaults: config.UseDefaults,
		attrs: minimalConfigAttrs.Merge(testing.Attrs{
			"charmhub-url": "meshuggah",
		}),
		err: `charm-hub url "meshuggah" not valid`,
	}, {
		about:       "Invalid ssh-allow cidr",
		useDefaults: config.UseDefaults,
		attrs: minimalConfigAttrs.Merge(testing.Attrs{
			"ssh-allow": "blah",
		}),
		err: `cidr "blah" not valid`,
	}, {
		about:       "Absent ssh-allow entry",
		useDefaults: config.UseDefaults,
		attrs:       minimalConfigAttrs,
	}, {
		about:       "Invalid saas-ingress-allow cidr",
		useDefaults: config.UseDefaults,
		attrs: minimalConfigAttrs.Merge(testing.Attrs{
			"saas-ingress-allow": "blah",
		}),
		err: `cidr "blah" not valid`,
	},
}

func (s *ConfigSuite) TestConfig(c *gc.C) {
	files := []jujutesting.TestFile{
		{Name: ".ssh/id_dsa.pub", Data: "dsa"},
		{Name: ".ssh/id_rsa.pub", Data: "rsa\n"},
		{Name: ".ssh/id_ed25519.pub", Data: "ed25519\n"},
		{Name: ".ssh/identity.pub", Data: "identity"},
		{Name: ".ssh/authorized_keys", Data: "auth0\n# first\nauth1\n\n"},
		{Name: ".ssh/authorized_keys2", Data: "auth2\nauth3\n"},
	}
	s.FakeHomeSuite.Home.AddFiles(c, files...)
	for i, test := range configTests {
		c.Logf("test %d. %s", i, test.about)
		test.check(c)
	}
}

func (test configTest) check(c *gc.C) {
	cfg, err := config.New(test.useDefaults, test.attrs)
	if test.err != "" {
		c.Check(cfg, gc.IsNil)
		c.Check(err, gc.ErrorMatches, test.err)
		return
	}
	if !c.Check(err, jc.ErrorIsNil, gc.Commentf("config.New failed")) {
		// As we have a Check not an Assert so the test should not
		// continue from here as it will result in a nil pointer panic.
		return
	}

	typ, _ := test.attrs["type"].(string)
	// "null" has been deprecated in favour of "manual",
	// and is automatically switched.
	if typ == "null" {
		typ = "manual"
	}
	name, _ := test.attrs["name"].(string)
	c.Check(cfg.Type(), gc.Equals, typ)
	c.Check(cfg.Name(), gc.Equals, name)
	agentVersion, ok := cfg.AgentVersion()
	if s := test.attrs["agent-version"]; s != nil {
		c.Check(ok, jc.IsTrue)
		c.Check(agentVersion, gc.Equals, version.MustParse(s.(string)))
	} else {
		c.Check(ok, jc.IsFalse)
		c.Check(agentVersion, gc.Equals, version.Zero)
	}

	if expected, ok := test.attrs["uuid"]; ok {
		c.Check(cfg.UUID(), gc.Equals, expected)
	}

	dev, _ := test.attrs["development"].(bool)
	c.Check(cfg.Development(), gc.Equals, dev)

	baseAttr, _ := test.attrs["default-base"].(string)
	defaultBase, ok := cfg.DefaultBase()
	if baseAttr != "" {
		c.Assert(ok, jc.IsTrue)
		c.Assert(defaultBase, gc.Equals, baseAttr)
	} else {
		c.Assert(ok, jc.IsFalse)
		c.Assert(defaultBase, gc.Equals, "")
	}

	if m, _ := test.attrs["firewall-mode"].(string); m != "" {
		c.Check(cfg.FirewallMode(), gc.Equals, m)
	}

	if m, _ := test.attrs["default-space"].(string); m != "" {
		c.Check(cfg.DefaultSpace(), gc.Equals, m)
	}

	if v, ok := test.attrs["ssl-hostname-verification"]; ok {
		c.Check(cfg.SSLHostnameVerification(), gc.Equals, v)
	}

	if v, ok := test.attrs["provisioner-harvest-mode"]; ok {
		harvestMeth, err := config.ParseHarvestMode(v.(string))
		c.Check(err, jc.ErrorIsNil)
		c.Check(cfg.ProvisionerHarvestMode(), gc.Equals, harvestMeth)
	} else {
		c.Check(cfg.ProvisionerHarvestMode(), gc.Equals, config.HarvestDestroyed)
	}

	if v, ok := test.attrs["image-stream"]; ok {
		c.Check(cfg.ImageStream(), gc.Equals, v)
	} else {
		c.Check(cfg.ImageStream(), gc.Equals, "released")
	}

	url, urlPresent := cfg.ImageMetadataURL()
	if v, _ := test.attrs["image-metadata-url"].(string); v != "" {
		c.Check(url, gc.Equals, v)
		c.Check(urlPresent, jc.IsTrue)
	} else {
		c.Check(urlPresent, jc.IsFalse)
	}

	imageMetadataDefaultsDisabled := cfg.ImageMetadataDefaultsDisabled()
	if v, ok := test.attrs["image-metadata-defaults-disabled"].(bool); ok {
		c.Assert(imageMetadataDefaultsDisabled, gc.Equals, v)
	} else {
		c.Assert(imageMetadataDefaultsDisabled, jc.IsFalse)
	}

	agentURL, urlPresent := cfg.AgentMetadataURL()
	expectedToolsURLValue := test.attrs["agent-metadata-url"]
	if urlPresent {
		c.Check(agentURL, gc.Equals, expectedToolsURLValue)
	} else {
		c.Check(agentURL, gc.Equals, "")
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

	containerURL, urlPresent := cfg.ContainerImageMetadataURL()
	if v, _ := test.attrs["container-image-metadata-url"].(string); v != "" {
		c.Check(containerURL, gc.Equals, v)
		c.Check(urlPresent, jc.IsTrue)
	} else {
		c.Check(urlPresent, jc.IsFalse)
	}

	if v, ok := test.attrs["container-image-stream"]; ok {
		c.Check(cfg.ContainerImageStream(), gc.Equals, v)
	} else {
		c.Check(cfg.ContainerImageStream(), gc.Equals, "released")
	}

	containerImageMetadataDefaultsDisabled := cfg.ContainerImageMetadataDefaultsDisabled()
	if v, ok := test.attrs["container-image-metadata-defaults-disabled"].(bool); ok {
		c.Assert(containerImageMetadataDefaultsDisabled, gc.Equals, v)
	} else {
		c.Assert(containerImageMetadataDefaultsDisabled, jc.IsFalse)
	}

	resourceTags, cfgHasResourceTags := cfg.ResourceTags()
	c.Check(cfgHasResourceTags, jc.IsTrue)
	if tags, ok := test.attrs["resource-tags"]; ok {
		switch tags := tags.(type) {
		case []string:
			if len(tags) > 0 {
				c.Check(resourceTags, jc.DeepEquals, testResourceTagsMap)
			}
		case string:
			if tags != "" {
				c.Check(resourceTags, jc.DeepEquals, testResourceTagsMap)
			}
		}
	} else {
		c.Check(resourceTags, gc.HasLen, 0)
	}

	xmit := cfg.TransmitVendorMetrics()
	expectedXmit, xmitAsserted := test.attrs["transmit-vendor-metrics"]
	if xmitAsserted {
		c.Check(xmit, gc.Equals, expectedXmit)
	} else {
		c.Check(xmit, jc.IsTrue)
	}

	if val, ok := test.attrs[config.NetBondReconfigureDelayKey].(int); ok {
		c.Assert(cfg.NetBondReconfigureDelay(), gc.Equals, val)
	}

	if val, ok := test.attrs[config.ContainerInheritPropertiesKey].(string); ok && val != "" {
		c.Assert(cfg.ContainerInheritProperties(), gc.Equals, val)
	}
	c.Assert(cfg.SSHAllow(), gc.DeepEquals, []string{"0.0.0.0/0", "::/0"})
}

func (s *ConfigSuite) TestAllAttrs(c *gc.C) {
	// Normally this is handled by jujutesting.FakeHome
	s.PatchEnvironment(osenv.JujuLoggingConfigEnvKey, "")
	attrs := map[string]interface{}{
		"type":                       "my-type",
		"name":                       "my-name",
		"uuid":                       "90168e4c-2f10-4e9c-83c2-1fb55a58e5a9",
		"firewall-mode":              config.FwInstance,
		"unknown":                    "my-unknown",
		"ssl-hostname-verification":  true,
		"default-base":               jujuversion.DefaultSupportedLTSBase().String(),
		"disable-network-management": false,
		"ignore-machine-addresses":   false,
		"automatically-retry-hooks":  true,
		"proxy-ssh":                  false,
		"development":                false,
		"test-mode":                  false,
	}
	cfg, err := config.New(config.NoDefaults, attrs)
	c.Assert(err, jc.ErrorIsNil)

	// Set from default
	attrs["logging-config"] = "<root>=INFO"

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
}, {
	about: "Can't change the charmhub-url (global->none)",
	old:   testing.Attrs{"charmhub-url": "http://a.com"},
	new:   testing.Attrs{"charmhub-url": "http://b.com"},
	err:   `cannot change charmhub-url from "http://a.com" to "http://b.com"`,
}, {
	about: "Can't clear apt-mirror",
	old:   testing.Attrs{"apt-mirror": "http://mirror"},
	err:   `cannot clear apt-mirror`,
}}

func (s *ConfigSuite) TestValidateChange(c *gc.C) {
	files := []jujutesting.TestFile{
		{Name: ".ssh/identity.pub", Data: "identity"},
	}
	s.FakeHomeSuite.Home.AddFiles(c, files...)

	for i, test := range validationTests {
		c.Logf("test %d: %s", i, test.about)
		newConfig := newTestConfig(c, test.new)
		oldConfig := newTestConfig(c, test.old)
		err := config.Validate(context.Background(), newConfig, oldConfig)
		if test.err == "" {
			c.Check(err, jc.ErrorIsNil)
		} else {
			c.Check(err, gc.ErrorMatches, test.err)
		}
	}
}

type configValidateCloudInitUserDataTest struct {
	about string
	value string
	err   string
}

var configValidateCloudInitUserDataTests = []configValidateCloudInitUserDataTest{
	{
		about: "Valid cloud init user data values",
		value: validCloudInitUserData,
	}, {
		about: "Invalid cloud init user data values: package int",
		value: invalidCloudInitUserDataPackageInt,
		err:   `cloudinit-userdata: packages must be a list of strings: expected string, got int\(76\)`,
	}, {
		about: "Invalid cloud init user data values: users",
		value: invalidCloudInitUserDataUsers,
		err:   `cloudinit-userdata: users not allowed`,
	}, {
		about: "Invalid cloud init user data values: runcmd",
		value: invalidCloudInitUserDataRunCmd,
		err:   `cloudinit-userdata: runcmd not allowed, use preruncmd or postruncmd instead`,
	}, {
		about: "Invalid cloud init user data values: bootcmd",
		value: invalidCloudInitUserDataBootCmd,
		err:   `cloudinit-userdata: bootcmd not allowed`,
	}, {
		about: "Invalid cloud init user data: yaml",
		value: invalidCloudInitUserDataInvalidYAML,
		err:   `cloudinit-userdata: must be valid YAML: yaml: line 2: did not find expected '-' indicator`,
	},
}

func (s *ConfigSuite) TestValidateCloudInitUserData(c *gc.C) {
	files := []jujutesting.TestFile{
		{Name: ".ssh/id_dsa.pub", Data: "dsa"},
		{Name: ".ssh/id_rsa.pub", Data: "rsa\n"},
		{Name: ".ssh/identity.pub", Data: "identity"},
		{Name: ".ssh/authorized_keys", Data: "auth0\n# first\nauth1\n\n"},
		{Name: ".ssh/authorized_keys2", Data: "auth2\nauth3\n"},
	}
	s.FakeHomeSuite.Home.AddFiles(c, files...)
	for i, test := range configValidateCloudInitUserDataTests {
		c.Logf("test %d of %d. %s", i+1, len(configValidateCloudInitUserDataTests), test.about)
		test.checkNew(c)
	}
}

func (test configValidateCloudInitUserDataTest) checkNew(c *gc.C) {
	final := testing.Attrs{
		"type": "my-type", "name": "my-name",
		"uuid":                      testing.ModelTag.Id(),
		config.CloudInitUserDataKey: test.value,
	}

	_, err := config.New(config.UseDefaults, final)
	if test.err != "" {
		c.Assert(err, gc.ErrorMatches, test.err)
		return
	}
	c.Assert(err, jc.ErrorIsNil)
}

func (s *ConfigSuite) addJujuFiles(c *gc.C) {
	s.FakeHomeSuite.Home.AddFiles(c, []jujutesting.TestFile{
		{Name: ".ssh/id_rsa.pub", Data: "rsa\n"},
	}...)
}

func (s *ConfigSuite) TestValidateUnknownAttrs(c *gc.C) {
	s.addJujuFiles(c)
	cfg, err := config.New(config.UseDefaults, map[string]interface{}{
		"name":              "myenv",
		"type":              "other",
		"uuid":              testing.ModelTag.Id(),
		"extra-info":        "official extra user data",
		"known":             "this",
		"unknown":           "that",
		"unknown-part-deux": []interface{}{"meshuggah"},
	})
	c.Assert(err, jc.ErrorIsNil)

	// No fields: all attrs passed through.
	attrs, err := cfg.ValidateUnknownAttrs(nil, nil)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(attrs, gc.DeepEquals, map[string]interface{}{
		"known":             "this",
		"unknown":           "that",
		"unknown-part-deux": []interface{}{"meshuggah"},
	})

	// Valid field: that and other attrs passed through.
	fields := schema.Fields{"known": schema.String()}
	attrs, err = cfg.ValidateUnknownAttrs(fields, nil)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(attrs, gc.DeepEquals, map[string]interface{}{
		"known":             "this",
		"unknown":           "that",
		"unknown-part-deux": []interface{}{"meshuggah"},
	})

	// Default field: inserted.
	fields["default"] = schema.String()
	defaults := schema.Defaults{"default": "the other"}
	attrs, err = cfg.ValidateUnknownAttrs(fields, defaults)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(attrs, gc.DeepEquals, map[string]interface{}{
		"known":             "this",
		"unknown":           "that",
		"unknown-part-deux": []interface{}{"meshuggah"},
		"default":           "the other",
	})

	// Invalid field: failure.
	fields["known"] = schema.Int()
	_, err = cfg.ValidateUnknownAttrs(fields, defaults)
	c.Assert(err, gc.ErrorMatches, `known: expected int, got string\("this"\)`)

	// Completely unknown attr, not-simple field type: failure.
	cfg, err = config.New(config.UseDefaults, map[string]interface{}{
		"name":       "myenv",
		"type":       "other",
		"uuid":       testing.ModelTag.Id(),
		"extra-info": "official extra user data",
		"known":      "this",
		"unknown":    "that",
		"mapAttr":    map[string]string{"foo": "bar"},
	})
	c.Assert(err, jc.ErrorIsNil)
	_, err = cfg.ValidateUnknownAttrs(nil, nil)
	c.Assert(err.Error(), gc.Equals, `mapAttr: unknown type (map["foo":"bar"])`)

	// Completely unknown attr, not-simple field type: failure.
	cfg, err = config.New(config.UseDefaults, map[string]interface{}{
		"name":       "myenv",
		"type":       "other",
		"uuid":       testing.ModelTag.Id(),
		"extra-info": "official extra user data",
		"known":      "this",
		"unknown":    "that",
		"bad":        []interface{}{1},
	})
	c.Assert(err, jc.ErrorIsNil)
	_, err = cfg.ValidateUnknownAttrs(nil, nil)
	c.Assert(err.Error(), gc.Equals, `bad: unknown type ([1])`)
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
	c.Assert(config.LoggingConfig(), gc.Equals, "<root>=WARNING;juju=DEBUG")
}

func (s *ConfigSuite) TestLoggingConfigDefaults(c *gc.C) {
	s.addJujuFiles(c)
	s.PatchEnvironment(osenv.JujuLoggingConfigEnvKey, "")
	config := newTestConfig(c, testing.Attrs{})
	c.Assert(config.LoggingConfig(), gc.Equals, "<root>=INFO")
}

func (s *ConfigSuite) TestLoggingConfigWithUnit(c *gc.C) {
	s.addJujuFiles(c)
	config := newTestConfig(c, testing.Attrs{
		"logging-config": "<root>=WARNING;unit=INFO"})
	c.Assert(config.LoggingConfig(), gc.Equals, "<root>=WARNING;unit=INFO")
}

func (s *ConfigSuite) TestLoggingConfigFromEnvironment(c *gc.C) {
	s.addJujuFiles(c)
	s.PatchEnvironment(osenv.JujuLoggingConfigEnvKey, "<root>=INFO;other=TRACE")

	config := newTestConfig(c, nil)
	c.Assert(config.LoggingConfig(), gc.Equals, "<root>=INFO;other=TRACE")

	// But an explicit value overrides the environ
	config = newTestConfig(c, testing.Attrs{
		"logging-config": "<root>=WARNING"})
	c.Assert(config.LoggingConfig(), gc.Equals, "<root>=WARNING")
}

func (s *ConfigSuite) TestBackupDir(c *gc.C) {
	s.addJujuFiles(c)
	testDir := c.MkDir()
	config := newTestConfig(c, testing.Attrs{
		"backup-dir": testDir})
	c.Assert(config.BackupDir(), gc.Equals, testDir)
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

func (s *ConfigSuite) TestCharmHubURL(c *gc.C) {
	config := newTestConfig(c, testing.Attrs{})
	chURL, ok := config.CharmHubURL()
	c.Assert(ok, jc.IsTrue)
	c.Assert(chURL, gc.Equals, charmhub.DefaultServerURL)
}

func (s *ConfigSuite) TestMode(c *gc.C) {
	cfg := newTestConfig(c, testing.Attrs{})
	mode, ok := cfg.Mode()
	c.Assert(ok, jc.IsTrue)
	c.Assert(mode, gc.DeepEquals, set.NewStrings(config.RequiresPromptsMode))

	cfg = newTestConfig(c, testing.Attrs{
		config.ModeKey: "",
	})
	mode, ok = cfg.Mode()
	c.Assert(ok, jc.IsFalse)
	c.Assert(mode, gc.DeepEquals, set.NewStrings())
}

func (s *ConfigSuite) TestSSHAllow(c *gc.C) {
	cfg := newTestConfig(c, testing.Attrs{})
	allowlist := cfg.SSHAllow()
	c.Assert(allowlist, gc.DeepEquals, []string{"0.0.0.0/0", "::/0"})

	cfg = newTestConfig(c, testing.Attrs{
		config.SSHAllowKey: "192.168.0.0/24,192.168.2.0/24",
	})
	allowlist = cfg.SSHAllow()
	c.Assert(allowlist, gc.HasLen, 2)
	c.Assert(allowlist[0], gc.Equals, "192.168.0.0/24")
	c.Assert(allowlist[1], gc.Equals, "192.168.2.0/24")

	cfg = newTestConfig(c, testing.Attrs{
		config.SSHAllowKey: "",
	})
	allowlist = cfg.SSHAllow()
	c.Assert(allowlist, gc.HasLen, 0)
}

func (s *ConfigSuite) TestApplicationOfferAllowList(c *gc.C) {
	cfg := newTestConfig(c, testing.Attrs{})
	allowlist := cfg.SAASIngressAllow()
	c.Assert(allowlist, gc.DeepEquals, []string{"0.0.0.0/0", "::/0"})

	cfg = newTestConfig(c, testing.Attrs{
		config.SAASIngressAllowKey: "192.168.0.0/24,192.168.2.0/24",
	})
	allowlist = cfg.SAASIngressAllow()
	c.Assert(allowlist, gc.HasLen, 2)
	c.Assert(allowlist[0], gc.Equals, "192.168.0.0/24")
	c.Assert(allowlist[1], gc.Equals, "192.168.2.0/24")

	attrs := testing.FakeConfig().Merge(testing.Attrs{
		config.SAASIngressAllowKey: "",
	})
	_, err := config.New(config.UseDefaults, attrs)
	c.Assert(err, gc.ErrorMatches, "empty cidrs not valid")
}

func (s *ConfigSuite) TestCharmHubURLSettingValue(c *gc.C) {
	url := "http://meshuggah-rocks.com/charmhub"
	config := newTestConfig(c, testing.Attrs{
		"charmhub-url": url,
	})
	chURL, ok := config.CharmHubURL()
	c.Assert(ok, jc.IsTrue)
	c.Assert(chURL, gc.Equals, url)
}

func (s *ConfigSuite) TestNoBothProxy(c *gc.C) {
	config := newTestConfig(c, testing.Attrs{
		"http-proxy":  "http://user@10.0.0.1",
		"https-proxy": "https://user@10.0.0.1",
		"ftp-proxy":   "ftp://user@10.0.0.1",
		"no-proxy":    "localhost,10.0.3.1",
	})
	_, err := config.Apply(testing.Attrs{
		"juju-http-proxy":  "http://user@10.0.0.1",
		"juju-https-proxy": "https://user@10.0.0.1",
		"juju-ftp-proxy":   "ftp://user@10.0.0.1",
		"juju-no-proxy":    "localhost,10.0.3.1",
	})
	c.Assert(err, gc.ErrorMatches, "cannot specify both legacy proxy values and juju proxy values")
}

func (s *ConfigSuite) TestLegacyProxyValuesWithFallback(c *gc.C) {
	s.addJujuFiles(c)

	config := newTestConfig(c, testing.Attrs{
		"http-proxy":  "http://user@10.0.0.1",
		"https-proxy": "https://user@10.0.0.1",
		"ftp-proxy":   "ftp://user@10.0.0.1",
		"no-proxy":    "localhost,10.0.3.1",
	})
	c.Assert(config.HTTPProxy(), gc.Equals, "http://user@10.0.0.1")
	c.Assert(config.AptHTTPProxy(), gc.Equals, "http://user@10.0.0.1")
	c.Assert(config.HTTPSProxy(), gc.Equals, "https://user@10.0.0.1")
	c.Assert(config.AptHTTPSProxy(), gc.Equals, "https://user@10.0.0.1")
	c.Assert(config.FTPProxy(), gc.Equals, "ftp://user@10.0.0.1")
	c.Assert(config.AptFTPProxy(), gc.Equals, "ftp://user@10.0.0.1")
	c.Assert(config.NoProxy(), gc.Equals, "localhost,10.0.3.1")
	c.Assert(config.AptNoProxy(), gc.Equals, "localhost,10.0.3.1")

	c.Assert(config.JujuHTTPProxy(), gc.Equals, "")
	c.Assert(config.JujuHTTPSProxy(), gc.Equals, "")
	c.Assert(config.JujuFTPProxy(), gc.Equals, "")
	// Default no-proxy value.
	c.Assert(config.JujuNoProxy(), gc.Equals, "127.0.0.1,localhost,::1")
}

func (s *ConfigSuite) TestJujuProxyValuesWithFallback(c *gc.C) {
	s.addJujuFiles(c)
	config := newTestConfig(c, testing.Attrs{
		"juju-http-proxy":  "http://user@10.0.0.1",
		"juju-https-proxy": "https://user@10.0.0.1",
		"juju-ftp-proxy":   "ftp://user@10.0.0.1",
		"juju-no-proxy":    "localhost,10.0.3.1",
	})
	c.Assert(config.JujuHTTPProxy(), gc.Equals, "http://user@10.0.0.1")
	c.Assert(config.AptHTTPProxy(), gc.Equals, "http://user@10.0.0.1")
	c.Assert(config.JujuHTTPSProxy(), gc.Equals, "https://user@10.0.0.1")
	c.Assert(config.AptHTTPSProxy(), gc.Equals, "https://user@10.0.0.1")
	c.Assert(config.JujuFTPProxy(), gc.Equals, "ftp://user@10.0.0.1")
	c.Assert(config.AptFTPProxy(), gc.Equals, "ftp://user@10.0.0.1")
	c.Assert(config.JujuNoProxy(), gc.Equals, "localhost,10.0.3.1")
	c.Assert(config.AptNoProxy(), gc.Equals, "localhost,10.0.3.1")

	c.Assert(config.HTTPProxy(), gc.Equals, "")
	c.Assert(config.HTTPSProxy(), gc.Equals, "")
	c.Assert(config.FTPProxy(), gc.Equals, "")
	// Default no-proxy value.
	c.Assert(config.NoProxy(), gc.Equals, "127.0.0.1,localhost,::1")
}

func (s *ConfigSuite) TestProxyValuesWithFallbackNoScheme(c *gc.C) {
	s.addJujuFiles(c)

	config := newTestConfig(c, testing.Attrs{
		"http-proxy":  "user@10.0.0.1",
		"https-proxy": "user@10.0.0.1",
		"ftp-proxy":   "user@10.0.0.1",
		"no-proxy":    "localhost,10.0.3.1",
	})
	c.Assert(config.HTTPProxy(), gc.Equals, "user@10.0.0.1")
	c.Assert(config.AptHTTPProxy(), gc.Equals, "http://user@10.0.0.1")
	c.Assert(config.HTTPSProxy(), gc.Equals, "user@10.0.0.1")
	c.Assert(config.AptHTTPSProxy(), gc.Equals, "https://user@10.0.0.1")
	c.Assert(config.FTPProxy(), gc.Equals, "user@10.0.0.1")
	c.Assert(config.AptFTPProxy(), gc.Equals, "ftp://user@10.0.0.1")
	c.Assert(config.NoProxy(), gc.Equals, "localhost,10.0.3.1")
	c.Assert(config.AptNoProxy(), gc.Equals, "localhost,10.0.3.1")
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
	c.Assert(config.HTTPProxy(), gc.Equals, "http://user@10.0.0.1")
	c.Assert(config.AptHTTPProxy(), gc.Equals, "http://user@10.0.0.2")
	c.Assert(config.HTTPSProxy(), gc.Equals, "https://user@10.0.0.1")
	c.Assert(config.AptHTTPSProxy(), gc.Equals, "https://user@10.0.0.2")
	c.Assert(config.FTPProxy(), gc.Equals, "ftp://user@10.0.0.1")
	c.Assert(config.AptFTPProxy(), gc.Equals, "ftp://user@10.0.0.2")
}

func (s *ConfigSuite) TestProxyValuesNotSet(c *gc.C) {
	s.addJujuFiles(c)
	config := newTestConfig(c, testing.Attrs{})
	c.Assert(config.HTTPProxy(), gc.Equals, "")
	c.Assert(config.AptHTTPProxy(), gc.Equals, "")
	c.Assert(config.HTTPSProxy(), gc.Equals, "")
	c.Assert(config.AptHTTPSProxy(), gc.Equals, "")
	c.Assert(config.FTPProxy(), gc.Equals, "")
	c.Assert(config.AptFTPProxy(), gc.Equals, "")
	c.Assert(config.NoProxy(), gc.Equals, "127.0.0.1,localhost,::1")

	c.Assert(config.SnapHTTPProxy(), gc.Equals, "")
	c.Assert(config.SnapHTTPSProxy(), gc.Equals, "")
	c.Assert(config.SnapStoreProxy(), gc.Equals, "")
	c.Assert(config.SnapStoreAssertions(), gc.Equals, "")
}

func (s *ConfigSuite) TestSnapProxyValues(c *gc.C) {
	s.addJujuFiles(c)
	config := newTestConfig(c, testing.Attrs{
		"snap-http-proxy":       "http://snap-proxy",
		"snap-https-proxy":      "https://snap-proxy",
		"snap-store-proxy":      "42",
		"snap-store-assertions": "trust us",
	})

	c.Assert(config.SnapHTTPProxy(), gc.Equals, "http://snap-proxy")
	c.Assert(config.SnapHTTPSProxy(), gc.Equals, "https://snap-proxy")
	c.Assert(config.SnapStoreProxy(), gc.Equals, "42")
	c.Assert(config.SnapStoreAssertions(), gc.Equals, "trust us")
	c.Assert(config.SnapProxySettings(), gc.Equals, proxy.Settings{
		Http:  "http://snap-proxy",
		Https: "https://snap-proxy",
	})
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
		NoProxy: "no proxy",
	}
	cfg, err := cfg.Apply(config.ProxyConfigMap(proxySettings))
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(cfg.LegacyProxySettings(), gc.DeepEquals, proxySettings)
	cfg, err = cfg.Apply(config.AptProxyConfigMap(proxySettings))
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(cfg.AptProxySettings(), gc.DeepEquals, expectedProxySettings)
}

func (s *ConfigSuite) TestAptProxyConfigMap(c *gc.C) {
	s.addJujuFiles(c)
	cfg := newTestConfig(c, testing.Attrs{})
	proxySettings := proxy.Settings{
		Http:    "http://httpproxy",
		Https:   "https://httpsproxy",
		Ftp:     "ftp://ftpproxy",
		NoProxy: "noproxyhost1,noproxyhost2",
	}
	cfg, err := cfg.Apply(config.AptProxyConfigMap(proxySettings))
	c.Assert(err, jc.ErrorIsNil)
	// The default proxy settings should still be empty.
	c.Assert(cfg.LegacyProxySettings(), gc.DeepEquals, proxy.Settings{NoProxy: "127.0.0.1,localhost,::1"})
	c.Assert(cfg.AptProxySettings(), gc.DeepEquals, proxySettings)
}

func (s *ConfigSuite) TestUpdateStatusHookIntervalConfigDefault(c *gc.C) {
	cfg := newTestConfig(c, testing.Attrs{})
	c.Assert(cfg.UpdateStatusHookInterval(), gc.Equals, 5*time.Minute)
}

func (s *ConfigSuite) TestUpdateStatusHookIntervalConfigValue(c *gc.C) {
	cfg := newTestConfig(c, testing.Attrs{
		"update-status-hook-interval": "30m",
	})
	c.Assert(cfg.UpdateStatusHookInterval(), gc.Equals, 30*time.Minute)
}

func (s *ConfigSuite) TestEgressSubnets(c *gc.C) {
	cfg := newTestConfig(c, testing.Attrs{
		"egress-subnets": "10.0.0.1/32, 192.168.1.1/16",
	})
	c.Assert(cfg.EgressSubnets(), gc.DeepEquals, []string{"10.0.0.1/32", "192.168.1.1/16"})
}

func (s *ConfigSuite) TestCloudInitUserDataFromEnvironment(c *gc.C) {
	cfg := newTestConfig(c, testing.Attrs{
		config.CloudInitUserDataKey: validCloudInitUserData,
	})
	c.Assert(cfg.CloudInitUserData(), gc.DeepEquals, map[string]interface{}{
		"packages":        []interface{}{"python-keystoneclient", "python-glanceclient"},
		"preruncmd":       []interface{}{"mkdir /tmp/preruncmd", "mkdir /tmp/preruncmd2"},
		"postruncmd":      []interface{}{"mkdir /tmp/postruncmd", "mkdir /tmp/postruncmd2"},
		"package_upgrade": false},
	)
}

func (s *ConfigSuite) TestContainerInheritProperties(c *gc.C) {
	cfg := newTestConfig(c, testing.Attrs{
		"container-inherit-properties": "ca-certs,apt-primary",
	})
	c.Assert(cfg.ContainerInheritProperties(), gc.Equals, "ca-certs,apt-primary")
}

func (s *ConfigSuite) TestSchemaNoExtra(c *gc.C) {
	schema, err := config.Schema(nil)
	c.Assert(err, gc.IsNil)
	orig := make(configschema.Fields)
	for name, field := range config.ConfigSchema {
		orig[name] = field
	}
	c.Assert(schema, jc.DeepEquals, orig)
	// Check that we actually returned a copy, not the original.
	schema["foo"] = configschema.Attr{}
	_, ok := orig["foo"]
	c.Assert(ok, jc.IsFalse)
}

func (s *ConfigSuite) TestSchemaWithExtraFields(c *gc.C) {
	extraField := configschema.Attr{
		Description: "fooish",
		Type:        configschema.Tstring,
	}
	schema, err := config.Schema(configschema.Fields{
		"foo": extraField,
	})
	c.Assert(err, gc.IsNil)
	c.Assert(schema["foo"], gc.DeepEquals, extraField)
	delete(schema, "foo")
	orig := make(configschema.Fields)
	for name, field := range config.ConfigSchema {
		orig[name] = field
	}
	c.Assert(schema, jc.DeepEquals, orig)
}

func (s *ConfigSuite) TestSchemaWithExtraOverlap(c *gc.C) {
	schema, err := config.Schema(configschema.Fields{
		"type": configschema.Attr{
			Description: "duplicate",
			Type:        configschema.Tstring,
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

func (s *ConfigSuite) TestLXDSnapChannelConfig(c *gc.C) {
	s.addJujuFiles(c)
	config := newTestConfig(c, testing.Attrs{
		"lxd-snap-channel": "latest/candidate"})
	c.Assert(config.LXDSnapChannel(), gc.Equals, "latest/candidate")
}

func (s *ConfigSuite) TestTelemetryConfig(c *gc.C) {
	cfg := newTestConfig(c, testing.Attrs{})
	c.Assert(cfg.Telemetry(), jc.IsTrue)
}

func (s *ConfigSuite) TestTelemetryConfigTrue(c *gc.C) {
	cfg := newTestConfig(c, testing.Attrs{config.DisableTelemetryKey: true})
	c.Assert(cfg.Telemetry(), jc.IsFalse)
}

func (s *ConfigSuite) TestTelemetryConfigDoesNotExist(c *gc.C) {
	final := testing.Attrs{
		"type": "my-type", "name": "my-name",
		"uuid": testing.ModelTag.Id(),
	}

	cfg, err := config.New(config.UseDefaults, final)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(cfg.Telemetry(), jc.IsTrue)
}

var validCloudInitUserData = `
packages:
  - 'python-keystoneclient'
  - 'python-glanceclient'
preruncmd:
  - mkdir /tmp/preruncmd
  - mkdir /tmp/preruncmd2
postruncmd:
  - mkdir /tmp/postruncmd
  - mkdir /tmp/postruncmd2
package_upgrade: false
`[1:]

var invalidCloudInitUserDataPackageInt = `
packages:
    - 76
postruncmd:
    - mkdir /tmp/runcmd
package_upgrade: true
`[1:]

var invalidCloudInitUserDataRunCmd = `
packages:
    - 'string1'
    - 'string2'
runcmd:
    - mkdir /tmp/runcmd
package_upgrade: true
`[1:]

var invalidCloudInitUserDataBootCmd = `
packages:
    - 'string1'
    - 'string2'
bootcmd:
    - mkdir /tmp/bootcmd
package_upgrade: true
`[1:]

var invalidCloudInitUserDataUsers = `
packages:
    - 'string1'
    - 'string2'
users:
    name: test-user
package_upgrade: true
`[1:]

var invalidCloudInitUserDataInvalidYAML = `
packages:
    - 'string1'
     'string2'
runcmd:
    - mkdir /tmp/runcmd
package_upgrade: true
`[1:]
