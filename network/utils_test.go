// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

// +build !windows

package network_test

import (
	"errors"
	"io/ioutil"
	"net"
	"os"
	"path/filepath"
	"strings"

	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	corenetwork "github.com/juju/juju/core/network"
	"github.com/juju/juju/network"
)

type UtilsSuite struct {
	testing.IsolationSuite
}

var _ = gc.Suite(&UtilsSuite{})

func (s *UtilsSuite) TestSupportsIPv6Error(c *gc.C) {
	s.PatchValue(network.NetListen, func(netFamily, bindAddress string) (net.Listener, error) {
		c.Check(netFamily, gc.Equals, "tcp6")
		c.Check(bindAddress, gc.Equals, "[::1]:0")
		return nil, errors.New("boom!")
	})
	c.Check(network.SupportsIPv6(), jc.IsFalse)
}

func (s *UtilsSuite) TestSupportsIPv6OK(c *gc.C) {
	s.PatchValue(network.NetListen, func(_, _ string) (net.Listener, error) {
		return &mockListener{}, nil
	})
	c.Check(network.SupportsIPv6(), jc.IsTrue)
}

func (*UtilsSuite) TestParseInterfaceType(c *gc.C) {
	fakeSysPath := filepath.Join(c.MkDir(), corenetwork.SysClassNetPath)
	err := os.MkdirAll(fakeSysPath, 0700)
	c.Check(err, jc.ErrorIsNil)

	writeFakeUEvent := func(interfaceName string, lines ...string) string {
		fakeInterfacePath := filepath.Join(fakeSysPath, interfaceName)
		err := os.MkdirAll(fakeInterfacePath, 0700)
		c.Check(err, jc.ErrorIsNil)

		fakeUEventPath := filepath.Join(fakeInterfacePath, "uevent")
		contents := strings.Join(lines, "\n")
		err = ioutil.WriteFile(fakeUEventPath, []byte(contents), 0644)
		c.Check(err, jc.ErrorIsNil)
		return fakeUEventPath
	}

	result := network.ParseInterfaceType(fakeSysPath, "missing")
	c.Check(result, gc.Equals, corenetwork.UnknownInterface)

	writeFakeUEvent("eth0", "IFINDEX=1", "INTERFACE=eth0")
	result = network.ParseInterfaceType(fakeSysPath, "eth0")
	c.Check(result, gc.Equals, corenetwork.UnknownInterface)

	fakeUEventPath := writeFakeUEvent("eth0.42", "DEVTYPE=vlan")
	result = network.ParseInterfaceType(fakeSysPath, "eth0.42")
	c.Check(result, gc.Equals, corenetwork.VLAN_8021QInterface)

	os.Chmod(fakeUEventPath, 0000) // permission denied error is OK
	result = network.ParseInterfaceType(fakeSysPath, "eth0.42")
	c.Check(result, gc.Equals, corenetwork.UnknownInterface)

	writeFakeUEvent("bond0", "DEVTYPE=bond")
	result = network.ParseInterfaceType(fakeSysPath, "bond0")
	c.Check(result, gc.Equals, corenetwork.BondInterface)

	writeFakeUEvent("br-ens4", "DEVTYPE=bridge")
	result = network.ParseInterfaceType(fakeSysPath, "br-ens4")
	c.Check(result, gc.Equals, corenetwork.BridgeInterface)

	// First DEVTYPE found wins.
	writeFakeUEvent("foo", "DEVTYPE=vlan", "DEVTYPE=bridge")
	result = network.ParseInterfaceType(fakeSysPath, "foo")
	c.Check(result, gc.Equals, corenetwork.VLAN_8021QInterface)

	writeFakeUEvent("fake", "DEVTYPE=warp-drive")
	result = network.ParseInterfaceType(fakeSysPath, "fake")
	c.Check(result, gc.Equals, corenetwork.UnknownInterface)
}

func (*UtilsSuite) TestGetBridgePorts(c *gc.C) {
	fakeSysPath := filepath.Join(c.MkDir(), corenetwork.SysClassNetPath)
	err := os.MkdirAll(fakeSysPath, 0700)
	c.Check(err, jc.ErrorIsNil)

	writeFakePorts := func(bridgeName string, portNames ...string) {
		fakePortsPath := filepath.Join(fakeSysPath, bridgeName, "brif")
		err := os.MkdirAll(fakePortsPath, 0700)
		c.Check(err, jc.ErrorIsNil)

		for _, portName := range portNames {
			portPath := filepath.Join(fakePortsPath, portName)
			err = ioutil.WriteFile(portPath, []byte(""), 0644)
			c.Check(err, jc.ErrorIsNil)
		}
	}

	result := network.GetBridgePorts(fakeSysPath, "missing")
	c.Check(result, gc.IsNil)

	writeFakePorts("br-eth0")
	result = network.GetBridgePorts(fakeSysPath, "br-eth0")
	c.Check(result, gc.IsNil)

	writeFakePorts("br-eth0", "eth0")
	result = network.GetBridgePorts(fakeSysPath, "br-eth0")
	c.Check(result, jc.DeepEquals, []string{"eth0"})

	writeFakePorts("br-ovs", "eth0", "eth1", "eth2")
	result = network.GetBridgePorts(fakeSysPath, "br-ovs")
	c.Check(result, jc.DeepEquals, []string{"eth0", "eth1", "eth2"})
}

type mockListener struct {
	net.Listener
}

func (*mockListener) Close() error {
	return nil
}
