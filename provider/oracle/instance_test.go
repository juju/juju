// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package oracle_test

import (
	"errors"
	"sync"

	"github.com/juju/go-oracle-cloud/response"
	gitjujutesting "github.com/juju/testing"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/core/instance"
	"github.com/juju/juju/core/network"
	"github.com/juju/juju/core/network/firewall"
	"github.com/juju/juju/environs"
	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/environs/context"
	"github.com/juju/juju/provider/oracle"
	oracletesting "github.com/juju/juju/provider/oracle/testing"
	"github.com/juju/juju/testing"
)

type instanceSuite struct {
	gitjujutesting.IsolationSuite
	env   *oracle.OracleEnviron
	mutex *sync.Mutex

	callCtx context.ProviderCallContext
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
	i.callCtx = context.NewCloudCallContext()
}

func (i *instanceSuite) setEnvironAPI(client oracle.EnvironAPI) {
	i.mutex.Lock()
	defer i.mutex.Unlock()
	oracle.SetEnvironAPI(i.env, client)
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
	inst, err := oracle.NewOracleInstance(oracletesting.DefaultFakeInstancer.Instance, i.env)
	c.Assert(err, gc.IsNil)
	c.Assert(inst, gc.NotNil)
	id := inst.Id()
	c.Assert(id, gc.Equals, instance.Id("0"))
}

func (i instanceSuite) TestStatus(c *gc.C) {
	instance, err := oracle.NewOracleInstance(oracletesting.DefaultFakeInstancer.Instance, i.env)
	c.Assert(err, gc.IsNil)
	c.Assert(instance, gc.NotNil)

	status := instance.Status(i.callCtx)
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

	addrs, err := instance.Addresses(i.callCtx)
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

	_, err = instance.Addresses(i.callCtx)
	c.Assert(err, gc.ErrorMatches, "FakeEnvironAPI")
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

	err = instance.OpenPorts(i.callCtx, "0", []firewall.IngressRule{
		firewall.NewIngressRule(network.MustParsePortRange("80/tcp")),
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

	err = instance.ClosePorts(i.callCtx, "0", []firewall.IngressRule{
		firewall.NewIngressRule(network.MustParsePortRange("80/tcp")),
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

	rules, err := instance.IngressRules(i.callCtx, "0")
	c.Assert(err, gc.IsNil)
	c.Assert(rules, gc.NotNil)
}
