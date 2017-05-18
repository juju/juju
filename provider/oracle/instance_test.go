// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package oracle_test

import (
	"errors"
	"sync"

	"github.com/juju/go-oracle-cloud/response"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/environs"
	"github.com/juju/juju/environs/config"
	jujunetwork "github.com/juju/juju/network"
	"github.com/juju/juju/provider/oracle"
	oracletesting "github.com/juju/juju/provider/oracle/testing"
	"github.com/juju/juju/testing"
)

type instanceSuite struct {
	env   *oracle.OracleEnviron
	mutex *sync.Mutex
}

func (i *instanceSuite) SetUpTest(c *gc.C) {
	var err error
	i.env, err = oracle.NewOracleEnviron(
		&oracle.EnvironProvider{},
		environs.OpenParams{
			Config: testing.ModelConfig(c),
		},
		oracletesting.DefaultEnvironAPI,
		&advancingClock,
	)
	c.Assert(err, gc.IsNil)
	c.Assert(i.env, gc.NotNil)
	i.mutex = &sync.Mutex{}
}

func (i *instanceSuite) setEnvironAPI(client oracle.EnvironAPI) {
	i.mutex.Lock()
	defer i.mutex.Unlock()
	i.env.SetEnvironAPI(client)
}

var _ = gc.Suite(&instanceSuite{})

func (i instanceSuite) TestNewOracleInstanceEmpty(c *gc.C) {
	instance, err := oracle.NewOracleInstance(response.Instance{}, i.env)
	c.Assert(err, gc.NotNil)
	c.Assert(instance, gc.IsNil)
}

func (i instanceSuite) TestNewOracleInstance(c *gc.C) {
	instance, err := oracle.NewOracleInstance(oracletesting.DefaultFakeInstancer.Instance, i.env)
	c.Assert(err, gc.IsNil)
	c.Assert(instance, gc.NotNil)
}

func (i instanceSuite) TestId(c *gc.C) {
	instance, err := oracle.NewOracleInstance(oracletesting.DefaultFakeInstancer.Instance, i.env)
	c.Assert(err, gc.IsNil)
	c.Assert(instance, gc.NotNil)

	id := instance.Id()
	ok := (len(id) > 0)
	c.Assert(ok, gc.Equals, true)
}

func (i instanceSuite) TestStatus(c *gc.C) {
	instance, err := oracle.NewOracleInstance(oracletesting.DefaultFakeInstancer.Instance, i.env)
	c.Assert(err, gc.IsNil)
	c.Assert(instance, gc.NotNil)

	status := instance.Status()
	ok := (len(status.Status) > 0)
	c.Assert(ok, gc.Equals, true)
	ok = (len(status.Message) > 0)
	c.Assert(ok, gc.Equals, false)
}

func (i instanceSuite) TestStorageAttachments(c *gc.C) {
	instance, err := oracle.NewOracleInstance(oracletesting.DefaultFakeInstancer.Instance, i.env)
	c.Assert(err, gc.IsNil)
	c.Assert(instance, gc.NotNil)

	attachs := instance.StorageAttachments()
	c.Assert(attachs, gc.NotNil)
}

func (i instanceSuite) TestAddresses(c *gc.C) {
	instance, err := oracle.NewOracleInstance(oracletesting.DefaultFakeInstancer.Instance, i.env)
	c.Assert(err, gc.IsNil)
	c.Assert(instance, gc.NotNil)

	addrs, err := instance.Addresses()
	c.Assert(err, gc.IsNil)
	c.Assert(addrs, gc.NotNil)
}

func (i instanceSuite) TestAddressesWithErrors(c *gc.C) {
	fakeEnv := &oracletesting.FakeEnvironAPI{
		FakeIpAssociation: oracletesting.FakeIpAssociation{
			AllErr: errors.New("FakeEnvironAPI"),
		},
	}
	i.setEnvironAPI(fakeEnv)

	instance, err := oracle.NewOracleInstance(oracletesting.DefaultFakeInstancer.Instance, i.env)
	c.Assert(err, gc.IsNil)
	c.Assert(instance, gc.NotNil)

	_, err = instance.Addresses()
	c.Assert(err, gc.NotNil)
}

func (i instanceSuite) TestOpenPorts(c *gc.C) {
	fakeConfig := map[string]interface{}{
		"firewall-mode": config.FwInstance,
	}
	config, err := i.env.Config().Apply(fakeConfig)
	c.Assert(err, gc.IsNil)

	err = i.env.SetConfig(config)
	c.Assert(err, gc.IsNil)

	instance, err := oracle.NewOracleInstance(oracletesting.DefaultFakeInstancer.Instance, i.env)
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
	fakeConfig := map[string]interface{}{
		"firewall-mode": config.FwInstance,
	}
	config, err := i.env.Config().Apply(fakeConfig)
	c.Assert(err, gc.IsNil)

	err = i.env.SetConfig(config)
	c.Assert(err, gc.IsNil)

	instance, err := oracle.NewOracleInstance(oracletesting.DefaultFakeInstancer.Instance, i.env)
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
	fakeConfig := map[string]interface{}{
		"firewall-mode": config.FwInstance,
	}

	config, err := i.env.Config().Apply(fakeConfig)
	c.Assert(err, gc.IsNil)

	err = i.env.SetConfig(config)
	c.Assert(err, gc.IsNil)

	instance, err := oracle.NewOracleInstance(oracletesting.DefaultFakeInstancer.Instance, i.env)
	c.Assert(err, gc.IsNil)
	c.Assert(instance, gc.NotNil)

	rules, err := instance.IngressRules("0")
	c.Assert(err, gc.IsNil)
	c.Assert(rules, gc.NotNil)
}
