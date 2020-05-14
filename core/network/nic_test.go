// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package network_test

import (
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/core/network"
)

type nicSuite struct {
	testing.IsolationSuite
}

var _ = gc.Suite(&nicSuite{})

func (*nicSuite) TestInterfaceInfosChildren(c *gc.C) {
	interfaces := getInterFaceInfos()

	c.Check(interfaces.Children(""), gc.DeepEquals, interfaces[:2])
	c.Check(interfaces.Children("bond0"), gc.DeepEquals, network.InterfaceInfos{
		interfaces[3], interfaces[4],
	})
	c.Check(interfaces.Children("eth2"), gc.HasLen, 0)
}

func (*nicSuite) TestInterfaceInfosIterHierarchy(c *gc.C) {
	var devs []string
	f := func(info network.InterfaceInfo) error {
		devs = append(devs, info.ParentInterfaceName+":"+info.InterfaceName)
		return nil
	}

	c.Assert(getInterFaceInfos().IterHierarchy(f), jc.ErrorIsNil)

	c.Check(devs, gc.DeepEquals, []string{
		":br-bond0",
		"br-bond0:bond0",
		"bond0:eth0",
		"bond0:eth1",
		":eth2",
	})
}

func getInterFaceInfos() network.InterfaceInfos {
	return network.InterfaceInfos{
		{
			DeviceIndex:   0,
			InterfaceName: "br-bond0",
		},
		{
			DeviceIndex:   1,
			InterfaceName: "eth2",
		},
		{
			DeviceIndex:         2,
			InterfaceName:       "bond0",
			ParentInterfaceName: "br-bond0",
		},
		{
			DeviceIndex:         3,
			InterfaceName:       "eth0",
			ParentInterfaceName: "bond0",
		},
		{
			DeviceIndex:         4,
			InterfaceName:       "eth1",
			ParentInterfaceName: "bond0",
		},
	}
}
