// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package network

import (
	"os/exec"
	stdtesting "testing"

	"github.com/juju/tc"

	"github.com/juju/juju/internal/testhelpers"
)

type ovsSuite struct {
	testhelpers.IsolationSuite
}

func TestOvsSuite(t *stdtesting.T) { tc.Run(t, &ovsSuite{}) }
func (s *ovsSuite) SetUpSuite(c *tc.C) {
	s.IsolationSuite.SetUpSuite(c)
}

func (s *ovsSuite) SetUpTest(c *tc.C) {
	s.IsolationSuite.SetUpTest(c)
}

func (s *ovsSuite) TestExistingOvsManagedBridgeInterfaces(c *tc.C) {
	// Patch output for "ovs-vsctl list-br" and make sure exec.LookPath can
	// detect it in the path
	testhelpers.PatchExecutableAsEchoArgs(c, s, "ovs-vsctl", 0)
	s.PatchValue(&getCommandOutput, func(cmd *exec.Cmd) ([]byte, error) {
		c.Assert(cmd.Args, tc.DeepEquals, []string{"ovs-vsctl", "list-br"}, tc.Commentf("expected ovs-vsctl to be invoked with 'list-br' as an argument"))
		return []byte("ovsbr1" + "\n"), nil
	})

	ifaces := InterfaceInfos{
		{InterfaceName: "eth0"},
		{InterfaceName: "eth1"},
		{InterfaceName: "lxdbr0"},
		{InterfaceName: "ovsbr1"},
	}

	ovsIfaces, err := OvsManagedBridgeInterfaces(ifaces)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(ovsIfaces, tc.HasLen, 1, tc.Commentf("expected ovs-managed bridge list to contain a single entry"))
	c.Assert(ovsIfaces[0].InterfaceName, tc.Equals, "ovsbr1", tc.Commentf("expected ovs-managed bridge list to contain iface 'ovsbr1'"))
}

func (s *ovsSuite) TestNonExistingOvsManagedBridgeInterfaces(c *tc.C) {
	// Patch output for "ovs-vsctl list-br" and make sure exec.LookPath can
	// detect it in the path
	testhelpers.PatchExecutableAsEchoArgs(c, s, "ovs-vsctl", 0)
	s.PatchValue(&getCommandOutput, func(cmd *exec.Cmd) ([]byte, error) {
		c.Assert(cmd.Args, tc.DeepEquals, []string{"ovs-vsctl", "list-br"}, tc.Commentf("expected ovs-vsctl to be invoked with 'list-br' as an argument"))
		return []byte("\n"), nil
	})

	ifaces := InterfaceInfos{
		{InterfaceName: "eth0"},
		{InterfaceName: "eth1"},
		{InterfaceName: "lxdbr0"},
	}

	ovsIfaces, err := OvsManagedBridgeInterfaces(ifaces)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(ovsIfaces, tc.HasLen, 0, tc.Commentf("expected ovs-managed bridge list to be empty"))
}

func (s *ovsSuite) TestMissingOvsTools(c *tc.C) {
	ifaces := InterfaceInfos{{InterfaceName: "eth0"}}
	ovsIfaces, err := OvsManagedBridgeInterfaces(ifaces)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(ovsIfaces, tc.HasLen, 0, tc.Commentf("expected ovs-managed bridge list to be empty"))
}
