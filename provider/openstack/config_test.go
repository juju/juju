// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package openstack

import (
	"os"

	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/cloud"
	"github.com/juju/juju/environs"
	"github.com/juju/juju/environs/config"
	envtesting "github.com/juju/juju/environs/testing"
	"github.com/juju/juju/testing"
)

type ConfigSuite struct {
	testing.BaseSuite
	savedVars map[string]string
}

// Ensure any environment variables a user may have set locally are reset.
var envVars = map[string]string{
	"AWS_SECRET_ACCESS_KEY": "",
	"EC2_SECRET_KEYS":       "",
	"NOVA_API_KEY":          "",
	"NOVA_PASSWORD":         "",
	"NOVA_PROJECT_ID":       "",
	"NOVA_REGION":           "",
	"NOVA_USERNAME":         "",
	"OS_ACCESS_KEY":         "",
	"OS_AUTH_URL":           "",
	"OS_PASSWORD":           "",
	"OS_REGION_NAME":        "",
	"OS_SECRET_KEY":         "",
	"OS_TENANT_NAME":        "",
	"OS_USERNAME":           "",
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
	envVars                 map[string]string
	region                  string
	useFloatingIP           bool
	useDefaultSecurityGroup bool
	network                 string
	username                string
	password                string
	tenantName              string
	authMode                AuthMode
	authURL                 string
	accessKey               string
	secretKey               string
	firewallMode            string
	err                     string
	sslHostnameVerification bool
	sslHostnameSet          bool
	blockStorageSource      string
}

var requiredConfig = testing.Attrs{
	"region":      "configtest",
	"auth-url":    "http://auth",
	"username":    "user",
	"password":    "pass",
	"tenant-name": "tenant",
}

func restoreEnvVars(envVars map[string]string) {
	for k, v := range envVars {
		os.Setenv(k, v)
	}
}

func (t configTest) check(c *gc.C) {
	attrs := testing.FakeConfig().Merge(testing.Attrs{
		"type": "openstack",
	}).Merge(t.config)

	cfg, err := config.New(config.NoDefaults, attrs)
	c.Assert(err, jc.ErrorIsNil)

	// Set environment variables if any.
	savedVars := make(map[string]string)
	if t.envVars != nil {
		for k, v := range t.envVars {
			savedVars[k] = os.Getenv(k)
			os.Setenv(k, v)
		}
	}
	defer restoreEnvVars(savedVars)

	e, err := environs.New(cfg)
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
	if t.region != "" {
		c.Assert(ecfg.region(), gc.Equals, t.region)
	}
	if t.authMode != "" {
		c.Assert(ecfg.authMode(), gc.Equals, t.authMode)
	}
	if t.accessKey != "" {
		c.Assert(ecfg.accessKey(), gc.Equals, t.accessKey)
	}
	if t.secretKey != "" {
		c.Assert(ecfg.secretKey(), gc.Equals, t.secretKey)
	}
	if t.username != "" {
		c.Assert(ecfg.username(), gc.Equals, t.username)
		c.Assert(ecfg.password(), gc.Equals, t.password)
		c.Assert(ecfg.tenantName(), gc.Equals, t.tenantName)
		c.Assert(ecfg.authURL(), gc.Equals, t.authURL)
		expected := map[string]string{
			"username":    t.username,
			"password":    t.password,
			"tenant-name": t.tenantName,
		}
		c.Assert(err, jc.ErrorIsNil)
		actual, err := e.Provider().SecretAttrs(ecfg.Config)
		c.Assert(err, jc.ErrorIsNil)
		c.Assert(expected, gc.DeepEquals, actual)
	}
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
	expectedStorage := "cinder"
	if t.blockStorageSource != "" {
		expectedStorage = t.blockStorageSource
	}
	storage, ok := ecfg.StorageDefaultBlockSource()
	c.Assert(ok, jc.IsTrue)
	c.Assert(storage, gc.Equals, expectedStorage)
}

func (s *ConfigSuite) SetUpTest(c *gc.C) {
	s.BaseSuite.SetUpTest(c)
	s.savedVars = make(map[string]string)
	for v, val := range envVars {
		s.savedVars[v] = os.Getenv(v)
		os.Setenv(v, val)
	}
	s.PatchValue(&authenticateClient, func(*Environ) error { return nil })
}

func (s *ConfigSuite) TearDownTest(c *gc.C) {
	for k, v := range s.savedVars {
		os.Setenv(k, v)
	}
	s.BaseSuite.TearDownTest(c)
}

var configTests = []configTest{
	{
		summary: "setting region",
		config: requiredConfig.Merge(testing.Attrs{
			"region": "testreg",
		}),
		region: "testreg",
	}, {
		summary: "setting region (2)",
		config: requiredConfig.Merge(testing.Attrs{
			"region": "configtest",
		}),
		region: "configtest",
	}, {
		summary: "changing region",
		config:  requiredConfig,
		change: testing.Attrs{
			"region": "otherregion",
		},
		err: `cannot change region from "configtest" to "otherregion"`,
	}, {
		summary: "invalid region",
		config: requiredConfig.Merge(testing.Attrs{
			"region": 666,
		}),
		err: `.*expected string, got int\(666\)`,
	}, {
		summary: "missing region in model",
		config:  requiredConfig.Delete("region"),
		err:     "missing region not valid",
	}, {
		summary: "invalid username",
		config: requiredConfig.Merge(testing.Attrs{
			"username": 666,
		}),
		err: `.*expected string, got int\(666\)`,
	}, {
		summary: "missing username in model",
		config:  requiredConfig.Delete("username"),
		err:     "missing username not valid",
	}, {
		summary: "invalid password",
		config: requiredConfig.Merge(testing.Attrs{
			"password": 666,
		}),
		err: `.*expected string, got int\(666\)`,
	}, {
		summary: "missing password in model",
		config:  requiredConfig.Delete("password"),
		err:     "missing password not valid",
	}, {
		summary: "invalid tenant-name",
		config: requiredConfig.Merge(testing.Attrs{
			"tenant-name": 666,
		}),
		err: `.*expected string, got int\(666\)`,
	}, {
		summary: "missing tenant in model",
		config:  requiredConfig.Delete("tenant-name"),
		err:     "missing tenant-name not valid",
	}, {
		summary: "invalid auth-url type",
		config: requiredConfig.Merge(testing.Attrs{
			"auth-url": 666,
		}),
		err: `.*expected string, got int\(666\)`,
	}, {
		summary: "missing auth-url in model",
		config:  requiredConfig.Delete("auth-url"),
		err:     "missing auth-url not valid",
	}, {
		summary: "invalid authorization mode",
		config: requiredConfig.Merge(testing.Attrs{
			"auth-mode": "invalid-mode",
		}),
		err: `auth-mode: expected one of \[keypair legacy userpass\], got "invalid-mode"`,
	}, {
		summary: "keypair authorization mode",
		config: requiredConfig.Merge(testing.Attrs{
			"auth-mode":  "keypair",
			"access-key": "MyAccessKey",
			"secret-key": "MySecretKey",
		}),
		authMode:  "keypair",
		accessKey: "MyAccessKey",
		secretKey: "MySecretKey",
	}, {
		summary: "keypair authorization mode without access key",
		config: requiredConfig.Merge(testing.Attrs{
			"auth-mode":  "keypair",
			"secret-key": "MySecretKey",
		}),
		err: "missing access-key not valid",
	}, {
		summary: "keypair authorization mode without secret key",
		config: requiredConfig.Merge(testing.Attrs{
			"auth-mode":  "keypair",
			"access-key": "MyAccessKey",
		}),
		err: "missing secret-key not valid",
	}, {
		summary: "invalid auth-url format",
		config: requiredConfig.Merge(testing.Attrs{
			"auth-url": "invalid",
		}),
		err: `invalid auth-url value "invalid"`,
	}, {
		summary: "valid auth args",
		config: requiredConfig.Merge(testing.Attrs{
			"username":    "jujuer",
			"password":    "open sesame",
			"tenant-name": "juju tenant",
			"auth-mode":   "legacy",
			"auth-url":    "http://some/url",
		}),
		username:   "jujuer",
		password:   "open sesame",
		tenantName: "juju tenant",
		authURL:    "http://some/url",
		authMode:   AuthLegacy,
	}, {
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
		summary:            "no default block storage specified",
		config:             requiredConfig,
		blockStorageSource: "cinder",
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
		"username":              "u",
		"password":              "p",
		"tenant-name":           "t",
		"region":                "r",
		"auth-url":              "http://auth",
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

func (s *ConfigSuite) TestPrepareSetsDefaultBlockSource(c *gc.C) {
	attrs := testing.FakeConfig().Merge(testing.Attrs{
		"type": "openstack",
	})
	cfg, err := config.New(config.NoDefaults, attrs)
	c.Assert(err, jc.ErrorIsNil)

	env, err := providerInstance.PrepareForBootstrap(envtesting.BootstrapContext(c), s.prepareForBootstrapParams(cfg))
	c.Assert(err, jc.ErrorIsNil)
	source, ok := env.(*Environ).ecfg().StorageDefaultBlockSource()
	c.Assert(ok, jc.IsTrue)
	c.Assert(source, gc.Equals, "cinder")
}

func (s *ConfigSuite) prepareForBootstrapParams(cfg *config.Config) environs.PrepareForBootstrapParams {
	return environs.PrepareForBootstrapParams{
		Config: cfg,
		Credentials: cloud.NewCredential(cloud.UserPassAuthType, map[string]string{
			"username":    "user",
			"password":    "secret",
			"tenant-name": "sometenant",
		}),
		CloudRegion:   "region",
		CloudEndpoint: "http://auth",
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
