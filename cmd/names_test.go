// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package cmd_test

import (
	gc "launchpad.net/gocheck"

	"launchpad.net/juju-core/cmd"
)

type namesSuite struct {
}

var _ = gc.Suite(&namesSuite{})

func (*namesSuite) TestNameChecks(c *gc.C) {
	assertMachineOrNewContainer := func(s string, expect bool) {
		c.Assert(cmd.IsMachineOrNewContainer(s), gc.Equals, expect)
	}
	assertMachineOrNewContainer("0", true)
	assertMachineOrNewContainer("00", false)
	assertMachineOrNewContainer("1", true)
	assertMachineOrNewContainer("0/lxc/0", true)
	assertMachineOrNewContainer("lxc:0", true)
	assertMachineOrNewContainer("lxc:lxc:0", false)
	assertMachineOrNewContainer("kvm:0/lxc/1", true)
	assertMachineOrNewContainer("lxc:", false)
	assertMachineOrNewContainer(":lxc", false)
	assertMachineOrNewContainer("0/lxc/", false)
	assertMachineOrNewContainer("0/lxc", false)
	assertMachineOrNewContainer("kvm:0/lxc", false)
	assertMachineOrNewContainer("0/lxc/01", false)
	assertMachineOrNewContainer("0/lxc/10", true)
	assertMachineOrNewContainer("0/kvm/4", true)
}
