// Copyright 2011, 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package ec2

// TODO: Clean this up so it matches environs/openstack/config_test.go.

import (
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"

	jc "github.com/juju/testing/checkers"
	"github.com/juju/utils"
	"gopkg.in/amz.v2/aws"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/environs"
	"github.com/juju/juju/environs/config"
	envtesting "github.com/juju/juju/environs/testing"
	"github.com/juju/juju/testing"
)

// Use local suite since this file lives in the ec2 package
// for testing internals.
type ConfigSuite struct {
	testing.BaseSuite
	savedHome, savedAccessKey, savedSecretKey string
}

var _ = gc.Suite(&ConfigSuite{})

var configTestRegion = aws.Region{
	Name:        "configtest",
	EC2Endpoint: "testregion.nowhere:1234",
}

var testAuth = aws.Auth{"gopher", "long teeth"}

// configTest specifies a config parsing test, checking that env when
// parsed as the ec2 section of a config file matches baseConfigResult
// when mutated by the mutate function, or that the parse matches the
// given error.
type configTest struct {
	config        map[string]interface{}
	change        map[string]interface{}
	expect        map[string]interface{}
	region        string
	cbucket       string
	pbucket       string
	pbucketRegion string
	accessKey     string
	secretKey     string
	firewallMode  string
	err           string
}

type attrs map[string]interface{}

func (t configTest) check(c *gc.C) {
	attrs := testing.FakeConfig().Merge(testing.Attrs{
		"type":           "ec2",
		"control-bucket": "x",
	}).Merge(t.config)
	cfg, err := config.New(config.NoDefaults, attrs)
	c.Assert(err, jc.ErrorIsNil)
	e, err := environs.New(cfg)
	if t.change != nil {
		c.Assert(err, jc.ErrorIsNil)

		// Testing a change in configuration.
		var old, changed, valid *config.Config
		ec2env := e.(*environ)
		old = ec2env.ecfg().Config
		changed, err = old.Apply(t.change)
		c.Assert(err, jc.ErrorIsNil)

		// Keep err for validation below.
		valid, err = providerInstance.Validate(changed, old)
		if err == nil {
			err = ec2env.SetConfig(valid)
		}
	}
	if t.err != "" {
		c.Check(err, gc.ErrorMatches, t.err)
		return
	}
	c.Assert(err, jc.ErrorIsNil)

	ecfg := e.(*environ).ecfg()
	c.Assert(ecfg.Name(), gc.Equals, "testenv")
	c.Assert(ecfg.controlBucket(), gc.Equals, "x")
	if t.region != "" {
		c.Assert(ecfg.region(), gc.Equals, t.region)
	}
	if t.accessKey != "" {
		c.Assert(ecfg.accessKey(), gc.Equals, t.accessKey)
		c.Assert(ecfg.secretKey(), gc.Equals, t.secretKey)
		expected := map[string]string{
			"access-key": t.accessKey,
			"secret-key": t.secretKey,
		}
		c.Assert(err, jc.ErrorIsNil)
		actual, err := e.Provider().SecretAttrs(ecfg.Config)
		c.Assert(err, jc.ErrorIsNil)
		c.Assert(expected, gc.DeepEquals, actual)
	} else {
		c.Assert(ecfg.accessKey(), gc.DeepEquals, testAuth.AccessKey)
		c.Assert(ecfg.secretKey(), gc.DeepEquals, testAuth.SecretKey)
	}
	if t.firewallMode != "" {
		c.Assert(ecfg.FirewallMode(), gc.Equals, t.firewallMode)
	}
	for name, expect := range t.expect {
		actual, found := ecfg.UnknownAttrs()[name]
		c.Check(found, jc.IsTrue)
		c.Check(actual, gc.Equals, expect)
	}

	// check storage bucket is configured correctly
	env := e.(*environ)
	c.Assert(env.Storage().(*ec2storage).bucket.Region.Name, gc.Equals, ecfg.region())
}

var configTests = []configTest{
	{
		config: attrs{},
	}, {
		// check that region defaults to us-east-1
		config: attrs{},
		region: "us-east-1",
	}, {
		config: attrs{
			"region": "eu-west-1",
		},
		region: "eu-west-1",
	}, {
		config: attrs{
			"region": "unknown",
		},
		err: ".*invalid region name.*",
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
			"region": "us-east-1",
		},
		err: `.*cannot change region from "configtest" to "us-east-1"`,
	}, {
		config: attrs{
			"region": 666,
		},
		err: `.*expected string, got int\(666\)`,
	}, {
		config: attrs{
			"access-key": 666,
		},
		err: `.*expected string, got int\(666\)`,
	}, {
		config: attrs{
			"secret-key": 666,
		},
		err: `.*expected string, got int\(666\)`,
	}, {
		config: attrs{
			"control-bucket": 666,
		},
		err: `.*expected string, got int\(666\)`,
	}, {
		change: attrs{
			"control-bucket": "new-x",
		},
		err: `.*cannot change control-bucket from "x" to "new-x"`,
	}, {
		config: attrs{
			"access-key": "jujuer",
			"secret-key": "open sesame",
		},
		accessKey: "jujuer",
		secretKey: "open sesame",
	}, {
		config: attrs{
			"access-key": "jujuer",
		},
		err: ".*environment has no access-key or secret-key",
	}, {
		config: attrs{
			"secret-key": "badness",
		},
		err: ".*environment has no access-key or secret-key",
	}, {
		config: attrs{
			"admin-secret": "Futumpsh",
		},
	}, {
		config:       attrs{},
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
	}, {
		config: attrs{
			"firewall-mode": "none",
		},
		firewallMode: config.FwNone,
	}, {
		config: attrs{
			"ssl-hostname-verification": false,
		},
		err: ".*disabling ssh-hostname-verification is not supported",
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

func indent(s string, with string) string {
	var r string
	lines := strings.Split(s, "\n")
	for _, l := range lines {
		r += with + l + "\n"
	}
	return r
}

func (s *ConfigSuite) SetUpTest(c *gc.C) {
	s.BaseSuite.SetUpTest(c)
	s.savedHome = utils.Home()
	s.savedAccessKey = os.Getenv("AWS_ACCESS_KEY_ID")
	s.savedSecretKey = os.Getenv("AWS_SECRET_ACCESS_KEY")

	home := c.MkDir()
	sshDir := filepath.Join(home, ".ssh")
	err := os.Mkdir(sshDir, 0777)
	c.Assert(err, jc.ErrorIsNil)
	err = ioutil.WriteFile(filepath.Join(sshDir, "id_rsa.pub"), []byte("sshkey\n"), 0666)
	c.Assert(err, jc.ErrorIsNil)

	utils.SetHome(home)
	os.Setenv("AWS_ACCESS_KEY_ID", testAuth.AccessKey)
	os.Setenv("AWS_SECRET_ACCESS_KEY", testAuth.SecretKey)
	aws.Regions["configtest"] = configTestRegion
}

func (s *ConfigSuite) TearDownTest(c *gc.C) {
	utils.SetHome(s.savedHome)
	os.Setenv("AWS_ACCESS_KEY_ID", s.savedAccessKey)
	os.Setenv("AWS_SECRET_ACCESS_KEY", s.savedSecretKey)
	delete(aws.Regions, "configtest")
	s.BaseSuite.TearDownTest(c)
}

func (s *ConfigSuite) TestConfig(c *gc.C) {
	for i, t := range configTests {
		c.Logf("test %d: %v", i, t.config)
		t.check(c)
	}
}

func (s *ConfigSuite) TestMissingAuth(c *gc.C) {
	os.Setenv("AWS_ACCESS_KEY_ID", "")
	os.Setenv("AWS_SECRET_ACCESS_KEY", "")
	// Since r37 goamz uses these as fallbacks, so unset them too.
	os.Setenv("EC2_ACCESS_KEY", "")
	os.Setenv("EC2_SECRET_KEY", "")
	test := configTests[0]
	test.err = ".*environment has no access-key or secret-key"
	test.check(c)
}

func (s *ConfigSuite) TestPrepareInsertsUniqueControlBucket(c *gc.C) {
	s.PatchValue(&verifyCredentials, func(*environ) error { return nil })
	attrs := testing.FakeConfig().Merge(testing.Attrs{
		"type": "ec2",
	})
	cfg, err := config.New(config.NoDefaults, attrs)
	c.Assert(err, jc.ErrorIsNil)

	ctx := envtesting.BootstrapContext(c)
	env0, err := providerInstance.Prepare(ctx, cfg)
	c.Assert(err, jc.ErrorIsNil)
	bucket0 := env0.(*environ).ecfg().controlBucket()
	c.Assert(bucket0, gc.Matches, "[a-f0-9]{32}")

	env1, err := providerInstance.Prepare(ctx, cfg)
	c.Assert(err, jc.ErrorIsNil)
	bucket1 := env1.(*environ).ecfg().controlBucket()
	c.Assert(bucket1, gc.Matches, "[a-f0-9]{32}")

	c.Assert(bucket1, gc.Not(gc.Equals), bucket0)
}

func (s *ConfigSuite) TestPrepareDoesNotTouchExistingControlBucket(c *gc.C) {
	s.PatchValue(&verifyCredentials, func(*environ) error { return nil })
	attrs := testing.FakeConfig().Merge(testing.Attrs{
		"type":           "ec2",
		"control-bucket": "burblefoo",
	})
	cfg, err := config.New(config.NoDefaults, attrs)
	c.Assert(err, jc.ErrorIsNil)

	env, err := providerInstance.Prepare(envtesting.BootstrapContext(c), cfg)
	c.Assert(err, jc.ErrorIsNil)
	bucket := env.(*environ).ecfg().controlBucket()
	c.Assert(bucket, gc.Equals, "burblefoo")
}
