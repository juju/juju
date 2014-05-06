// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package dummy_test

import (
	stdtesting "testing"
	"time"

	gc "launchpad.net/gocheck"

	"launchpad.net/juju-core/environs"
	"launchpad.net/juju-core/environs/bootstrap"
	"launchpad.net/juju-core/environs/config"
	"launchpad.net/juju-core/environs/jujutest"
	"launchpad.net/juju-core/environs/network"
	"launchpad.net/juju-core/instance"
	"launchpad.net/juju-core/provider/dummy"
	"launchpad.net/juju-core/testing"
)

func TestPackage(t *stdtesting.T) {
	testing.MgoTestPackage(t)
}

func init() {
	gc.Suite(&jujutest.LiveTests{
		TestConfig:     dummy.SampleConfig(),
		CanOpenState:   true,
		HasProvisioner: false,
	})
	gc.Suite(&suite{
		Tests: jujutest.Tests{
			TestConfig: dummy.SampleConfig(),
		},
	})
}

type suite struct {
	jujutest.Tests
}

func (s *suite) TearDownTest(c *gc.C) {
	s.Tests.TearDownTest(c)
	dummy.Reset()
}

func (s *suite) TestAllocateAddress(c *gc.C) {
	cfg, err := config.New(config.NoDefaults, t.TestConfig)
	c.Assert(err, gc.IsNil)
	e, err := environs.Prepare(cfg, coretesting.Context(c), t.ConfigStore)
	c.Assert(err, gc.IsNil, gc.Commentf("preparing environ %#v", t.TestConfig))
	c.Assert(e, gc.NotNil)

	envtesting.UploadFakeTools(c, e.Storage())
	err := bootstrap.EnsureNotBootstrapped(e)
	c.Assert(err, gc.IsNil)
	err = bootstrap.Bootstrap(coretesting.Context(c), e, environs.BootstrapParams{})
	c.Assert(err, gc.IsNil)

	inst, _ := testing.AssertStartInstance(c, e, "0")
	c.Assert(inst, gc.NotNil)
	netId := network.Id("net1")

	opc := make(chan dummy.Operation, 200)
	dummy.Listen(opc)

	expectAddress := instance.NewAddress("0.1.2.1", instance.NetworkCloudLocal)
	address, err := e.AllocateAddress(inst.Id(), netId)
	c.Assert(err, gc.IsNil)
	c.Assert(address, gc.DeepEquals, expectAddress)

	assertAllocateAddress(c, e, opc, inst.Id(), netId, expectAddress)

	expectAddress = instance.NewAddress("0.1.2.2", instance.NetworkCloudLocal)
	address, err = e.AllocateAddress(inst.Id(), netId)
	c.Assert(err, gc.IsNil)
	c.Assert(address, gc.DeepEquals, expectAddress)
	assertAllocateAddress(c, e, opc, inst.Id(), netId, expectAddress)
}

func (s *suite) prepareAndBootstrap(c *gc.C) environs.Environ {
	return e
}

func assertAllocateAddress(c *gc.C, e environs.Environ, opc chan dummy.Operation, expectInstId instance.Id, expectNetId network.Id, expectAddress instance.Address) {
	select {
	case op := <-opc:
		addrOp, ok := op.(dummy.OpAllocateAddress)
		if !ok {
			c.Fatalf("unexpected op: %#v", op)
		}
		c.Check(addrOp.NetworkId, gc.Equals, expectNetId)
		c.Check(addrOp.InstanceId, gc.Equals, expectInstId)
		c.Check(addrOp.Address, gc.Equals, expectAddress)
		return
	case <-time.After(coretesting.ShortWait):
		c.Fatalf("time out wating for operation")
	}
}
