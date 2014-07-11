// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package network_test

import (
	jc "github.com/juju/testing/checkers"
	gc "launchpad.net/gocheck"

	"github.com/juju/juju/network"
	"github.com/juju/juju/testing"
)

type PortSuite struct {
	testing.BaseSuite
}

var _ = gc.Suite(&PortSuite{})

type hostPortTest struct {
	about         string
	hostPorts     []network.HostPort
	expectedIndex int
	preferIPv6    bool
}

// hostPortTest returns the HostPort equivalent test to the
// receiving selectTest.
func (t selectTest) hostPortTest() hostPortTest {
	hps := network.AddressesWithPort(t.addresses, 9999)
	for i := range hps {
		hps[i].Port = i + 1
	}
	return hostPortTest{
		about:         t.about,
		hostPorts:     hps,
		expectedIndex: t.expectedIndex,
		preferIPv6:    t.preferIPv6,
	}
}

// expected returns the expected host:port result
// of the test.
func (t hostPortTest) expected() string {
	if t.expectedIndex == -1 {
		return ""
	}
	return t.hostPorts[t.expectedIndex].NetAddr()
}

func (s *PortSuite) TestSelectPublicHostPort(c *gc.C) {
	oldValue := network.GetPreferIPv6()
	defer func() {
		network.SetPreferIPv6(oldValue)
	}()
	for i, t0 := range selectPublicTests {
		t := t0.hostPortTest()
		c.Logf("test %d: %s", i, t.about)
		network.SetPreferIPv6(t.preferIPv6)
		c.Check(network.SelectPublicHostPort(t.hostPorts), jc.DeepEquals, t.expected())
	}
}

func (s *PortSuite) TestSelectInternalHostPort(c *gc.C) {
	oldValue := network.GetPreferIPv6()
	defer func() {
		network.SetPreferIPv6(oldValue)
	}()
	for i, t0 := range selectInternalTests {
		t := t0.hostPortTest()
		c.Logf("test %d: %s", i, t.about)
		network.SetPreferIPv6(t.preferIPv6)
		c.Check(network.SelectInternalHostPort(t.hostPorts, false), jc.DeepEquals, t.expected())
	}
}

func (s *PortSuite) TestSelectInternalMachineHostPort(c *gc.C) {
	oldValue := network.GetPreferIPv6()
	defer func() {
		network.SetPreferIPv6(oldValue)
	}()
	for i, t0 := range selectInternalMachineTests {
		t := t0.hostPortTest()
		c.Logf("test %d: %s", i, t.about)
		network.SetPreferIPv6(t.preferIPv6)
		c.Check(network.SelectInternalHostPort(t.hostPorts, true), gc.DeepEquals, t.expected())
	}
}

func (*PortSuite) TestAddressesWithPort(c *gc.C) {
	addrs := network.NewAddresses("0.1.2.3", "0.2.4.6")
	hps := network.AddressesWithPort(addrs, 999)
	c.Assert(hps, jc.DeepEquals, []network.HostPort{{
		Address: network.NewAddress("0.1.2.3", network.ScopeUnknown),
		Port:    999,
	}, {
		Address: network.NewAddress("0.2.4.6", network.ScopeUnknown),
		Port:    999,
	}})
}
