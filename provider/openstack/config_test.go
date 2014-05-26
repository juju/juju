// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package openstack

import (
	"os"
	"time"

	"github.com/juju/loggo"
	jc "github.com/juju/testing/checkers"
	gc "launchpad.net/gocheck"

	"launchpad.net/juju-core/environs"
	"launchpad.net/juju-core/environs/config"
	"launchpad.net/juju-core/testing"
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
	config                  map[string]interface{}
	change                  map[string]interface{}
	expect                  map[string]interface{}
	envVars                 map[string]string
	region                  string
	controlBucket           string
	useFloatingIP           bool
	useDefaultSecurityGroup bool
	network                 string
	username                string
	password                string
	tenantName              string
	authMode                string
	authURL                 string
	accessKey               string
	secretKey               string
	firewallMode            string
	err                     string
	sslHostnameVerification bool
	sslHostnameSet          bool
}

type attrs map[string]interface{}

func restoreEnvVars(envVars map[string]string) {
	for k, v := range envVars {
		os.Setenv(k, v)
	}
}

func (t configTest) check(c *gc.C) {
	attrs := testing.FakeConfig().Merge(testing.Attrs{
		"type":           "openstack",
		"control-bucket": "x",
	}).Merge(t.config)

	cfg, err := config.New(config.NoDefaults, attrs)
	c.Assert(err, gc.IsNil)

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
		c.Assert(err, gc.IsNil)

		// Testing a change in configuration.
		var old, changed, valid *config.Config
		osenv := e.(*environ)
		old = osenv.ecfg().Config
		changed, err = old.Apply(t.change)
		c.Assert(err, gc.IsNil)

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
	c.Assert(err, gc.IsNil)

	ecfg := e.(*environ).ecfg()
	c.Assert(ecfg.Name(), gc.Equals, "testenv")
	c.Assert(ecfg.controlBucket(), gc.Equals, "x")
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
		c.Assert(err, gc.IsNil)
		actual, err := e.Provider().SecretAttrs(ecfg.Config)
		c.Assert(err, gc.IsNil)
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
		c.Check(found, gc.Equals, true)
		c.Check(actual, gc.Equals, expect)
	}
}

func (s *ConfigSuite) SetUpTest(c *gc.C) {
	s.BaseSuite.SetUpTest(c)
	s.savedVars = make(map[string]string)
	for v, val := range envVars {
		s.savedVars[v] = os.Getenv(v)
		os.Setenv(v, val)
	}
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
		config: attrs{
			"region": "testreg",
		},
		region: "testreg",
	}, {
		summary: "setting region (2)",
		config: attrs{
			"region": "configtest",
		},
		region: "configtest",
	}, {
		summary: "changing region",
		config: attrs{
			"region": "configtest",
		},
		change: attrs{
			"region": "somereg",
		},
		err: `cannot change region from "configtest" to "somereg"`,
	}, {
		summary: "invalid region",
		config: attrs{
			"region": 666,
		},
		err: `.*expected string, got int\(666\)`,
	}, {
		summary: "missing region in environment",
		envVars: map[string]string{
			"OS_REGION_NAME": "",
			"NOVA_REGION":    "",
		},
		err: "required environment variable not set for credentials attribute: Region",
	}, {
		summary: "invalid username",
		config: attrs{
			"username": 666,
		},
		err: `.*expected string, got int\(666\)`,
	}, {
		summary: "missing username in environment",
		err:     "required environment variable not set for credentials attribute: User",
		envVars: map[string]string{
			"OS_USERNAME":   "",
			"NOVA_USERNAME": "",
		},
	}, {
		summary: "invalid password",
		config: attrs{
			"password": 666,
		},
		err: `.*expected string, got int\(666\)`,
	}, {
		summary: "missing password in environment",
		err:     "required environment variable not set for credentials attribute: Secrets",
		envVars: map[string]string{
			"OS_PASSWORD":   "",
			"NOVA_PASSWORD": "",
		},
	}, {
		summary: "invalid tenant-name",
		config: attrs{
			"tenant-name": 666,
		},
		err: `.*expected string, got int\(666\)`,
	}, {
		summary: "missing tenant in environment",
		err:     "required environment variable not set for credentials attribute: TenantName",
		envVars: map[string]string{
			"OS_TENANT_NAME":  "",
			"NOVA_PROJECT_ID": "",
		},
	}, {
		summary: "invalid auth-url type",
		config: attrs{
			"auth-url": 666,
		},
		err: `.*expected string, got int\(666\)`,
	}, {
		summary: "missing auth-url in environment",
		err:     "required environment variable not set for credentials attribute: URL",
		envVars: map[string]string{
			"OS_AUTH_URL": "",
		},
	}, {
		summary: "invalid authorization mode",
		config: attrs{
			"auth-mode": "invalid-mode",
		},
		err: ".*invalid authorization mode.*",
	}, {
		summary: "keypair authorization mode",
		config: attrs{
			"auth-mode":  "keypair",
			"access-key": "MyAccessKey",
			"secret-key": "MySecretKey",
		},
		authMode:  "keypair",
		accessKey: "MyAccessKey",
		secretKey: "MySecretKey",
	}, {
		summary: "keypair authorization mode without access key",
		config: attrs{
			"auth-mode":  "keypair",
			"secret-key": "MySecretKey",
		},
		envVars: map[string]string{
			"OS_USERNAME": "",
		},
		err: "required environment variable not set for credentials attribute: User",
	}, {
		summary: "keypair authorization mode without secret key",
		config: attrs{
			"auth-mode":  "keypair",
			"access-key": "MyAccessKey",
		},
		envVars: map[string]string{
			"OS_PASSWORD": "",
		},
		err: "required environment variable not set for credentials attribute: Secrets",
	}, {
		summary: "invalid auth-url format",
		config: attrs{
			"auth-url": "invalid",
		},
		err: `invalid auth-url value "invalid"`,
	}, {
		summary: "invalid control-bucket",
		config: attrs{
			"control-bucket": 666,
		},
		err: `.*expected string, got int\(666\)`,
	}, {
		summary: "changing control-bucket",
		change: attrs{
			"control-bucket": "new-x",
		},
		err: `cannot change control-bucket from "x" to "new-x"`,
	}, {
		summary: "valid auth args",
		config: attrs{
			"username":    "jujuer",
			"password":    "open sesame",
			"tenant-name": "juju tenant",
			"auth-mode":   "legacy",
			"auth-url":    "http://some/url",
		},
		username:   "jujuer",
		password:   "open sesame",
		tenantName: "juju tenant",
		authURL:    "http://some/url",
		authMode:   string(AuthLegacy),
	}, {
		summary: "valid auth args in environment",
		envVars: map[string]string{
			"OS_USERNAME":    "jujuer",
			"OS_PASSWORD":    "open sesame",
			"OS_AUTH_URL":    "http://some/url",
			"OS_TENANT_NAME": "juju tenant",
			"OS_REGION_NAME": "region",
		},
		username:   "jujuer",
		password:   "open sesame",
		tenantName: "juju tenant",
		authURL:    "http://some/url",
		region:     "region",
	}, {
		summary:  "default auth mode based on environment",
		authMode: string(AuthUserPass),
	}, {
		summary: "default use floating ip",
		// Do not use floating IP's by default.
		useFloatingIP: false,
	}, {
		summary: "use floating ip",
		config: attrs{
			"use-floating-ip": true,
		},
		useFloatingIP: true,
	}, {
		summary: "default use default security group",
		// Do not use default security group by default.
		useDefaultSecurityGroup: false,
	}, {
		summary: "use default security group",
		config: attrs{
			"use-default-secgroup": true,
		},
		useDefaultSecurityGroup: true,
	}, {
		summary: "admin-secret given",
		config: attrs{
			"admin-secret": "Futumpsh",
		},
	}, {
		summary:      "default firewall-mode",
		config:       attrs{},
		firewallMode: config.FwInstance,
	}, {
		summary: "instance firewall-mode",
		config: attrs{
			"firewall-mode": "instance",
		},
		firewallMode: config.FwInstance,
	}, {
		summary: "global firewall-mode",
		config: attrs{
			"firewall-mode": "global",
		},
		firewallMode: config.FwGlobal,
	}, {
		config: attrs{
			"future": "hammerstein",
		},
		expect: attrs{
			"future": "hammerstein",
		},
	}, {
		change: attrs{
			"future": "hammerstein",
		},
		expect: attrs{
			"future": "hammerstein",
		},
	}, {
		change: attrs{
			"ssl-hostname-verification": false,
		},
		sslHostnameVerification: false,
		sslHostnameSet:          true,
	}, {
		change: attrs{
			"ssl-hostname-verification": true,
		},
		sslHostnameVerification: true,
		sslHostnameSet:          true,
	}, {
		summary: "default network",
		network: "",
	}, {
		summary: "network",
		config: attrs{
			"network": "a-network-label",
		},
		network: "a-network-label",
	},
}

func (s *ConfigSuite) TestConfig(c *gc.C) {
	s.setupEnvCredentials()
	for i, t := range configTests {
		c.Logf("test %d: %s (%v)", i, t.summary, t.config)
		t.check(c)
	}
}

func (s *ConfigSuite) TestDeprecatedAttributesRemoved(c *gc.C) {
	s.setupEnvCredentials()
	attrs := testing.FakeConfig().Merge(testing.Attrs{
		"type":                  "openstack",
		"control-bucket":        "x",
		"default-image-id":      "id-1234",
		"default-instance-type": "big",
	})

	cfg, err := config.New(config.NoDefaults, attrs)
	c.Assert(err, gc.IsNil)
	// Keep err for validation below.
	valid, err := providerInstance.Validate(cfg, nil)
	c.Assert(err, gc.IsNil)
	// Check deprecated attributes removed.
	allAttrs := valid.AllAttrs()
	for _, attr := range []string{"default-image-id", "default-instance-type"} {
		_, ok := allAttrs[attr]
		c.Assert(ok, jc.IsFalse)
	}
}

func (s *ConfigSuite) TestPrepareInsertsUniqueControlBucket(c *gc.C) {
	s.setupEnvCredentials()
	attrs := testing.FakeConfig().Merge(testing.Attrs{
		"type": "openstack",
	})
	cfg, err := config.New(config.NoDefaults, attrs)
	c.Assert(err, gc.IsNil)

	ctx := testing.Context(c)
	env0, err := providerInstance.Prepare(ctx, cfg)
	c.Assert(err, gc.IsNil)
	bucket0 := env0.(*environ).ecfg().controlBucket()
	c.Assert(bucket0, gc.Matches, "[a-f0-9]{32}")

	env1, err := providerInstance.Prepare(ctx, cfg)
	c.Assert(err, gc.IsNil)
	bucket1 := env1.(*environ).ecfg().controlBucket()
	c.Assert(bucket1, gc.Matches, "[a-f0-9]{32}")

	c.Assert(bucket1, gc.Not(gc.Equals), bucket0)
}

func (s *ConfigSuite) TestPrepareDoesNotTouchExistingControlBucket(c *gc.C) {
	s.setupEnvCredentials()
	attrs := testing.FakeConfig().Merge(testing.Attrs{
		"type":           "openstack",
		"control-bucket": "burblefoo",
	})
	cfg, err := config.New(config.NoDefaults, attrs)
	c.Assert(err, gc.IsNil)

	env, err := providerInstance.Prepare(testing.Context(c), cfg)
	c.Assert(err, gc.IsNil)
	bucket := env.(*environ).ecfg().controlBucket()
	c.Assert(bucket, gc.Equals, "burblefoo")
}

func (s *ConfigSuite) setupEnvCredentials() {
	os.Setenv("OS_USERNAME", "user")
	os.Setenv("OS_PASSWORD", "secret")
	os.Setenv("OS_AUTH_URL", "http://auth")
	os.Setenv("OS_TENANT_NAME", "sometenant")
	os.Setenv("OS_REGION_NAME", "region")
}

type ConfigDeprecationSuite struct {
	ConfigSuite
	writer *testWriter
}

var _ = gc.Suite(&ConfigDeprecationSuite{})

func (s *ConfigDeprecationSuite) SetUpTest(c *gc.C) {
	s.ConfigSuite.SetUpTest(c)
}

func (s *ConfigDeprecationSuite) TearDownTest(c *gc.C) {
	s.ConfigSuite.TearDownTest(c)
}

func (s *ConfigDeprecationSuite) setupLogger(c *gc.C) {
	var err error
	s.writer = &testWriter{}
	err = loggo.RegisterWriter("test", s.writer, loggo.WARNING)
	c.Assert(err, gc.IsNil)
}

func (s *ConfigDeprecationSuite) resetLogger(c *gc.C) {
	_, _, err := loggo.RemoveWriter("test")
	c.Assert(err, gc.IsNil)
}

type testWriter struct {
	messages []string
}

func (t *testWriter) Write(level loggo.Level, module, filename string, line int, timestamp time.Time, message string) {
	t.messages = append(t.messages, message)
}

func (s *ConfigDeprecationSuite) setupEnv(c *gc.C, deprecatedKey, value string) {
	s.setupEnvCredentials()
	attrs := testing.FakeConfig().Merge(testing.Attrs{
		"name":           "testenv",
		"type":           "openstack",
		"control-bucket": "x",
		deprecatedKey:    value,
	})
	_, err := environs.NewFromAttrs(attrs)
	c.Assert(err, gc.IsNil)
}
