// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package dummy_test

import (
	"net"
	"strings"
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

func (s *suite) TestSupportsAddressAllocation(c *gc.C) {
	e := s.bootstrapTestEnviron(c, false)
	defer func() {
		err := e.Destroy()
		c.Assert(err, jc.ErrorIsNil)
	}()

	// By default, it's supported.
	supported, err := e.SupportsAddressAllocation("any-id")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(supported, jc.IsTrue)

	// Any subnet id with prefix "noalloc-" simulates address
	// allocation is not supported.
	supported, err = e.SupportsAddressAllocation("noalloc-foo")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(supported, jc.IsFalse)

	// Test we can induce an error for SupportsAddressAllocation.
	s.breakMethods(c, e, "SupportsAddressAllocation")
	supported, err = e.SupportsAddressAllocation("any-id")
	c.Assert(err, gc.ErrorMatches, `dummy\.SupportsAddressAllocation is broken`)
	c.Assert(supported, jc.IsFalse)
}

func (s *suite) breakMethods(c *gc.C, e environs.NetworkingEnviron, names ...string) {
	cfg := e.Config()
	brokenCfg, err := cfg.Apply(map[string]interface{}{
		"broken": strings.Join(names, " "),
	})
	c.Assert(err, jc.ErrorIsNil)
	err = e.SetConfig(brokenCfg)
	c.Assert(err, jc.ErrorIsNil)
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

	// Test allocating a couple of addresses.
	newAddress := network.NewScopedAddress("0.1.2.1", network.ScopeCloudLocal)
	err := e.AllocateAddress(inst.Id(), subnetId, newAddress)
	c.Assert(err, jc.ErrorIsNil)
	assertAllocateAddress(c, e, opc, inst.Id(), subnetId, newAddress)

	newAddress = network.NewScopedAddress("0.1.2.2", network.ScopeCloudLocal)
	err = e.AllocateAddress(inst.Id(), subnetId, newAddress)
	c.Assert(err, jc.ErrorIsNil)
	assertAllocateAddress(c, e, opc, inst.Id(), subnetId, newAddress)

	// Test we can induce errors.
	s.breakMethods(c, e, "AllocateAddress")
	newAddress = network.NewScopedAddress("0.1.2.3", network.ScopeCloudLocal)
	err = e.AllocateAddress(inst.Id(), subnetId, newAddress)
	c.Assert(err, gc.ErrorMatches, `dummy\.AllocateAddress is broken`)
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

	// Release a couple of addresses.
	address := network.NewScopedAddress("0.1.2.1", network.ScopeCloudLocal)
	err := e.ReleaseAddress(inst.Id(), subnetId, address)
	c.Assert(err, jc.ErrorIsNil)
	assertReleaseAddress(c, e, opc, inst.Id(), subnetId, address)

	address = network.NewScopedAddress("0.1.2.2", network.ScopeCloudLocal)
	err = e.ReleaseAddress(inst.Id(), subnetId, address)
	c.Assert(err, jc.ErrorIsNil)
	assertReleaseAddress(c, e, opc, inst.Id(), subnetId, address)

	// Test we can induce errors.
	s.breakMethods(c, e, "ReleaseAddress")
	address = network.NewScopedAddress("0.1.2.3", network.ScopeCloudLocal)
	err = e.ReleaseAddress(inst.Id(), subnetId, address)
	c.Assert(err, gc.ErrorMatches, `dummy\.ReleaseAddress is broken`)
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
		ProviderId:       "dummy-eth0",
		ProviderSubnetId: "dummy-private",
		NetworkName:      "juju-private",
		CIDR:             "0.10.0.0/24",
		DeviceIndex:      0,
		InterfaceName:    "eth0",
		VLANTag:          0,
		MACAddress:       "aa:bb:cc:dd:ee:f0",
		Disabled:         false,
		NoAutoStart:      false,
		ConfigType:       network.ConfigDHCP,
		Address:          network.NewAddress("0.10.0.2"),
		DNSServers:       network.NewAddresses("ns1.dummy", "ns2.dummy"),
		GatewayAddress:   network.NewAddress("0.10.0.1"),
		ExtraConfig:      nil,
	}, {
		ProviderId:       "dummy-eth1",
		ProviderSubnetId: "dummy-public",
		NetworkName:      "juju-public",
		CIDR:             "0.20.0.0/24",
		DeviceIndex:      1,
		InterfaceName:    "eth1",
		VLANTag:          1,
		MACAddress:       "aa:bb:cc:dd:ee:f1",
		Disabled:         true,
		NoAutoStart:      true,
		ConfigType:       network.ConfigDHCP,
		Address:          network.NewAddress("0.20.0.2"),
		DNSServers:       network.NewAddresses("ns1.dummy", "ns2.dummy"),
		GatewayAddress:   network.NewAddress("0.20.0.1"),
		ExtraConfig:      nil,
	}}
	info, err := e.NetworkInterfaces("i-42")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(info, jc.DeepEquals, expectInfo)
	assertInterfaces(c, e, opc, "i-42", expectInfo)

	// Test that with instance id prefix "i-no-nics-" no results are
	// returned.
	info, err = e.NetworkInterfaces("i-no-nics-here")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(info, gc.HasLen, 0)
	assertInterfaces(c, e, opc, "i-no-nics-here", expectInfo[:0])

	// Test we can induce errors.
	s.breakMethods(c, e, "NetworkInterfaces")
	info, err = e.NetworkInterfaces("i-any")
	c.Assert(err, gc.ErrorMatches, `dummy\.NetworkInterfaces is broken`)
	c.Assert(info, gc.HasLen, 0)
}

func (s *suite) TestSubnets(c *gc.C) {
	e := s.bootstrapTestEnviron(c, false)
	defer func() {
		err := e.Destroy()
		c.Assert(err, jc.ErrorIsNil)
	}()

	opc := make(chan dummy.Operation, 200)
	dummy.Listen(opc)

	expectInfo := []network.SubnetInfo{{
		CIDR:              "0.10.0.0/24",
		ProviderId:        "dummy-private",
		AllocatableIPLow:  net.ParseIP("0.10.0.0"),
		AllocatableIPHigh: net.ParseIP("0.10.0.255"),
	}, {
		CIDR:              "0.20.0.0/24",
		ProviderId:        "dummy-public",
		AllocatableIPLow:  net.ParseIP("0.20.0.0"),
		AllocatableIPHigh: net.ParseIP("0.20.0.255"),
	}}
	// Prepare a version of the above with no allocatable range to
	// test the magic "i-no-alloc-" prefix below.
	noallocInfo := make([]network.SubnetInfo, len(expectInfo))
	for i, exp := range expectInfo {
		pid := string(exp.ProviderId)
		pid = strings.TrimPrefix(pid, "dummy-")
		noallocInfo[i].ProviderId = network.Id("noalloc-" + pid)
		noallocInfo[i].AllocatableIPLow = nil
		noallocInfo[i].AllocatableIPHigh = nil
		noallocInfo[i].CIDR = exp.CIDR
	}

	ids := []network.Id{"dummy-private", "dummy-public", "foo-bar"}
	netInfo, err := e.Subnets("i-foo", ids)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(netInfo, jc.DeepEquals, expectInfo)
	assertSubnets(c, e, opc, "i-foo", ids, expectInfo)

	// Test filtering by id(s).
	netInfo, err = e.Subnets("i-foo", nil)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(netInfo, jc.DeepEquals, expectInfo)
	assertSubnets(c, e, opc, "i-foo", nil, expectInfo)
	netInfo, err = e.Subnets("i-foo", ids[0:1])
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(netInfo, jc.DeepEquals, expectInfo[0:1])
	assertSubnets(c, e, opc, "i-foo", ids[0:1], expectInfo[0:1])
	netInfo, err = e.Subnets("i-foo", ids[1:])
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(netInfo, jc.DeepEquals, expectInfo[1:])
	assertSubnets(c, e, opc, "i-foo", ids[1:], expectInfo[1:])

	// Test that using an instance id with prefix "i-no-subnets-"
	// returns no results, regardless whether ids are given or not.
	netInfo, err = e.Subnets("i-no-subnets-foo", nil)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(netInfo, gc.HasLen, 0)
	assertSubnets(c, e, opc, "i-no-subnets-foo", nil, expectInfo[:0])

	netInfo, err = e.Subnets("i-no-subnets-foo", ids)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(netInfo, gc.HasLen, 0)
	assertSubnets(c, e, opc, "i-no-subnets-foo", ids, expectInfo[:0])

	// Test the behavior with "i-no-alloc-" instance id prefix.
	// When # is "all", all returned subnets have no allocatable range
	// set and have provider ids with "noalloc-" prefix.
	netInfo, err = e.Subnets("i-no-alloc-all", nil)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(netInfo, jc.DeepEquals, noallocInfo)
	assertSubnets(c, e, opc, "i-no-alloc-all", nil, noallocInfo)

	// When # is an integer, the #-th subnet in result has no
	// allocatable range set and a provider id prefix "noalloc-".
	netInfo, err = e.Subnets("i-no-alloc-0", nil)
	c.Assert(err, jc.ErrorIsNil)
	expectResult := []network.SubnetInfo{noallocInfo[0], expectInfo[1]}
	c.Assert(netInfo, jc.DeepEquals, expectResult)
	assertSubnets(c, e, opc, "i-no-alloc-0", nil, expectResult)

	netInfo, err = e.Subnets("i-no-alloc-1", nil)
	c.Assert(err, jc.ErrorIsNil)
	expectResult = []network.SubnetInfo{expectInfo[0], noallocInfo[1]}
	c.Assert(netInfo, jc.DeepEquals, expectResult)
	assertSubnets(c, e, opc, "i-no-alloc-1", nil, expectResult)

	// For the last case above, also test the error returned when # is
	// not integer or it's out of range of the results (including when
	// filtering by ids is applied).
	netInfo, err = e.Subnets("i-no-alloc-foo", nil)
	c.Assert(err, gc.ErrorMatches, `invalid index "foo"; expected int`)
	c.Assert(netInfo, gc.HasLen, 0)

	netInfo, err = e.Subnets("i-no-alloc-1", ids[:1])
	c.Assert(err, gc.ErrorMatches, `index 1 out of range; expected 0..0`)
	c.Assert(netInfo, gc.HasLen, 0)

	netInfo, err = e.Subnets("i-no-alloc-2", ids)
	c.Assert(err, gc.ErrorMatches, `index 2 out of range; expected 0..1`)
	c.Assert(netInfo, gc.HasLen, 0)

	// Test we can induce errors.
	s.breakMethods(c, e, "Subnets")
	netInfo, err = e.Subnets("i-any", nil)
	c.Assert(err, gc.ErrorMatches, `dummy\.Subnets is broken`)
	c.Assert(netInfo, gc.HasLen, 0)
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

func assertSubnets(
	c *gc.C,
	e environs.Environ,
	opc chan dummy.Operation,
	instId instance.Id,
	subnetIds []network.Id,
	expectInfo []network.SubnetInfo,
) {
	select {
	case op := <-opc:
		netOp, ok := op.(dummy.OpSubnets)
		if !ok {
			c.Fatalf("unexpected op: %#v", op)
		}
		c.Check(netOp.InstanceId, gc.Equals, instId)
		c.Check(netOp.SubnetIds, jc.DeepEquals, subnetIds)
		c.Check(netOp.Info, jc.DeepEquals, expectInfo)
		return
	case <-time.After(testing.ShortWait):
		c.Fatalf("time out wating for operation")
	}
}
