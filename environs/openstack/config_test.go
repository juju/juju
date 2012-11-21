package openstack

import (
	. "launchpad.net/gocheck"
	"launchpad.net/goyaml"
	"launchpad.net/juju-core/environs"
	"launchpad.net/juju-core/environs/config"
	"os"
	"testing"
)

type ConfigSuite struct {
	savedUsername, savedPassword, savedTenant, savedAuthURL string
}

var _ = Suite(&ConfigSuite{})

// Hook up gocheck into the gotest runner.
func Test(t *testing.T) { TestingT(t) }

// configTest specifies a config parsing test, checking that env when
// parsed as the openstack section of a config file matches
// baseConfigResult when mutated by the mutate function, or that the
// parse matches the given error.
type configTest struct {
	config       attrs
	change       attrs
	region       string
	container    string
	username     string
	password     string
	tenantId     string
	identityURL  string
	firewallMode config.FirewallMode
	err          string
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
	if _, ok := testenv["container"]; !ok {
		testenv["container"] = "x"
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
	c.Assert(ecfg.container(), Equals, "x")
	if t.region != "" {
		c.Assert(ecfg.region(), Equals, t.region)
	}
	if t.username != "" {
		c.Assert(ecfg.username(), Equals, t.username)
		c.Assert(ecfg.password(), Equals, t.password)
		c.Assert(ecfg.tenantId(), Equals, t.tenantId)
		c.Assert(ecfg.identityURL(), Equals, t.identityURL)
		expected := map[string]interface{}{
			"username":  t.username,
			"password":  t.password,
			"tenant-id": t.tenantId,
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
	s.savedUsername = os.Getenv("OS_USERNAME")
	s.savedPassword = os.Getenv("OS_PASSWORD")
	s.savedTenant = os.Getenv("OS_TENANT_NAME")
	s.savedAuthURL = os.Getenv("OS_AUTH_URL")

	os.Setenv("OS_USERNAME", "testuser")
	os.Setenv("OS_PASSWORD", "testpass")
	os.Setenv("OS_TENANT_NAME", "testtenant")
	os.Setenv("OS_AUTH_URL", "some url")
}

func (s *ConfigSuite) TearDownTest(c *C) {
	os.Setenv("OS_USERNAME", s.savedUsername)
	os.Setenv("OS_PASSWORD", s.savedPassword)
	os.Setenv("OS_TENANT_NAME", s.savedTenant)
	os.Setenv("OS_AUTH_URL", s.savedAuthURL)
}

var configTests = []configTest{
	{
		config: attrs{
			"region": "lcy01",
		},
		region: "lcy01",
	}, {
		config: attrs{
			"region": "configtest",
		},
		region: "configtest",
	}, {
		config: attrs{
			"region": "configtest",
		},
		change: attrs{
			"region": "lcy01",
		},
		err: `cannot change region from "configtest" to "lcy01"`,
	}, {
		config: attrs{
			"region": 666,
		},
		err: ".*expected string, got 666",
	}, {
		config: attrs{
			"username": 666,
		},
		err: ".*expected string, got 666",
	}, {
		config: attrs{
			"password": 666,
		},
		err: ".*expected string, got 666",
	}, {
		config: attrs{
			"tenant-id": 666,
		},
		err: ".*expected string, got 666",
	}, {
		config: attrs{
			"identity-url": 666,
		},
		err: ".*expected string, got 666",
	}, {
		config: attrs{
			"container": 666,
		},
		err: ".*expected string, got 666",
	}, {
		change: attrs{
			"container": "new-x",
		},
		err: `cannot change container from "x" to "new-x"`,
	}, {
		config: attrs{
			"username":     "jujuer",
			"password":     "open sesame",
			"tenant-id":    "juju tenant",
			"identity-url": "some url",
		},
		username:    "jujuer",
		password:    "open sesame",
		tenantId:    "juju tenant",
		identityURL: "some url",
	}, {
		config: attrs{
			"username": "jujuer",
		},
		err: ".*environment has no username, password, tenant-id, or identity-url",
	}, {
		config: attrs{
			"password": "open sesame",
		},
		err: ".*environment has no username, password, tenant-id, or identity-url",
	}, {
		config: attrs{
			"tenant-id": "juju tenant",
		},
		err: ".*environment has no username, password, tenant-id, or identity-url",
	}, {
		config: attrs{
			"identity-url": "some url",
		},
		err: ".*environment has no username, password, tenant-id, or identity-url",
	}, {
		config: attrs{
			"admin-secret": "Futumpsh",
		},
	}, {
		config:       attrs{},
		firewallMode: config.FwInstance,
	}, {
		config: attrs{
			"firewall-mode": "",
		},
		firewallMode: config.FwInstance,
	}, {
		config: attrs{
			"firewall-mode": "instance",
		},
		firewallMode: config.FwInstance,
	}, {
		config: attrs{
			"firewall-mode": "global",
		},
		firewallMode: config.FwGlobal,
	},
}

func (s *ConfigSuite) TestConfig(c *C) {
	for i, t := range configTests {
		c.Logf("test %d: %v", i, t.config)
		t.check(c)
	}
}
