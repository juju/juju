// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package oracle_test

import (
	"errors"

	"github.com/juju/go-oracle-cloud/response"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/environs"
	"github.com/juju/juju/environs/config"
	jujunetwork "github.com/juju/juju/network"
	"github.com/juju/juju/provider/oracle"
	"github.com/juju/juju/testing"
)

type instanceSuite struct{}

var _ = gc.Suite(&instanceSuite{})

func (i instanceSuite) TestNewOracleInstanceEmpty(c *gc.C) {
	environ, err := oracle.NewOracleEnviron(
		oracle.DefaultProvider,
		environs.OpenParams{
			Config: testing.ModelConfig(c),
		},
		DefaultEnvironAPI,
		&advancingClock,
	)
	c.Assert(err, gc.IsNil)
	c.Assert(environ, gc.NotNil)

	instance, err := oracle.NewOracleInstance(response.Instance{}, environ)
	c.Assert(err, gc.NotNil)
	c.Assert(instance, gc.IsNil)
}

func (i instanceSuite) TestNewOracleInstance(c *gc.C) {
	environ, err := oracle.NewOracleEnviron(
		oracle.DefaultProvider,
		environs.OpenParams{
			Config: testing.ModelConfig(c),
		},
		DefaultEnvironAPI,
		&advancingClock,
	)
	c.Assert(err, gc.IsNil)
	c.Assert(environ, gc.NotNil)

	instance, err := oracle.NewOracleInstance(DefaultFakeInstancer.Instance, environ)
	c.Assert(err, gc.IsNil)
	c.Assert(instance, gc.NotNil)
}

func (i instanceSuite) TestId(c *gc.C) {
	environ, err := oracle.NewOracleEnviron(
		oracle.DefaultProvider,
		environs.OpenParams{
			Config: testing.ModelConfig(c),
		},
		DefaultEnvironAPI,
		&advancingClock,
	)
	c.Assert(err, gc.IsNil)
	c.Assert(environ, gc.NotNil)

	instance, err := oracle.NewOracleInstance(DefaultFakeInstancer.Instance, environ)
	c.Assert(err, gc.IsNil)
	c.Assert(instance, gc.NotNil)

	id := instance.Id()
	ok := (len(id) > 0)
	c.Assert(ok, gc.Equals, true)
}

func (i instanceSuite) TestStatus(c *gc.C) {
	environ, err := oracle.NewOracleEnviron(
		oracle.DefaultProvider,
		environs.OpenParams{
			Config: testing.ModelConfig(c),
		},
		DefaultEnvironAPI,
		&advancingClock,
	)
	c.Assert(err, gc.IsNil)
	c.Assert(environ, gc.NotNil)

	instance, err := oracle.NewOracleInstance(DefaultFakeInstancer.Instance, environ)
	c.Assert(err, gc.IsNil)
	c.Assert(instance, gc.NotNil)

	status := instance.Status()
	ok := (len(status.Status) > 0)
	c.Assert(ok, gc.Equals, true)
	ok = (len(status.Message) > 0)
	c.Assert(ok, gc.Equals, false)
}

func (i instanceSuite) TestStorageAttachments(c *gc.C) {
	environ, err := oracle.NewOracleEnviron(
		oracle.DefaultProvider,
		environs.OpenParams{
			Config: testing.ModelConfig(c),
		},
		DefaultEnvironAPI,
		&advancingClock,
	)
	c.Assert(err, gc.IsNil)
	c.Assert(environ, gc.NotNil)

	instance, err := oracle.NewOracleInstance(DefaultFakeInstancer.Instance, environ)
	c.Assert(err, gc.IsNil)
	c.Assert(instance, gc.NotNil)

	attachs := instance.StorageAttachments()
	c.Assert(attachs, gc.NotNil)
}

func (i instanceSuite) TestAddresses(c *gc.C) {
	environ, err := oracle.NewOracleEnviron(
		oracle.DefaultProvider,
		environs.OpenParams{
			Config: testing.ModelConfig(c),
		},
		DefaultEnvironAPI,
		&advancingClock,
	)
	c.Assert(err, gc.IsNil)
	c.Assert(environ, gc.NotNil)

	instance, err := oracle.NewOracleInstance(DefaultFakeInstancer.Instance, environ)
	c.Assert(err, gc.IsNil)
	c.Assert(instance, gc.NotNil)

	addrs, err := instance.Addresses()
	c.Assert(err, gc.IsNil)
	c.Assert(addrs, gc.NotNil)
}

func (i instanceSuite) TestAddressesWithErrors(c *gc.C) {
	environ, err := oracle.NewOracleEnviron(
		oracle.DefaultProvider,
		environs.OpenParams{
			Config: testing.ModelConfig(c),
		},
		&FakeEnvironAPI{
			FakeIpAssociation: FakeIpAssociation{
				AllErr: errors.New("FakeEnvironAPI"),
			},
		},
		&advancingClock,
	)
	c.Assert(err, gc.IsNil)
	c.Assert(environ, gc.NotNil)

	instance, err := oracle.NewOracleInstance(DefaultFakeInstancer.Instance, environ)
	c.Assert(err, gc.IsNil)
	c.Assert(instance, gc.NotNil)

	_, err = instance.Addresses()
	c.Assert(err, gc.NotNil)
}

func (i instanceSuite) TestOpenPorts(c *gc.C) {
	fakeConfig := testing.CustomModelConfig(c, testing.Attrs{
		"firewall-mode": config.FwInstance,
	})

	environ, err := oracle.NewOracleEnviron(
		oracle.DefaultProvider,
		environs.OpenParams{
			Config: fakeConfig,
		},
		DefaultEnvironAPI,
		&advancingClock,
	)
	c.Assert(err, gc.IsNil)
	c.Assert(environ, gc.NotNil)

	instance, err := oracle.NewOracleInstance(DefaultFakeInstancer.Instance, environ)
	c.Assert(err, gc.IsNil)
	c.Assert(instance, gc.NotNil)

	err = instance.OpenPorts("0", []jujunetwork.IngressRule{
		jujunetwork.IngressRule{
			PortRange: jujunetwork.PortRange{
				FromPort: 0,
				ToPort:   0,
			},
			SourceCIDRs: nil,
		},
	})
	c.Assert(err, gc.IsNil)
}

func (i instanceSuite) TestClosePorts(c *gc.C) {
	fakeConfig := testing.CustomModelConfig(c, testing.Attrs{
		"firewall-mode": config.FwInstance,
	})

	environ, err := oracle.NewOracleEnviron(
		oracle.DefaultProvider,
		environs.OpenParams{
			Config: fakeConfig,
		},
		DefaultEnvironAPI,
		&advancingClock,
	)
	c.Assert(err, gc.IsNil)
	c.Assert(environ, gc.NotNil)

	instance, err := oracle.NewOracleInstance(DefaultFakeInstancer.Instance, environ)
	c.Assert(err, gc.IsNil)
	c.Assert(instance, gc.NotNil)

	err = instance.ClosePorts("0", []jujunetwork.IngressRule{
		jujunetwork.IngressRule{
			PortRange: jujunetwork.PortRange{
				FromPort: 0,
				ToPort:   0,
			},
			SourceCIDRs: nil,
		},
	})
	c.Assert(err, gc.IsNil)
}

func (i instanceSuite) TestIngressRules(c *gc.C) {
	fakeConfig := testing.CustomModelConfig(c, testing.Attrs{
		"firewall-mode": config.FwInstance,
	})

	environ, err := oracle.NewOracleEnviron(
		oracle.DefaultProvider,
		environs.OpenParams{
			Config: fakeConfig,
		},
		DefaultEnvironAPI,
		&advancingClock,
	)
	c.Assert(err, gc.IsNil)
	c.Assert(environ, gc.NotNil)

	instance, err := oracle.NewOracleInstance(DefaultFakeInstancer.Instance, environ)
	c.Assert(err, gc.IsNil)
	c.Assert(instance, gc.NotNil)

	rules, err := instance.IngressRules("0")
	c.Assert(err, gc.IsNil)
	c.Assert(rules, gc.NotNil)
}
