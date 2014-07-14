// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package dummy_test

import (
	"net/url"
	stdtesting "testing"
	"time"

	jc "github.com/juju/testing/checkers"
	gc "launchpad.net/gocheck"

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
	"github.com/juju/juju/version"
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
	jujutest.LiveTests
}

func (s *liveSuite) SetUpSuite(c *gc.C) {
	s.BaseSuite.SetUpSuite(c)
	s.LiveTests.SetUpSuite(c)
}

func (s *liveSuite) TearDownSuite(c *gc.C) {
	s.LiveTests.TearDownSuite(c)
	s.BaseSuite.TearDownSuite(c)
}

func (s *liveSuite) SetUpTest(c *gc.C) {
	s.BaseSuite.SetUpTest(c)
	s.LiveTests.SetUpTest(c)
}

func (s *liveSuite) TearDownTest(c *gc.C) {
	s.LiveTests.TearDownTest(c)
	s.BaseSuite.TearDownTest(c)
}

type suite struct {
	testing.BaseSuite
	jujutest.Tests
}

func (s *suite) SetUpTest(c *gc.C) {
	s.BaseSuite.SetUpTest(c)
	s.Tests.SetUpTest(c)
}

func (s *suite) TearDownTest(c *gc.C) {
	s.Tests.TearDownTest(c)
	dummy.Reset()
	s.BaseSuite.TearDownTest(c)
}

func (s *suite) bootstrapTestEnviron(c *gc.C, preferIPv6 bool) environs.Environ {
	s.TestConfig["prefer-ipv6"] = preferIPv6
	cfg, err := config.New(config.NoDefaults, s.TestConfig)
	c.Assert(err, gc.IsNil)
	e, err := environs.Prepare(cfg, testing.Context(c), s.ConfigStore)
	c.Assert(err, gc.IsNil, gc.Commentf("preparing environ %#v", s.TestConfig))
	c.Assert(e, gc.NotNil)

	envtesting.UploadFakeTools(c, e.Storage())
	err = bootstrap.EnsureNotBootstrapped(e)
	c.Assert(err, gc.IsNil)
	err = bootstrap.Bootstrap(testing.Context(c), e, environs.BootstrapParams{})
	c.Assert(err, gc.IsNil)
	return e
}

func (s *suite) TestAllocateAddress(c *gc.C) {
	e := s.bootstrapTestEnviron(c, false)

	inst, _ := jujutesting.AssertStartInstance(c, e, "0")
	c.Assert(inst, gc.NotNil)
	netId := network.Id("net1")

	opc := make(chan dummy.Operation, 200)
	dummy.Listen(opc)

	expectAddress := network.NewAddress("0.1.2.1", network.ScopeCloudLocal)
	address, err := e.AllocateAddress(inst.Id(), netId)
	c.Assert(err, gc.IsNil)
	c.Assert(address, gc.DeepEquals, expectAddress)

	assertAllocateAddress(c, e, opc, inst.Id(), netId, expectAddress)

	expectAddress = network.NewAddress("0.1.2.2", network.ScopeCloudLocal)
	address, err = e.AllocateAddress(inst.Id(), netId)
	c.Assert(err, gc.IsNil)
	c.Assert(address, gc.DeepEquals, expectAddress)
	assertAllocateAddress(c, e, opc, inst.Id(), netId, expectAddress)
}

func (s *suite) TestListNetworks(c *gc.C) {
	e := s.bootstrapTestEnviron(c, false)

	opc := make(chan dummy.Operation, 200)
	dummy.Listen(opc)

	expectInfo := []network.BasicInfo{
		{CIDR: "0.10.0.0/8", ProviderId: "dummy-private"},
		{CIDR: "0.20.0.0/24", ProviderId: "dummy-public"},
	}
	netInfo, err := e.ListNetworks()
	c.Assert(err, gc.IsNil)
	c.Assert(netInfo, jc.DeepEquals, expectInfo)
	assertListNetworks(c, e, opc, expectInfo)
}

func (s *suite) TestPreferIPv6On(c *gc.C) {
	e := s.bootstrapTestEnviron(c, true)
	inst, _ := jujutesting.AssertStartInstance(c, e, "0")
	c.Assert(inst, gc.NotNil)
	addrs, err := inst.Addresses()
	c.Assert(err, gc.IsNil)
	c.Assert(addrs, jc.DeepEquals, network.NewAddresses("only-0.dns", "127.0.0.1", "fc00::1"))
	storageURL, err := e.Storage().URL("tools/releases/juju-" + version.Current.String() + ".tgz")
	c.Assert(err, gc.IsNil)
	toolsURL, err := url.Parse(storageURL)
	c.Assert(err, gc.IsNil)
	c.Assert(toolsURL.Host, gc.Matches, `\[::1\]:\d+`)
}

func (s *suite) TestPreferIPv6Off(c *gc.C) {
	e := s.bootstrapTestEnviron(c, false)
	inst, _ := jujutesting.AssertStartInstance(c, e, "0")
	c.Assert(inst, gc.NotNil)
	addrs, err := inst.Addresses()
	c.Assert(err, gc.IsNil)
	c.Assert(addrs, jc.DeepEquals, network.NewAddresses("only-0.dns", "127.0.0.1"))
	storageURL, err := e.Storage().URL("tools/releases/juju-" + version.Current.String() + ".tgz")
	c.Assert(err, gc.IsNil)
	toolsURL, err := url.Parse(storageURL)
	c.Assert(err, gc.IsNil)
	c.Assert(toolsURL.Host, gc.Matches, `127\.0\.0\.1:\d+`)
}

func assertAllocateAddress(c *gc.C, e environs.Environ, opc chan dummy.Operation, expectInstId instance.Id, expectNetId network.Id, expectAddress network.Address) {
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
	case <-time.After(testing.ShortWait):
		c.Fatalf("time out wating for operation")
	}
}

func assertListNetworks(c *gc.C, e environs.Environ, opc chan dummy.Operation, expectInfo []network.BasicInfo) {
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
