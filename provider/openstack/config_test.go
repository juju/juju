// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package openstack

import (
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/cloud"
	"github.com/juju/juju/environs"
	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/testing"
)

type ConfigSuite struct {
	testing.BaseSuite
}

var _ = gc.Suite(&ConfigSuite{})

// configTest specifies a config parsing test, checking that env when
// parsed as the openstack section of a config file matches
// baseConfigResult when mutated by the mutate function, or that the
// parse matches the given error.
type configTest struct {
	summary                 string
	config                  testing.Attrs
	change                  map[string]interface{}
	expect                  map[string]interface{}
	region                  string
	useFloatingIP           bool
	useDefaultSecurityGroup bool
	network                 string
	firewallMode            string
	err                     string
	sslHostnameVerification bool
	sslHostnameSet          bool
	blockStorageSource      string
}

var requiredConfig = testing.Attrs{}

func (t configTest) check(c *gc.C) {
	attrs := testing.FakeConfig().Merge(testing.Attrs{
		"type": "openstack",
	}).Merge(t.config)

	cfg, err := config.New(config.NoDefaults, attrs)
	c.Assert(err, jc.ErrorIsNil)

	credential := cloud.NewCredential(cloud.UserPassAuthType, map[string]string{
		"username":    "user",
		"password":    "secret",
		"tenant-name": "sometenant",
	})
	cloudSpec := environs.CloudSpec{
		Type:       "openstack",
		Name:       "openstack",
		Endpoint:   "http://auth",
		Region:     "Configtest",
		Credential: &credential,
	}

	e, err := environs.New(environs.OpenParams{
		Cloud:  cloudSpec,
		Config: cfg,
	})
	if t.change != nil {
		c.Assert(err, jc.ErrorIsNil)

		// Testing a change in configuration.
		var old, changed, valid *config.Config
		osenv := e.(*Environ)
		old = osenv.ecfg().Config
		changed, err = old.Apply(t.change)
		c.Assert(err, jc.ErrorIsNil)

		// Keep err for validation below.
		valid, err = providerInstance.Validate(changed, old)
		if err == nil {
			err = osenv.SetConfig(valid)
		}
	}
	if t.err != "" {
		c.Check(err, gc.ErrorMatches, t.err)
		return
	}
	c.Assert(err, jc.ErrorIsNil)

	ecfg := e.(*Environ).ecfg()
	c.Assert(ecfg.Name(), gc.Equals, "testenv")
	if t.firewallMode != "" {
		c.Assert(ecfg.FirewallMode(), gc.Equals, t.firewallMode)
	}
	c.Assert(ecfg.useFloatingIP(), gc.Equals, t.useFloatingIP)
	c.Assert(ecfg.useDefaultSecurityGroup(), gc.Equals, t.useDefaultSecurityGroup)
	c.Assert(ecfg.network(), gc.Equals, t.network)
	// Default should be true
	expectedHostnameVerification := true
	if t.sslHostnameSet {
		expectedHostnameVerification = t.sslHostnameVerification
	}
	c.Assert(ecfg.SSLHostnameVerification(), gc.Equals, expectedHostnameVerification)
	for name, expect := range t.expect {
		actual, found := ecfg.UnknownAttrs()[name]
		c.Check(found, jc.IsTrue)
		c.Check(actual, gc.Equals, expect)
	}
	if t.blockStorageSource != "" {
		storage, ok := ecfg.StorageDefaultBlockSource()
		c.Assert(ok, jc.IsTrue)
		c.Assert(storage, gc.Equals, t.blockStorageSource)
	}
}

func (s *ConfigSuite) SetUpTest(c *gc.C) {
	s.BaseSuite.SetUpTest(c)
	s.PatchValue(&authenticateClient, func(*Environ) error { return nil })
}

var configTests = []configTest{
	{
		summary: "default use floating ip",
		config:  requiredConfig,
		// Do not use floating IP's by default.
		useFloatingIP: false,
	}, {
		summary: "use floating ip",
		config: requiredConfig.Merge(testing.Attrs{
			"use-floating-ip": true,
		}),
		useFloatingIP: true,
	}, {
		summary: "default use default security group",
		config:  requiredConfig,
		// Do not use default security group by default.
		useDefaultSecurityGroup: false,
	}, {
		summary: "use default security group",
		config: requiredConfig.Merge(testing.Attrs{
			"use-default-secgroup": true,
		}),
		useDefaultSecurityGroup: true,
	}, {
		summary: "admin-secret given",
		config: requiredConfig.Merge(testing.Attrs{
			"admin-secret": "Futumpsh",
		}),
	}, {
		summary:      "default firewall-mode",
		config:       requiredConfig,
		firewallMode: config.FwInstance,
	}, {
		summary: "instance firewall-mode",
		config: requiredConfig.Merge(testing.Attrs{
			"firewall-mode": "instance",
		}),
		firewallMode: config.FwInstance,
	}, {
		summary: "global firewall-mode",
		config: requiredConfig.Merge(testing.Attrs{
			"firewall-mode": "global",
		}),
		firewallMode: config.FwGlobal,
	}, {
		summary: "none firewall-mode",
		config: requiredConfig.Merge(testing.Attrs{
			"firewall-mode": "none",
		}),
		firewallMode: config.FwNone,
	}, {
		config: requiredConfig.Merge(testing.Attrs{
			"future": "hammerstein",
		}),
		expect: testing.Attrs{
			"future": "hammerstein",
		},
	}, {
		config: requiredConfig,
		change: testing.Attrs{
			"future": "hammerstein",
		},
		expect: testing.Attrs{
			"future": "hammerstein",
		},
	}, {
		config: requiredConfig,
		change: testing.Attrs{
			"ssl-hostname-verification": false,
		},
		sslHostnameVerification: false,
		sslHostnameSet:          true,
	}, {
		config: requiredConfig,
		change: testing.Attrs{
			"ssl-hostname-verification": true,
		},
		sslHostnameVerification: true,
		sslHostnameSet:          true,
	}, {
		summary: "default network",
		config:  requiredConfig,
		network: "",
	}, {
		summary: "network",
		config: requiredConfig.Merge(testing.Attrs{
			"network": "a-network-label",
		}),
		network: "a-network-label",
	}, {
		summary: "block storage specified",
		config: requiredConfig.Merge(testing.Attrs{
			"storage-default-block-source": "my-cinder",
		}),
		blockStorageSource: "my-cinder",
	},
}

func (s *ConfigSuite) TestConfig(c *gc.C) {
	for i, t := range configTests {
		c.Logf("test %d: %s (%v)", i, t.summary, t.config)
		t.check(c)
	}
}

func (s *ConfigSuite) TestDeprecatedAttributesRemoved(c *gc.C) {
	attrs := testing.FakeConfig().Merge(testing.Attrs{
		"type":                  "openstack",
		"default-image-id":      "id-1234",
		"default-instance-type": "big",
	})

	cfg, err := config.New(config.NoDefaults, attrs)
	c.Assert(err, jc.ErrorIsNil)
	// Keep err for validation below.
	valid, err := providerInstance.Validate(cfg, nil)
	c.Assert(err, jc.ErrorIsNil)
	// Check deprecated attributes removed.
	allAttrs := valid.AllAttrs()
	for _, attr := range []string{"default-image-id", "default-instance-type"} {
		_, ok := allAttrs[attr]
		c.Assert(ok, jc.IsFalse)
	}
}

func (s *ConfigSuite) TestPrepareConfigSetsDefaultBlockSource(c *gc.C) {
	attrs := testing.FakeConfig().Merge(testing.Attrs{
		"type": "openstack",
	})
	cfg, err := config.New(config.NoDefaults, attrs)
	c.Assert(err, jc.ErrorIsNil)
	_, ok := cfg.StorageDefaultBlockSource()
	c.Assert(ok, jc.IsFalse)

	cfg, err = providerInstance.PrepareConfig(prepareConfigParams(cfg))
	c.Assert(err, jc.ErrorIsNil)
	source, ok := cfg.StorageDefaultBlockSource()
	c.Assert(ok, jc.IsTrue)
	c.Assert(source, gc.Equals, "cinder")
}

func prepareConfigParams(cfg *config.Config) environs.PrepareConfigParams {
	credential := cloud.NewCredential(cloud.UserPassAuthType, map[string]string{
		"username":    "user",
		"password":    "secret",
		"tenant-name": "sometenant",
	})
	return environs.PrepareConfigParams{
		Config: cfg,
		Cloud: environs.CloudSpec{
			Type:       "openstack",
			Name:       "canonistack",
			Region:     "region",
			Endpoint:   "http://auth",
			Credential: &credential,
		},
	}
}

func (*ConfigSuite) TestSchema(c *gc.C) {
	fields := providerInstance.Schema()
	// Check that all the fields defined in environs/config
	// are in the returned schema.
	globalFields, err := config.Schema(nil)
	c.Assert(err, gc.IsNil)
	for name, field := range globalFields {
		c.Check(fields[name], jc.DeepEquals, field)
	}
}
