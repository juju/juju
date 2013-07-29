// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package names_test

import (
	stdtesting "testing"

	gc "launchpad.net/gocheck"

	"launchpad.net/juju-core/names"
)

type machineSuite struct{}

var _ = gc.Suite(&unitSuite{})

func Test(t *stdtesting.T) {
	gc.TestingT(t)
}

func (s *machineSuite) TestMachineTag(c *gc.C) {
	c.Assert(names.MachineTag("10"), gc.Equals, "machine-10")
	// Check a container id.
	c.Assert(names.MachineTag("10/lxc/1"), gc.Equals, "machine-10-lxc-1")
}

func (s *machineSuite) TestMachineIdFromTag(c *gc.C) {
	id, err := names.MachineIdFromTag("machine-10")
	c.Assert(err, gc.IsNil)
	c.Assert(id, gc.Equals, "10")
	// Check a container id.
	id, err = names.MachineIdFromTag("machine-10-lxc-1")
	c.Assert(err, gc.IsNil)
	c.Assert(id, gc.Equals, "10/lxc/1")
	// Check reversability.
	nested := "2/kvm/0/lxc/3"
	id, err = names.MachineIdFromTag(names.MachineTag(nested))
	c.Assert(err, gc.IsNil)
	c.Assert(id, gc.Equals, nested)
	// Try with an invalid tag format.
	id, err = names.MachineIdFromTag("foo")
	c.Assert(err, gc.ErrorMatches, "invalid machine tag format: foo")
	c.Assert(id, gc.Equals, "")
}
