// Copyright 2021 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package network_test

import (
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"

	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/core/network"
)

type sourceSuite struct {
	testing.IsolationSuite
}

var _ = gc.Suite(&sourceSuite{})

func (*sourceSuite) TestParseInterfaceType(c *gc.C) {
	fakeSysPath := filepath.Join(c.MkDir(), network.SysClassNetPath)
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
	c.Check(result, gc.Equals, network.UnknownInterface)

	writeFakeUEvent("eth0", "IFINDEX=1", "INTERFACE=eth0")
	result = network.ParseInterfaceType(fakeSysPath, "eth0")
	c.Check(result, gc.Equals, network.UnknownInterface)

	fakeUEventPath := writeFakeUEvent("eth0.42", "DEVTYPE=vlan")
	result = network.ParseInterfaceType(fakeSysPath, "eth0.42")
	c.Check(result, gc.Equals, network.VLAN_8021QInterface)

	os.Chmod(fakeUEventPath, 0000) // permission denied error is OK
	result = network.ParseInterfaceType(fakeSysPath, "eth0.42")
	c.Check(result, gc.Equals, network.UnknownInterface)

	writeFakeUEvent("bond0", "DEVTYPE=bond")
	result = network.ParseInterfaceType(fakeSysPath, "bond0")
	c.Check(result, gc.Equals, network.BondInterface)

	writeFakeUEvent("br-ens4", "DEVTYPE=bridge")
	result = network.ParseInterfaceType(fakeSysPath, "br-ens4")
	c.Check(result, gc.Equals, network.BridgeInterface)

	// First DEVTYPE found wins.
	writeFakeUEvent("foo", "DEVTYPE=vlan", "DEVTYPE=bridge")
	result = network.ParseInterfaceType(fakeSysPath, "foo")
	c.Check(result, gc.Equals, network.VLAN_8021QInterface)

	writeFakeUEvent("fake", "DEVTYPE=warp-drive")
	result = network.ParseInterfaceType(fakeSysPath, "fake")
	c.Check(result, gc.Equals, network.UnknownInterface)
}

func (*sourceSuite) TestGetBridgePorts(c *gc.C) {
	fakeSysPath := filepath.Join(c.MkDir(), network.SysClassNetPath)
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
