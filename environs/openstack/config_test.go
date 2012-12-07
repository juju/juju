package openstack

import (
	. "launchpad.net/gocheck"
	"launchpad.net/goyaml"
	"launchpad.net/juju-core/environs"
	"launchpad.net/juju-core/environs/config"
	coretesting "launchpad.net/juju-core/testing"
	"os"
	"testing"
)

type ConfigSuite struct {
	savedVars map[string]string
}

var envVars = map[string]string{
	"OS_USERNAME":    "testuser",
	"OS_PASSWORD":    "testpass",
	"OS_TENANT_NAME": "testtenant",
	"OS_AUTH_URL":    "http://somehost",
	"OS_REGION_NAME": "testreg",
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
	username      string
	password      string
	tenantName    string
	authURL       string
	firewallMode  config.FirewallMode
	err           string
}

type attrs map[string]interface{}

func (t configTest) check(c *C) {
	envs := attrs{
		"environments": attrs{
			"testenv": attrs{
				"type":            "openstack",
				"authorized-keys": "foo",
				"ca-cert":         coretesting.CACert,
				"ca-private-key":  coretesting.CAKey,
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
			"auth-url":    "http://some/url",
		},
		username:   "jujuer",
		password:   "open sesame",
		tenantName: "juju tenant",
		authURL:    "http://some/url",
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
	for i, t := range configTests {
		c.Logf("test %d: %s (%v)", i, t.summary, t.config)
		t.check(c)
	}
}

func (s *ConfigSuite) TestMissingRegion(c *C) {
	os.Setenv("OS_REGION_NAME", "")
	os.Setenv("NOVA_REGION_NAME", "")
	test := configTests[0]
	delete(test.config, "region")
	test.err = "required environment variable not set for credentials attribute: Region"
	test.check(c)
}

func (s *ConfigSuite) TestMissingUsername(c *C) {
	os.Setenv("OS_USERNAME", "")
	os.Setenv("NOVA_USERNAME", "")
	test := configTests[0]
	test.err = "required environment variable not set for credentials attribute: User"
	test.check(c)
}

func (s *ConfigSuite) TestMissingPassword(c *C) {
	os.Setenv("OS_PASSWORD", "")
	os.Setenv("NOVA_PASSWORD", "")
	test := configTests[0]
	test.err = "required environment variable not set for credentials attribute: Secrets"
	test.check(c)
}

func (s *ConfigSuite) TestMissinTenant(c *C) {
	os.Setenv("OS_TENANT_NAME", "")
	os.Setenv("NOVA_PROJECT_ID", "")
	test := configTests[0]
	test.err = "required environment variable not set for credentials attribute: TenantName"
	test.check(c)
}

func (s *ConfigSuite) TestMissingAuthUrl(c *C) {
	os.Setenv("OS_AUTH_URL", "")
	test := configTests[0]
	test.err = "required environment variable not set for credentials attribute: URL"
	test.check(c)
}
