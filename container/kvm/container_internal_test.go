// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package kvm

import (
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/network"
	"github.com/juju/testing"
)

// gocheck boilerplate.
type containerInternalSuite struct {
	testing.IsolationSuite
}

var _ = gc.Suite(&containerInternalSuite{})

func (_ containerInternalSuite) TestInterfaceInfo(c *gc.C) {
	i := interfaceInfo{config: network.InterfaceInfo{
		MACAddress: "mac", ParentInterfaceName: "piname", InterfaceName: "iname"}}
	c.Check(i.InterfaceName(), gc.Equals, "iname")
	c.Check(i.ParentInterfaceName(), gc.Equals, "piname")
	c.Assert(i.MACAddress(), gc.Equals, "mac")
}
