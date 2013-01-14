package openstack

import (
	. "launchpad.net/gocheck"
	"launchpad.net/goyaml"
	"launchpad.net/juju-core/environs"
	"launchpad.net/juju-core/environs/config"
	"os"
)

type ConfigSuite struct {
	savedVars map[string]string
}

// Ensure any environment variables a user may have set locally are reset.
var envVars = map[string]string{
	"OS_USERNAME":     "",
	"OS_PASSWORD":     "",
	"OS_TENANT_NAME":  "",
	"OS_AUTH_URL":     "",
	"OS_REGION_NAME":  "",
	"NOVA_USERNAME":   "",
	"NOVA_PASSWORD":   "",
	"NOVA_PROJECT_ID": "",
	"NOVA_REGION":     "",
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
	region        string
	controlBucket string
	publicBucket  string
	pbucketURL    string
	username      string
	password      string
	tenantName    string
	authMethod    string
	authURL       string
	firewallMode  config.FirewallMode
	err           string
}

type attrs map[string]interface{}

func (t configTest) check(c *C) {
	envs := attrs{
		"environments": attrs{
			"testenv": attrs{
				"type": "openstack",
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
}

func (s *ConfigSuite) SetUpTest(c *C) {
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
		summary: "invalid username",
		config: attrs{
			"username": 666,
		},
		err: ".*expected string, got 666",
	}, {
		summary: "invalid password",
		config: attrs{
			"password": 666,
		},
		err: ".*expected string, got 666",
	}, {
		summary: "invalid tenant-name",
		config: attrs{
			"tenant-name": 666,
		},
		err: ".*expected string, got 666",
	}, {
		summary: "invalid auth-url type",
		config: attrs{
			"auth-url": 666,
		},
		err: ".*expected string, got 666",
	}, {
		summary: "invalid authorization method",
		config: attrs{
			"auth-method": "invalid-method",
		},
		err: ".*invalid authorization method.*",
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
			"auth-method": "legacy",
			"auth-url":    "http://some/url",
		},
		username:   "jujuer",
		password:   "open sesame",
		tenantName: "juju tenant",
		authURL:    "http://some/url",
		authMethod: "legacy",
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

var regionTestConfig = configTests[0]
var credentialsTestConfig = configTests[12]

func (s *ConfigSuite) TestMissingRegion(c *C) {
	s.setupEnvCredentials()
	os.Setenv("OS_REGION_NAME", "")
	os.Setenv("NOVA_REGION", "")
	test := credentialsTestConfig
	test.err = "required environment variable not set for credentials attribute: Region"
	test.check(c)
}

func (s *ConfigSuite) TestMissingUsername(c *C) {
	s.setupEnvCredentials()
	os.Setenv("OS_USERNAME", "")
	os.Setenv("NOVA_USERNAME", "")
	test := regionTestConfig
	test.err = "required environment variable not set for credentials attribute: User"
	test.check(c)
}

func (s *ConfigSuite) TestMissingPassword(c *C) {
	s.setupEnvCredentials()
	os.Setenv("OS_PASSWORD", "")
	os.Setenv("NOVA_PASSWORD", "")
	test := regionTestConfig
	test.err = "required environment variable not set for credentials attribute: Secrets"
	test.check(c)
}
func (s *ConfigSuite) TestMissingTenant(c *C) {
	s.setupEnvCredentials()
	os.Setenv("OS_TENANT_NAME", "")
	os.Setenv("NOVA_PROJECT_ID", "")
	test := regionTestConfig
	test.err = "required environment variable not set for credentials attribute: TenantName"
	test.check(c)
}

func (s *ConfigSuite) TestMissingAuthUrl(c *C) {
	s.setupEnvCredentials()
	os.Setenv("OS_AUTH_URL", "")
	test := regionTestConfig
	test.err = "required environment variable not set for credentials attribute: URL"
	test.check(c)
}

func (s *ConfigSuite) TestCredentialsFromEnv(c *C) {
	// Specify a basic configuration without credentials.
	envs := attrs{
		"environments": attrs{
			"testenv": attrs{
				"type": "openstack",
			},
		},
	}
	data, err := goyaml.Marshal(envs)
	c.Assert(err, IsNil)
	// Poke the credentials into the environment.
	s.setupEnvCredentials()
	es, err := environs.ReadEnvironsBytes(data)
	c.Check(err, IsNil)
	e, err := es.Open("testenv")
	ecfg := e.(*environ).ecfg()
	// The credentials below come from environment variables set during test setup.
	c.Assert(ecfg.username(), Equals, "user")
	c.Assert(ecfg.password(), Equals, "secret")
	c.Assert(ecfg.authURL(), Equals, "http://auth")
	c.Assert(ecfg.region(), Equals, "region")
	c.Assert(ecfg.tenantName(), Equals, "sometenant")
}

func (s *ConfigSuite) TestDefaultAuthorisationMethod(c *C) {
	// Specify a basic configuration without authorization method.
	envs := attrs{
		"environments": attrs{
			"testenv": attrs{
				"type": "openstack",
			},
		},
	}
	data, err := goyaml.Marshal(envs)
	c.Assert(err, IsNil)
	s.setupEnvCredentials()
	es, err := environs.ReadEnvironsBytes(data)
	c.Check(err, IsNil)
	e, err := es.Open("testenv")
	ecfg := e.(*environ).ecfg()
	c.Assert(ecfg.authMethod(), Equals, string(AuthUserPass))
}
