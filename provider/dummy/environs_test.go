// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package dummy_test

import (
	"strings"
	stdtesting "testing"
	"time"

	"github.com/juju/errors"
	gitjujutesting "github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/cloud"
	"github.com/juju/juju/core/instance"
	corenetwork "github.com/juju/juju/core/network"
	"github.com/juju/juju/environs"
	"github.com/juju/juju/environs/bootstrap"
	"github.com/juju/juju/environs/context"
	"github.com/juju/juju/environs/jujutest"
	sstesting "github.com/juju/juju/environs/simplestreams/testing"
	envtesting "github.com/juju/juju/environs/testing"
	"github.com/juju/juju/juju/keys"
	jujutesting "github.com/juju/juju/juju/testing"
	"github.com/juju/juju/network"
	"github.com/juju/juju/provider/dummy"
	"github.com/juju/juju/testing"
	jujuversion "github.com/juju/juju/version"
)

const AdminSecret = "admin-secret"

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
	s.BaseSuite.PatchValue(&keys.JujuPublicKey, sstesting.SignedMetadataPublicKey)
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
	s.BaseSuite.PatchValue(&dummy.LogDir, c.MkDir())
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

	callCtx context.ProviderCallContext
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
	s.PatchValue(&jujuversion.Current, testing.FakeVersionNumber)
	s.MgoSuite.SetUpTest(c)
	s.Tests.SetUpTest(c)
	s.PatchValue(&dummy.LogDir, c.MkDir())
	s.callCtx = context.NewCloudCallContext()
}

func (s *suite) TearDownTest(c *gc.C) {
	s.Tests.TearDownTest(c)
	s.MgoSuite.TearDownTest(c)
	dummy.Reset(c)
	s.BaseSuite.TearDownTest(c)
}

func (s *suite) bootstrapTestEnviron(c *gc.C) environs.NetworkingEnviron {
	e, err := bootstrap.PrepareController(
		false,
		envtesting.BootstrapContext(c),
		s.ControllerStore,
		bootstrap.PrepareParams{
			ControllerConfig: testing.FakeControllerConfig(),
			ModelConfig:      s.TestConfig,
			ControllerName:   s.TestConfig["name"].(string),
			Cloud:            dummy.SampleCloudSpec(),
			AdminSecret:      AdminSecret,
		},
	)
	c.Assert(err, gc.IsNil, gc.Commentf("preparing environ %#v", s.TestConfig))
	c.Assert(e, gc.NotNil)
	env := e.(environs.Environ)
	netenv, supported := environs.SupportsNetworking(env)
	c.Assert(supported, jc.IsTrue)

	err = bootstrap.Bootstrap(envtesting.BootstrapContext(c), netenv,
		context.NewCloudCallContext(), bootstrap.BootstrapParams{
			ControllerConfig: testing.FakeControllerConfig(),
			Cloud: cloud.Cloud{
				Name:      "dummy",
				Type:      "dummy",
				AuthTypes: []cloud.AuthType{cloud.EmptyAuthType},
			},
			AdminSecret:  AdminSecret,
			CAPrivateKey: testing.CAKey,
		})
	c.Assert(err, jc.ErrorIsNil)
	return netenv
}

func (s *suite) TestAvailabilityZone(c *gc.C) {
	e := s.bootstrapTestEnviron(c)
	defer func() {
		err := e.Destroy(s.callCtx)
		c.Assert(err, jc.ErrorIsNil)
	}()

	inst, hwc := jujutesting.AssertStartInstance(c, e, s.callCtx, s.ControllerUUID, "0")
	c.Assert(inst, gc.NotNil)
	c.Check(hwc.AvailabilityZone, gc.NotNil)
}

func (s *suite) TestSupportsSpaces(c *gc.C) {
	e := s.bootstrapTestEnviron(c)
	defer func() {
		err := e.Destroy(s.callCtx)
		c.Assert(err, jc.ErrorIsNil)
	}()

	// Without change spaces are supported.
	ok, err := e.SupportsSpaces(s.callCtx)
	c.Assert(ok, jc.IsTrue)
	c.Assert(err, jc.ErrorIsNil)

	// Now turn it off.
	isEnabled := dummy.SetSupportsSpaces(false)
	c.Assert(isEnabled, jc.IsTrue)
	ok, err = e.SupportsSpaces(s.callCtx)
	c.Assert(ok, jc.IsFalse)
	c.Assert(err, jc.Satisfies, errors.IsNotSupported)

	// And finally turn it on again.
	isEnabled = dummy.SetSupportsSpaces(true)
	c.Assert(isEnabled, jc.IsFalse)
	ok, err = e.SupportsSpaces(s.callCtx)
	c.Assert(ok, jc.IsTrue)
	c.Assert(err, jc.ErrorIsNil)
}

func (s *suite) TestSupportsSpaceDiscovery(c *gc.C) {
	e := s.bootstrapTestEnviron(c)
	defer func() {
		err := e.Destroy(s.callCtx)
		c.Assert(err, jc.ErrorIsNil)
	}()

	// Without change space discovery is not supported.
	ok, err := e.SupportsSpaceDiscovery(s.callCtx)
	c.Assert(ok, jc.IsFalse)
	c.Assert(err, jc.ErrorIsNil)

	// Now turn it on.
	isEnabled := dummy.SetSupportsSpaceDiscovery(true)
	c.Assert(isEnabled, jc.IsFalse)
	ok, err = e.SupportsSpaceDiscovery(s.callCtx)
	c.Assert(ok, jc.IsTrue)
	c.Assert(err, jc.ErrorIsNil)

	// And finally turn it off again.
	isEnabled = dummy.SetSupportsSpaceDiscovery(false)
	c.Assert(isEnabled, jc.IsTrue)
	ok, err = e.SupportsSpaceDiscovery(s.callCtx)
	c.Assert(ok, jc.IsFalse)
	c.Assert(err, jc.ErrorIsNil)
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

func (s *suite) TestNetworkInterfaces(c *gc.C) {
	e := s.bootstrapTestEnviron(c)
	defer func() {
		err := e.Destroy(s.callCtx)
		c.Assert(err, jc.ErrorIsNil)
	}()

	opc := make(chan dummy.Operation, 200)
	dummy.Listen(opc)

	expectInfo := []network.InterfaceInfo{{
		ProviderId:       "dummy-eth0",
		ProviderSubnetId: "dummy-private",
		CIDR:             "0.10.0.0/24",
		DeviceIndex:      0,
		InterfaceName:    "eth0",
		InterfaceType:    "ethernet",
		VLANTag:          0,
		MACAddress:       "aa:bb:cc:dd:ee:f0",
		Disabled:         false,
		NoAutoStart:      false,
		ConfigType:       network.ConfigDHCP,
		Address:          network.NewAddress("0.10.0.2"),
		DNSServers:       network.NewAddresses("ns1.dummy", "ns2.dummy"),
		GatewayAddress:   network.NewAddress("0.10.0.1"),
	}, {
		ProviderId:       "dummy-eth1",
		ProviderSubnetId: "dummy-public",
		CIDR:             "0.20.0.0/24",
		DeviceIndex:      1,
		InterfaceName:    "eth1",
		InterfaceType:    "ethernet",
		VLANTag:          1,
		MACAddress:       "aa:bb:cc:dd:ee:f1",
		Disabled:         false,
		NoAutoStart:      true,
		ConfigType:       network.ConfigDHCP,
		Address:          network.NewAddress("0.20.0.2"),
		DNSServers:       network.NewAddresses("ns1.dummy", "ns2.dummy"),
		GatewayAddress:   network.NewAddress("0.20.0.1"),
	}, {
		ProviderId:       "dummy-eth2",
		ProviderSubnetId: "dummy-disabled",
		CIDR:             "0.30.0.0/24",
		DeviceIndex:      2,
		InterfaceName:    "eth2",
		InterfaceType:    "ethernet",
		VLANTag:          2,
		MACAddress:       "aa:bb:cc:dd:ee:f2",
		Disabled:         true,
		NoAutoStart:      false,
		ConfigType:       network.ConfigDHCP,
		Address:          network.NewAddress("0.30.0.2"),
		DNSServers:       network.NewAddresses("ns1.dummy", "ns2.dummy"),
		GatewayAddress:   network.NewAddress("0.30.0.1"),
	}}
	info, err := e.NetworkInterfaces(s.callCtx, "i-42")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(info, jc.DeepEquals, expectInfo)
	assertInterfaces(c, e, opc, "i-42", expectInfo)

	// Test we can induce errors.
	s.breakMethods(c, e, "NetworkInterfaces")
	info, err = e.NetworkInterfaces(s.callCtx, "i-any")
	c.Assert(err, gc.ErrorMatches, `dummy\.NetworkInterfaces is broken`)
	c.Assert(info, gc.HasLen, 0)
}

func (s *suite) TestSubnets(c *gc.C) {
	e := s.bootstrapTestEnviron(c)
	defer func() {
		err := e.Destroy(s.callCtx)
		c.Assert(err, jc.ErrorIsNil)
	}()

	opc := make(chan dummy.Operation, 200)
	dummy.Listen(opc)

	expectInfo := []corenetwork.SubnetInfo{{
		CIDR:              "0.10.0.0/24",
		ProviderId:        "dummy-private",
		AvailabilityZones: []string{"zone1", "zone2"},
	}, {
		CIDR:       "0.20.0.0/24",
		ProviderId: "dummy-public",
	}}

	ids := []corenetwork.Id{"dummy-private", "dummy-public", "foo-bar"}
	netInfo, err := e.Subnets(s.callCtx, "i-foo", ids)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(netInfo, jc.DeepEquals, expectInfo)
	assertSubnets(c, e, opc, "i-foo", ids, expectInfo)

	// Test filtering by id(s).
	netInfo, err = e.Subnets(s.callCtx, "i-foo", nil)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(netInfo, jc.DeepEquals, expectInfo)
	assertSubnets(c, e, opc, "i-foo", nil, expectInfo)
	netInfo, err = e.Subnets(s.callCtx, "i-foo", ids[0:1])
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(netInfo, jc.DeepEquals, expectInfo[0:1])
	assertSubnets(c, e, opc, "i-foo", ids[0:1], expectInfo[0:1])
	netInfo, err = e.Subnets(s.callCtx, "i-foo", ids[1:])
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(netInfo, jc.DeepEquals, expectInfo[1:])
	assertSubnets(c, e, opc, "i-foo", ids[1:], expectInfo[1:])

	// Test we can induce errors.
	s.breakMethods(c, e, "Subnets")
	netInfo, err = e.Subnets(s.callCtx, "i-any", nil)
	c.Assert(err, gc.ErrorMatches, `dummy\.Subnets is broken`)
	c.Assert(netInfo, gc.HasLen, 0)
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
	subnetIds []corenetwork.Id,
	expectInfo []corenetwork.SubnetInfo,
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
