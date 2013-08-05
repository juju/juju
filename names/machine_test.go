// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package names_test

import (
	stdtesting "testing"

	gc "launchpad.net/gocheck"

	"launchpad.net/juju-core/names"
)

type machineSuite struct{}

var _ = gc.Suite(&machineSuite{})

func Test(t *stdtesting.T) {
	gc.TestingT(t)
}

func (s *machineSuite) TestMachineTag(c *gc.C) {
	c.Assert(names.MachineTag("10"), gc.Equals, "machine-10")
	// Check a container id.
	c.Assert(names.MachineTag("10/lxc/1"), gc.Equals, "machine-10-lxc-1")
}

func (s *machineSuite) TestMachineFromTag(c *gc.C) {
	kind, id, err := names.ParseTag("machine-10", names.MachineTagKind)
	c.Assert(kind, gc.Equals, names.MachineTagKind)
	c.Assert(err, gc.IsNil)
	c.Assert(id, gc.Equals, "10")

	// Check a container id.
	kind, id, err = names.ParseTag("machine-10-lxc-1", names.MachineTagKind)
	c.Assert(kind, gc.Equals, names.MachineTagKind)
	c.Assert(err, gc.IsNil)
	c.Assert(id, gc.Equals, "10/lxc/1")

	// Check reversability.
	nested := "2/kvm/0/lxc/3"
	kind, id, err = names.ParseTag(names.MachineTag(nested), names.MachineTagKind)
	c.Assert(kind, gc.Equals, names.MachineTagKind)
	c.Assert(err, gc.IsNil)
	c.Assert(id, gc.Equals, nested)

	// Try with an invalid tag formats.
	kind, id, err = names.ParseTag("foo", names.MachineTagKind)
	c.Assert(err, gc.ErrorMatches, `"foo" is not a valid machine tag`)
	c.Assert(kind, gc.Equals, "")
	c.Assert(id, gc.Equals, "")

	kind, id, err = names.ParseTag("machine-#", names.MachineTagKind)
	c.Assert(err, gc.ErrorMatches, `"machine-#" is not a valid machine tag`)
	c.Assert(kind, gc.Equals, "")
	c.Assert(id, gc.Equals, "")
}

var machineIdTests = []struct {
	pattern string
	valid   bool
}{
	{pattern: "42", valid: true},
	{pattern: "042", valid: false},
	{pattern: "0", valid: true},
	{pattern: "foo", valid: false},
	{pattern: "/", valid: false},
	{pattern: "55/", valid: false},
	{pattern: "1/foo", valid: false},
	{pattern: "2/foo/", valid: false},
	{pattern: "3/lxc/42", valid: true},
	{pattern: "3/lxc-nodash/42", valid: false},
	{pattern: "0/lxc/00", valid: false},
	{pattern: "0/lxc/0/", valid: false},
	{pattern: "03/lxc/42", valid: false},
	{pattern: "3/lxc/042", valid: false},
	{pattern: "4/foo/bar", valid: false},
	{pattern: "5/lxc/42/foo", valid: false},
	{pattern: "6/lxc/42/kvm/0", valid: true},
	{pattern: "06/lxc/42/kvm/0", valid: false},
	{pattern: "6/lxc/042/kvm/0", valid: false},
	{pattern: "6/lxc/42/kvm/00", valid: false},
	{pattern: "06/lxc/042/kvm/00", valid: false},
}

func (s *machineSuite) TestMachineIdFormats(c *gc.C) {
	for i, test := range machineIdTests {
		c.Logf("test %d: %q", i, test.pattern)
		c.Assert(names.IsMachine(test.pattern), gc.Equals, test.valid)
	}
}
