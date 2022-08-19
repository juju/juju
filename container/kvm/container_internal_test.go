// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package kvm

import (
	"github.com/juju/testing"
	gc "gopkg.in/check.v1"

	corenetwork "github.com/juju/juju/core/network"
)

type containerInternalSuite struct {
	testing.IsolationSuite
}

var _ = gc.Suite(&containerInternalSuite{})

func (containerInternalSuite) TestInterfaceInfo(c *gc.C) {
	i := interfaceInfo{config: corenetwork.InterfaceInfo{
		MACAddress: "mac", ParentInterfaceName: "piname", InterfaceName: "iname"}}
	c.Check(i.InterfaceName(), gc.Equals, "iname")
	c.Check(i.ParentInterfaceName(), gc.Equals, "piname")
	c.Assert(i.MACAddress(), gc.Equals, "mac")
}
