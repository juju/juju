// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package dummy_test

import (
	stdtesting "testing"
	"time"

	gitjujutesting "github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/environs"
	"github.com/juju/juju/environs/bootstrap"
	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/environs/jujutest"
	envtesting "github.com/juju/juju/environs/testing"
	"github.com/juju/juju/instance"
	jujutesting "github.com/juju/juju/juju/testing"
	"github.com/juju/juju/network"
	"github.com/juju/juju/provider/dummy"
	"github.com/juju/juju/testing"
)

func TestPackage(t *stdtesting.T) {
	testing.MgoTestPackage(t)
}

func init() {
	gc.Suite(&liveSuite{
		LiveTests: jujutest.LiveTests{
			TestConfig:     dummy.SampleConfig(),
			CanOpenState:   true,
			HasProvisioner: false,
		},
	})
	gc.Suite(&suite{
		Tests: jujutest.Tests{
			TestConfig: dummy.SampleConfig(),
		},
	})
}

type liveSuite struct {
	testing.BaseSuite
	gitjujutesting.MgoSuite
	jujutest.LiveTests
}

func (s *liveSuite) SetUpSuite(c *gc.C) {
	s.BaseSuite.SetUpSuite(c)
	s.MgoSuite.SetUpSuite(c)
	s.LiveTests.SetUpSuite(c)
}

func (s *liveSuite) TearDownSuite(c *gc.C) {
	s.LiveTests.TearDownSuite(c)
	s.MgoSuite.TearDownSuite(c)
	s.BaseSuite.TearDownSuite(c)
}

func (s *liveSuite) SetUpTest(c *gc.C) {
	s.BaseSuite.SetUpTest(c)
	s.MgoSuite.SetUpTest(c)
	s.LiveTests.SetUpTest(c)
}

func (s *liveSuite) TearDownTest(c *gc.C) {
	s.Destroy(c)
	s.LiveTests.TearDownTest(c)
	s.MgoSuite.TearDownTest(c)
	s.BaseSuite.TearDownTest(c)
}

type suite struct {
	testing.BaseSuite
	gitjujutesting.MgoSuite
	jujutest.Tests
}

func (s *suite) SetUpSuite(c *gc.C) {
	s.BaseSuite.SetUpSuite(c)
	s.MgoSuite.SetUpSuite(c)
}

func (s *suite) TearDownSuite(c *gc.C) {
	s.MgoSuite.TearDownSuite(c)
	s.BaseSuite.TearDownSuite(c)
}

func (s *suite) SetUpTest(c *gc.C) {
	s.BaseSuite.SetUpTest(c)
	s.MgoSuite.SetUpTest(c)
	s.Tests.SetUpTest(c)
}

func (s *suite) TearDownTest(c *gc.C) {
	s.Tests.TearDownTest(c)
	s.MgoSuite.TearDownTest(c)
	dummy.Reset()
	s.BaseSuite.TearDownTest(c)
}

func (s *suite) bootstrapTestEnviron(c *gc.C, preferIPv6 bool) environs.NetworkingEnviron {
	s.TestConfig["prefer-ipv6"] = preferIPv6
	cfg, err := config.New(config.NoDefaults, s.TestConfig)
	c.Assert(err, jc.ErrorIsNil)
	env, err := environs.Prepare(cfg, envtesting.BootstrapContext(c), s.ConfigStore)
	c.Assert(err, gc.IsNil, gc.Commentf("preparing environ %#v", s.TestConfig))
	c.Assert(env, gc.NotNil)
	netenv, supported := environs.SupportsNetworking(env)
	c.Assert(supported, jc.IsTrue)

	err = bootstrap.EnsureNotBootstrapped(netenv)
	c.Assert(err, jc.ErrorIsNil)
	err = bootstrap.Bootstrap(envtesting.BootstrapContext(c), netenv, bootstrap.BootstrapParams{})
	c.Assert(err, jc.ErrorIsNil)
	return netenv
}

func (s *suite) TestAvailabilityZone(c *gc.C) {
	e := s.bootstrapTestEnviron(c, true)
	defer func() {
		err := e.Destroy()
		c.Assert(err, jc.ErrorIsNil)
	}()

	inst, hwc := jujutesting.AssertStartInstance(c, e, "0")
	c.Assert(inst, gc.NotNil)
	c.Check(hwc.AvailabilityZone, gc.IsNil)
}

func (s *suite) TestAllocateAddress(c *gc.C) {
	e := s.bootstrapTestEnviron(c, false)
	defer func() {
		err := e.Destroy()
		c.Assert(err, jc.ErrorIsNil)
	}()

	inst, _ := jujutesting.AssertStartInstance(c, e, "0")
	c.Assert(inst, gc.NotNil)
	subnetId := network.Id("net1")

	opc := make(chan dummy.Operation, 200)
	dummy.Listen(opc)

	newAddress := network.NewAddress("0.1.2.1", network.ScopeCloudLocal)
	err := e.AllocateAddress(inst.Id(), subnetId, newAddress)
	c.Assert(err, jc.ErrorIsNil)

	assertAllocateAddress(c, e, opc, inst.Id(), subnetId, newAddress)

	newAddress = network.NewAddress("0.1.2.2", network.ScopeCloudLocal)
	err = e.AllocateAddress(inst.Id(), subnetId, newAddress)
	c.Assert(err, jc.ErrorIsNil)
	assertAllocateAddress(c, e, opc, inst.Id(), subnetId, newAddress)
}

func (s *suite) TestReleaseAddress(c *gc.C) {
	e := s.bootstrapTestEnviron(c, false)
	defer func() {
		err := e.Destroy()
		c.Assert(err, jc.ErrorIsNil)
	}()

	inst, _ := jujutesting.AssertStartInstance(c, e, "0")
	c.Assert(inst, gc.NotNil)
	subnetId := network.Id("net1")

	opc := make(chan dummy.Operation, 200)
	dummy.Listen(opc)

	address := network.NewAddress("0.1.2.1", network.ScopeCloudLocal)
	err := e.ReleaseAddress(inst.Id(), subnetId, address)
	c.Assert(err, jc.ErrorIsNil)

	assertReleaseAddress(c, e, opc, inst.Id(), subnetId, address)

	address = network.NewAddress("0.1.2.2", network.ScopeCloudLocal)
	err = e.ReleaseAddress(inst.Id(), subnetId, address)
	c.Assert(err, jc.ErrorIsNil)
	assertReleaseAddress(c, e, opc, inst.Id(), subnetId, address)
}

func (s *suite) TestNetworkInterfaces(c *gc.C) {
	e := s.bootstrapTestEnviron(c, false)
	defer func() {
		err := e.Destroy()
		c.Assert(err, jc.ErrorIsNil)
	}()

	opc := make(chan dummy.Operation, 200)
	dummy.Listen(opc)

	expectInfo := []network.InterfaceInfo{{
		ProviderId:     "dummy-private",
		NetworkName:    "juju-private",
		CIDR:           "0.1.2.0/24",
		InterfaceName:  "eth0",
		VLANTag:        0,
		MACAddress:     "aa:bb:cc:dd:ee:f0",
		Disabled:       false,
		NoAutoStart:    false,
		ConfigType:     network.ConfigDHCP,
		Address:        network.NewAddress("0.1.2.1", network.ScopeUnknown),
		DNSServers:     network.NewAddresses("ns1.dummy", "ns2.dummy"),
		GatewayAddress: network.NewAddress("0.1.2.1", network.ScopeUnknown),
		ExtraConfig:    nil,
	}, {
		ProviderId:     "dummy-public",
		NetworkName:    "juju-public",
		CIDR:           "0.2.2.0/24",
		InterfaceName:  "eth1",
		VLANTag:        1,
		MACAddress:     "aa:bb:cc:dd:ee:f1",
		Disabled:       true,
		NoAutoStart:    true,
		ConfigType:     network.ConfigDHCP,
		Address:        network.NewAddress("0.2.2.1", network.ScopeUnknown),
		DNSServers:     network.NewAddresses("ns1.dummy", "ns2.dummy"),
		GatewayAddress: network.NewAddress("0.2.2.1", network.ScopeUnknown),
		ExtraConfig:    nil,
	}}
	info, err := e.NetworkInterfaces(instance.Id("i-42"))
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(info, jc.DeepEquals, expectInfo)
	assertInterfaces(c, e, opc, instance.Id("i-42"), expectInfo)
}

func (s *suite) TestSubnets(c *gc.C) {
	e := s.bootstrapTestEnviron(c, false)
	defer func() {
		err := e.Destroy()
		c.Assert(err, jc.ErrorIsNil)
	}()

	opc := make(chan dummy.Operation, 200)
	dummy.Listen(opc)

	expectInfo := []network.SubnetInfo{
		{CIDR: "0.10.0.0/8", ProviderId: "dummy-private"},
		{CIDR: "0.20.0.0/24", ProviderId: "dummy-public"},
	}
	netInfo, err := e.Subnets("", []network.Id{"dummy-private", "dummy-public"})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(netInfo, jc.DeepEquals, expectInfo)
	assertSubnets(c, e, opc, expectInfo)
}

func (s *suite) TestPreferIPv6On(c *gc.C) {
	e := s.bootstrapTestEnviron(c, true)
	defer func() {
		err := e.Destroy()
		c.Assert(err, jc.ErrorIsNil)
	}()

	inst, _ := jujutesting.AssertStartInstance(c, e, "0")
	c.Assert(inst, gc.NotNil)
	addrs, err := inst.Addresses()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(addrs, jc.DeepEquals, network.NewAddresses("only-0.dns", "127.0.0.1", "fc00::1"))
}

func (s *suite) TestPreferIPv6Off(c *gc.C) {
	e := s.bootstrapTestEnviron(c, false)
	defer func() {
		err := e.Destroy()
		c.Assert(err, jc.ErrorIsNil)
	}()

	inst, _ := jujutesting.AssertStartInstance(c, e, "0")
	c.Assert(inst, gc.NotNil)
	addrs, err := inst.Addresses()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(addrs, jc.DeepEquals, network.NewAddresses("only-0.dns", "127.0.0.1"))
}

func assertAllocateAddress(c *gc.C, e environs.Environ, opc chan dummy.Operation, expectInstId instance.Id, expectSubnetId network.Id, expectAddress network.Address) {
	select {
	case op := <-opc:
		addrOp, ok := op.(dummy.OpAllocateAddress)
		if !ok {
			c.Fatalf("unexpected op: %#v", op)
		}
		c.Check(addrOp.SubnetId, gc.Equals, expectSubnetId)
		c.Check(addrOp.InstanceId, gc.Equals, expectInstId)
		c.Check(addrOp.Address, gc.Equals, expectAddress)
		return
	case <-time.After(testing.ShortWait):
		c.Fatalf("time out wating for operation")
	}
}

func assertReleaseAddress(c *gc.C, e environs.Environ, opc chan dummy.Operation, expectInstId instance.Id, expectSubnetId network.Id, expectAddress network.Address) {
	select {
	case op := <-opc:
		addrOp, ok := op.(dummy.OpReleaseAddress)
		if !ok {
			c.Fatalf("unexpected op: %#v", op)
		}
		c.Check(addrOp.SubnetId, gc.Equals, expectSubnetId)
		c.Check(addrOp.InstanceId, gc.Equals, expectInstId)
		c.Check(addrOp.Address, gc.Equals, expectAddress)
		return
	case <-time.After(testing.ShortWait):
		c.Fatalf("time out wating for operation")
	}
}

func assertInterfaces(c *gc.C, e environs.Environ, opc chan dummy.Operation, expectInstId instance.Id, expectInfo []network.InterfaceInfo) {
	select {
	case op := <-opc:
		netOp, ok := op.(dummy.OpNetworkInterfaces)
		if !ok {
			c.Fatalf("unexpected op: %#v", op)
		}
		c.Check(netOp.Env, gc.Equals, e.Config().Name())
		c.Check(netOp.InstanceId, gc.Equals, expectInstId)
		c.Check(netOp.Info, jc.DeepEquals, expectInfo)
		return
	case <-time.After(testing.ShortWait):
		c.Fatalf("time out wating for operation")
	}
}

func assertSubnets(c *gc.C, e environs.Environ, opc chan dummy.Operation, expectInfo []network.SubnetInfo) {
	select {
	case op := <-opc:
		netOp, ok := op.(dummy.OpListNetworks)
		if !ok {
			c.Fatalf("unexpected op: %#v", op)
		}
		c.Check(netOp.Info, jc.DeepEquals, expectInfo)
		return
	case <-time.After(testing.ShortWait):
		c.Fatalf("time out wating for operation")
	}
}
