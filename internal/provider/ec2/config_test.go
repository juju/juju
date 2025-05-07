// Copyright 2011, 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package ec2

// TODO: Clean this up so it matches environs/openstack/config_test.go.

import (
	"context"

	"github.com/juju/tc"

	"github.com/juju/juju/cloud"
	"github.com/juju/juju/environs"
	environscloudspec "github.com/juju/juju/environs/cloudspec"
	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/internal/testhelpers"
	"github.com/juju/juju/internal/testing"
)

// Use local suite since this file lives in the ec2 package
// for testing internals.
type ConfigSuite struct {
	testhelpers.IsolationSuite
}

var _ = tc.Suite(&ConfigSuite{})

// configTest specifies a config parsing test, checking that env when
// parsed as the ec2 section of a config file matches baseConfigResult
// when mutated by the mutate function, or that the parse matches the
// given error.
type configTest struct {
	config             map[string]interface{}
	change             map[string]interface{}
	expect             map[string]interface{}
	vpcID              string
	forceVPCID         bool
	firewallMode       string
	blockStorageSource string
	err                string
}

type attrs map[string]interface{}

func (t configTest) check(c *tc.C) {
	credential := cloud.NewCredential(
		cloud.AccessKeyAuthType,
		map[string]string{
			"access-key": "x",
			"secret-key": "y",
		},
	)
	cloudSpec := environscloudspec.CloudSpec{
		Type:       "ec2",
		Name:       "ec2test",
		Region:     "us-east-1",
		Credential: &credential,
	}
	attrs := testing.FakeConfig().Merge(testing.Attrs{
		"type": "ec2",
	}).Merge(t.config)
	cfg, err := config.New(config.NoDefaults, attrs)
	c.Assert(err, tc.ErrorIsNil)
	e, err := environs.New(context.Background(), environs.OpenParams{
		Cloud:  cloudSpec,
		Config: cfg,
	}, environs.NoopCredentialInvalidator())
	if t.change != nil {
		c.Assert(err, tc.ErrorIsNil)

		// Testing a change in configuration.
		var old, changed, valid *config.Config
		ec2env := e.(*environ)
		old = ec2env.ecfg().Config
		changed, err = old.Apply(t.change)
		c.Assert(err, tc.ErrorIsNil)

		// Keep err for validation below.
		valid, err = providerInstance.Validate(context.Background(), changed, old)
		if err == nil {
			err = ec2env.SetConfig(context.Background(), valid)
		}
	}
	if t.err != "" {
		c.Check(err, tc.ErrorMatches, t.err)
		return
	}
	c.Assert(err, tc.ErrorIsNil)

	ecfg := e.(*environ).ecfg()
	c.Assert(ecfg.Name(), tc.Equals, "testmodel")
	c.Assert(ecfg.vpcID(), tc.Equals, t.vpcID)
	c.Assert(ecfg.forceVPCID(), tc.Equals, t.forceVPCID)

	if t.firewallMode != "" {
		c.Assert(ecfg.FirewallMode(), tc.Equals, t.firewallMode)
	}
	for name, expect := range t.expect {
		actual, found := ecfg.UnknownAttrs()[name]
		c.Check(found, tc.IsTrue)
		c.Check(actual, tc.Equals, expect)
	}
}

var configTests = []configTest{
	{
		config: attrs{},
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

func (s *ConfigSuite) TestConfig(c *tc.C) {
	for i, t := range configTests {
		c.Logf("test %d: %v", i, t.config)
		t.check(c)
	}
}

// TestModelConfigDefaults is asserting the default model config values returned
// from the ec2 provider. If you have broken this test it means you have broken
// business logic in Juju around this provider and this needs to be very
// considered.
func (s *ConfigSuite) TestModelConfigDefaults(c *tc.C) {
	defaults, err := providerInstance.ModelConfigDefaults(context.Background())
	c.Check(err, tc.ErrorIsNil)
	c.Check(defaults[config.StorageDefaultBlockSourceKey], tc.Equals, "ebs")
}

func (*ConfigSuite) TestSchema(c *tc.C) {
	fields := providerInstance.Schema()
	// Check that all the fields defined in environs/config
	// are in the returned schema.
	globalFields, err := config.Schema(nil)
	c.Assert(err, tc.IsNil)
	for name, field := range globalFields {
		c.Check(fields[name], tc.DeepEquals, field)
	}
}
