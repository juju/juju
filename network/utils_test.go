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
