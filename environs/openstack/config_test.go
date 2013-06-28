// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package openstack

import (
	. "launchpad.net/gocheck"
	"launchpad.net/goyaml"
	"launchpad.net/juju-core/environs"
	"launchpad.net/juju-core/environs/config"
	"os"
)

type ConfigSuite struct {
	savedVars   map[string]string
	oldJujuHome string
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

var _ = Suite(&ConfigSuite{})

// configTest specifies a config parsing test, checking that env when
// parsed as the openstack section of a config file matches
// baseConfigResult when mutated by the mutate function, or that the
// parse matches the given error.
type configTest struct {
	summary       string
	config        attrs
	change        attrs
	expect        attrs
	envVars       map[string]string
	region        string
	controlBucket string
	publicBucket  string
	pbucketURL    string
	useFloatingIP bool
	username      string
	password      string
	tenantName    string
	authMode      string
	authURL       string
	accessKey     string
	secretKey     string
	firewallMode  config.FirewallMode
	err           string
}

type attrs map[string]interface{}

func restoreEnvVars(envVars map[string]string) {
	for k, v := range envVars {
		os.Setenv(k, v)
	}
}

func (t configTest) check(c *C) {
	envs := attrs{
		"environments": attrs{
			"testenv": attrs{
				"type":            "openstack",
				"authorized-keys": "fakekey",
			},
		},
	}
	testenv := envs["environments"].(attrs)["testenv"].(attrs)
	for k, v := range t.config {
		testenv[k] = v
	}
	if _, ok := testenv["control-bucket"]; !ok {
		testenv["control-bucket"] = "x"
	}
	data, err := goyaml.Marshal(envs)
	c.Assert(err, IsNil)

	es, err := environs.ReadEnvironsBytes(data)
	c.Check(err, IsNil)

	// Set environment variables if any.
	savedVars := make(map[string]string)
	if t.envVars != nil {
		for k, v := range t.envVars {
			savedVars[k] = os.Getenv(k)
			os.Setenv(k, v)
		}
	}
	defer restoreEnvVars(savedVars)

	e, err := es.Open("testenv")
	if t.change != nil {
		c.Assert(err, IsNil)

		// Testing a change in configuration.
		var old, changed, valid *config.Config
		osenv := e.(*environ)
		old = osenv.ecfg().Config
		changed, err = old.Apply(t.change)
		c.Assert(err, IsNil)

		// Keep err for validation below.
		valid, err = providerInstance.Validate(changed, old)
		if err == nil {
			err = osenv.SetConfig(valid)
		}
	}
	if t.err != "" {
		c.Check(err, ErrorMatches, t.err)
		return
	}
	c.Assert(err, IsNil)

	ecfg := e.(*environ).ecfg()
	c.Assert(ecfg.Name(), Equals, "testenv")
	c.Assert(ecfg.controlBucket(), Equals, "x")
	if t.region != "" {
		c.Assert(ecfg.region(), Equals, t.region)
	}
	if t.authMode != "" {
		c.Assert(ecfg.authMode(), Equals, t.authMode)
	}
	if t.accessKey != "" {
		c.Assert(ecfg.accessKey(), Equals, t.accessKey)
	}
	if t.secretKey != "" {
		c.Assert(ecfg.secretKey(), Equals, t.secretKey)
	}
	if t.username != "" {
		c.Assert(ecfg.username(), Equals, t.username)
		c.Assert(ecfg.password(), Equals, t.password)
		c.Assert(ecfg.tenantName(), Equals, t.tenantName)
		c.Assert(ecfg.authURL(), Equals, t.authURL)
		expected := map[string]interface{}{
			"username":    t.username,
			"password":    t.password,
			"tenant-name": t.tenantName,
		}
		c.Assert(err, IsNil)
		actual, err := e.Provider().SecretAttrs(ecfg.Config)
		c.Assert(err, IsNil)
		c.Assert(expected, DeepEquals, actual)
	}
	if t.pbucketURL != "" {
		c.Assert(ecfg.publicBucketURL(), Equals, t.pbucketURL)
		c.Assert(ecfg.publicBucket(), Equals, t.publicBucket)
	}
	if t.firewallMode != "" {
		c.Assert(ecfg.FirewallMode(), Equals, t.firewallMode)
	}
	c.Assert(ecfg.useFloatingIP(), Equals, t.useFloatingIP)
	for name, expect := range t.expect {
		actual, found := ecfg.UnknownAttrs()[name]
		c.Check(found, Equals, true)
		c.Check(actual, Equals, expect)
	}
}

func (s *ConfigSuite) SetUpTest(c *C) {
	s.oldJujuHome = config.SetJujuHome(c.MkDir())
	s.savedVars = make(map[string]string)
	for v, val := range envVars {
		s.savedVars[v] = os.Getenv(v)
		os.Setenv(v, val)
	}
}

func (s *ConfigSuite) TearDownTest(c *C) {
	for k, v := range s.savedVars {
		os.Setenv(k, v)
	}
	config.SetJujuHome(s.oldJujuHome)
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
		err: ".*expected string, got 666",
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
		err: ".*expected string, got 666",
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
		err: ".*expected string, got 666",
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
		err: ".*expected string, got 666",
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
		err: ".*expected string, got 666",
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
		err: ".*expected string, got 666",
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
		summary: "public bucket URL",
		config: attrs{
			"public-bucket":     "juju-dist-non-default",
			"public-bucket-url": "http://some/url",
		},
		publicBucket: "juju-dist-non-default",
		pbucketURL:   "http://some/url",
	}, {
		summary: "public bucket URL with default bucket",
		config: attrs{
			"public-bucket-url": "http://some/url",
		},
		publicBucket: "juju-dist",
		pbucketURL:   "http://some/url",
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
		summary: "unset firewall-mode",
		config: attrs{
			"firewall-mode": "",
		},
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
	},
}

func (s *ConfigSuite) TestConfig(c *C) {
	s.setupEnvCredentials()
	for i, t := range configTests {
		c.Logf("test %d: %s (%v)", i, t.summary, t.config)
		t.check(c)
	}
}

func (s *ConfigSuite) setupEnvCredentials() {
	os.Setenv("OS_USERNAME", "user")
	os.Setenv("OS_PASSWORD", "secret")
	os.Setenv("OS_AUTH_URL", "http://auth")
	os.Setenv("OS_TENANT_NAME", "sometenant")
	os.Setenv("OS_REGION_NAME", "region")
}
