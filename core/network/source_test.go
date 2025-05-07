// Copyright 2021 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package network_test

import (
	"os"
	"path/filepath"
	"strings"

	"github.com/juju/tc"
	"github.com/juju/testing"

	"github.com/juju/juju/core/network"
)

type sourceSuite struct {
	testing.IsolationSuite
}

var _ = tc.Suite(&sourceSuite{})

func (*sourceSuite) TestParseInterfaceType(c *tc.C) {
	fakeSysPath := filepath.Join(c.MkDir(), network.SysClassNetPath)
	err := os.MkdirAll(fakeSysPath, 0700)
	c.Check(err, tc.ErrorIsNil)

	writeFakeUEvent := func(interfaceName string, lines ...string) string {
		fakeInterfacePath := filepath.Join(fakeSysPath, interfaceName)
		err := os.MkdirAll(fakeInterfacePath, 0700)
		c.Check(err, tc.ErrorIsNil)

		fakeUEventPath := filepath.Join(fakeInterfacePath, "uevent")
		contents := strings.Join(lines, "\n")
		err = os.WriteFile(fakeUEventPath, []byte(contents), 0644)
		c.Check(err, tc.ErrorIsNil)
		return fakeUEventPath
	}

	result := network.ParseInterfaceType(fakeSysPath, "missing")
	c.Check(result, tc.Equals, network.UnknownDevice)

	writeFakeUEvent("eth0", "IFINDEX=1", "INTERFACE=eth0")
	result = network.ParseInterfaceType(fakeSysPath, "eth0")
	c.Check(result, tc.Equals, network.UnknownDevice)

	fakeUEventPath := writeFakeUEvent("eth0.42", "DEVTYPE=vlan")
	result = network.ParseInterfaceType(fakeSysPath, "eth0.42")
	c.Check(result, tc.Equals, network.VLAN8021QDevice)

	os.Chmod(fakeUEventPath, 0000) // permission denied error is OK
	result = network.ParseInterfaceType(fakeSysPath, "eth0.42")
	c.Check(result, tc.Equals, network.UnknownDevice)

	writeFakeUEvent("bond0", "DEVTYPE=bond")
	result = network.ParseInterfaceType(fakeSysPath, "bond0")
	c.Check(result, tc.Equals, network.BondDevice)

	writeFakeUEvent("br-ens4", "DEVTYPE=bridge")
	result = network.ParseInterfaceType(fakeSysPath, "br-ens4")
	c.Check(result, tc.Equals, network.BridgeDevice)

	// First DEVTYPE found wins.
	writeFakeUEvent("foo", "DEVTYPE=vlan", "DEVTYPE=bridge")
	result = network.ParseInterfaceType(fakeSysPath, "foo")
	c.Check(result, tc.Equals, network.VLAN8021QDevice)

	writeFakeUEvent("fake", "DEVTYPE=warp-drive")
	result = network.ParseInterfaceType(fakeSysPath, "fake")
	c.Check(result, tc.Equals, network.UnknownDevice)
}

func (*sourceSuite) TestGetBridgePorts(c *tc.C) {
	fakeSysPath := filepath.Join(c.MkDir(), network.SysClassNetPath)
	err := os.MkdirAll(fakeSysPath, 0700)
	c.Check(err, tc.ErrorIsNil)

	writeFakePorts := func(bridgeName string, portNames ...string) {
		fakePortsPath := filepath.Join(fakeSysPath, bridgeName, "brif")
		err := os.MkdirAll(fakePortsPath, 0700)
		c.Check(err, tc.ErrorIsNil)

		for _, portName := range portNames {
			portPath := filepath.Join(fakePortsPath, portName)
			err = os.WriteFile(portPath, []byte(""), 0644)
			c.Check(err, tc.ErrorIsNil)
		}
	}

	result := network.GetBridgePorts(fakeSysPath, "missing")
	c.Check(result, tc.IsNil)

	writeFakePorts("br-eth0")
	result = network.GetBridgePorts(fakeSysPath, "br-eth0")
	c.Check(result, tc.IsNil)

	writeFakePorts("br-eth0", "eth0")
	result = network.GetBridgePorts(fakeSysPath, "br-eth0")
	c.Check(result, tc.DeepEquals, []string{"eth0"})

	writeFakePorts("br-ovs", "eth0", "eth1", "eth2")
	result = network.GetBridgePorts(fakeSysPath, "br-ovs")
	c.Check(result, tc.DeepEquals, []string{"eth0", "eth1", "eth2"})
}
