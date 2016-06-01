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
	"gopkg.in/amz.v3/aws"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/cloud"
	"github.com/juju/juju/environs"
	"github.com/juju/juju/environs/config"
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

var testAuth = aws.Auth{
	AccessKey: "gopher",
	SecretKey: "long teeth",
}

// configTest specifies a config parsing test, checking that env when
// parsed as the ec2 section of a config file matches baseConfigResult
// when mutated by the mutate function, or that the parse matches the
// given error.
type configTest struct {
	config             map[string]interface{}
	change             map[string]interface{}
	expect             map[string]interface{}
	region             string
	vpcID              string
	forceVPCID         bool
	accessKey          string
	secretKey          string
	firewallMode       string
	blockStorageSource string
	err                string
}

type attrs map[string]interface{}

func (t configTest) check(c *gc.C) {
	attrs := testing.FakeConfig().Merge(testing.Attrs{
		"type": "ec2",
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
	if t.region != "" {
		c.Assert(ecfg.region(), gc.Equals, t.region)
	}

	c.Assert(ecfg.vpcID(), gc.Equals, t.vpcID)
	c.Assert(ecfg.forceVPCID(), gc.Equals, t.forceVPCID)

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
		config:     attrs{},
		vpcID:      "",
		forceVPCID: false,
	}, {
		config: attrs{
			"vpc-id": "invalid",
		},
		err:        `.*vpc-id: "invalid" is not a valid AWS VPC ID`,
		vpcID:      "",
		forceVPCID: false,
	}, {
		config: attrs{
			"vpc-id": vpcIDNone,
		},
		vpcID:      "none",
		forceVPCID: false,
	}, {
		config: attrs{
			"vpc-id": 42,
		},
		err:        `.*expected string, got int\(42\)`,
		vpcID:      "",
		forceVPCID: false,
	}, {
		config: attrs{
			"vpc-id-force": "nonsense",
		},
		err:        `.*expected bool, got string\("nonsense"\)`,
		vpcID:      "",
		forceVPCID: false,
	}, {
		config: attrs{
			"vpc-id":       "vpc-anything",
			"vpc-id-force": 999,
		},
		err:        `.*expected bool, got int\(999\)`,
		vpcID:      "",
		forceVPCID: false,
	}, {
		config: attrs{
			"vpc-id":       "",
			"vpc-id-force": true,
		},
		err:        `.*cannot use vpc-id-force without specifying vpc-id as well`,
		vpcID:      "",
		forceVPCID: true,
	}, {
		config: attrs{
			"vpc-id": "vpc-a1b2c3d4",
		},
		vpcID:      "vpc-a1b2c3d4",
		forceVPCID: false,
	}, {
		config: attrs{
			"vpc-id":       "vpc-some-id",
			"vpc-id-force": true,
		},
		vpcID:      "vpc-some-id",
		forceVPCID: true,
	}, {
		config: attrs{
			"vpc-id":       "vpc-abcd",
			"vpc-id-force": false,
		},
		vpcID:      "vpc-abcd",
		forceVPCID: false,
	}, {
		config: attrs{
			"vpc-id":       "vpc-unchanged",
			"vpc-id-force": true,
		},
		change: attrs{
			"vpc-id":       "vpc-unchanged",
			"vpc-id-force": false,
		},
		err:        `.*cannot change vpc-id-force from true to false`,
		vpcID:      "vpc-unchanged",
		forceVPCID: true,
	}, {
		config: attrs{
			"vpc-id": "",
		},
		change: attrs{
			"vpc-id": "none",
		},
		err:        `.*cannot change vpc-id from "" to "none"`,
		vpcID:      "",
		forceVPCID: false,
	}, {
		config: attrs{
			"vpc-id": "",
		},
		change: attrs{
			"vpc-id": "vpc-changed",
		},
		err:        `.*cannot change vpc-id from "" to "vpc-changed"`,
		vpcID:      "",
		forceVPCID: false,
	}, {
		config: attrs{
			"vpc-id": "vpc-initial",
		},
		change: attrs{
			"vpc-id": "",
		},
		err:        `.*cannot change vpc-id from "vpc-initial" to ""`,
		vpcID:      "vpc-initial",
		forceVPCID: false,
	}, {
		config: attrs{
			"vpc-id": "vpc-old",
		},
		change: attrs{
			"vpc-id": "vpc-new",
		},
		err:        `.*cannot change vpc-id from "vpc-old" to "vpc-new"`,
		vpcID:      "vpc-old",
		forceVPCID: false,
	}, {
		config: attrs{
			"vpc-id":       "vpc-foo",
			"vpc-id-force": true,
		},
		change:     attrs{},
		vpcID:      "vpc-foo",
		forceVPCID: true,
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
			"access-key": "jujuer",
			"secret-key": "open sesame",
		},
		accessKey: "jujuer",
		secretKey: "open sesame",
	}, {
		config: attrs{
			"access-key": "jujuer",
		},
		err: ".*model has no access-key or secret-key",
	}, {
		config: attrs{
			"secret-key": "badness",
		},
		err: ".*model has no access-key or secret-key",
	}, {
		config: attrs{
			"admin-secret": "Futumpsh",
		},
	}, {
		config:       attrs{},
		firewallMode: config.FwInstance,
	}, {
		config:             attrs{},
		blockStorageSource: "ebs",
	}, {
		config: attrs{
			"storage-default-block-source": "ebs-fast",
		},
		blockStorageSource: "ebs-fast",
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

	err = utils.SetHome(home)
	c.Assert(err, jc.ErrorIsNil)
	os.Setenv("AWS_ACCESS_KEY_ID", testAuth.AccessKey)
	os.Setenv("AWS_SECRET_ACCESS_KEY", testAuth.SecretKey)
	aws.Regions["configtest"] = configTestRegion
}

func (s *ConfigSuite) TearDownTest(c *gc.C) {
	err := utils.SetHome(s.savedHome)
	c.Assert(err, jc.ErrorIsNil)
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

	// Since PR #52 amz.v3 uses these AWS_ vars as fallbacks, if set.
	os.Setenv("AWS_ACCESS_KEY", "")
	os.Setenv("AWS_SECRET_KEY", "")

	// Since LP r37 goamz uses also these EC2_ as fallbacks, so unset them too.
	os.Setenv("EC2_ACCESS_KEY", "")
	os.Setenv("EC2_SECRET_KEY", "")
	test := configTests[0]
	test.err = ".*model has no access-key or secret-key"
	test.check(c)
}

func (s *ConfigSuite) TestBootstrapConfigSetsDefaultBlockSource(c *gc.C) {
	s.PatchValue(&verifyCredentials, func(*environ) error { return nil })
	attrs := testing.FakeConfig().Merge(testing.Attrs{
		"type": "ec2",
	})
	cfg, err := config.New(config.NoDefaults, attrs)
	c.Assert(err, jc.ErrorIsNil)

	cfg, err = providerInstance.BootstrapConfig(environs.BootstrapConfigParams{
		Config: cfg,
		Credentials: cloud.NewCredential(
			cloud.AccessKeyAuthType,
			map[string]string{
				"access-key": "x",
				"secret-key": "y",
			},
		),
		CloudRegion: "test",
	})
	c.Assert(err, jc.ErrorIsNil)
	source, ok := cfg.StorageDefaultBlockSource()
	c.Assert(ok, jc.IsTrue)
	c.Assert(source, gc.Equals, "ebs")
}

func (s *ConfigSuite) TestPrepareSetsDefaultBlockSource(c *gc.C) {
	s.PatchValue(&verifyCredentials, func(*environ) error { return nil })
	attrs := testing.FakeConfig().Merge(testing.Attrs{
		"type": "ec2",
	})
	config, err := config.New(config.NoDefaults, attrs)
	c.Assert(err, jc.ErrorIsNil)

	cfg, err := providerInstance.BootstrapConfig(environs.BootstrapConfigParams{
		Config:      config,
		CloudRegion: "test",
		Credentials: cloud.NewCredential(
			cloud.AccessKeyAuthType,
			map[string]string{
				"access-key": "x",
				"secret-key": "y",
			},
		),
	})
	c.Assert(err, jc.ErrorIsNil)

	source, ok := cfg.StorageDefaultBlockSource()
	c.Assert(ok, jc.IsTrue)
	c.Assert(source, gc.Equals, "ebs")
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
