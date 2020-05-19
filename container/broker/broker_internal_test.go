// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package broker

import (
	"os/exec"

	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/core/network"
)

type brokerSuite struct {
	testing.IsolationSuite
}

var _ = gc.Suite(&brokerSuite{})

func (s *brokerSuite) SetUpSuite(c *gc.C) {
	s.IsolationSuite.SetUpSuite(c)
}

func (s *brokerSuite) SetUpTest(c *gc.C) {
	s.IsolationSuite.SetUpTest(c)
}

func (s *brokerSuite) TestExistingOVSManagedBridges(c *gc.C) {
	// Patch output for "ovs-vsctl list-br" and make sure exec.LookPath can
	// detect it in the path
	testing.PatchExecutableAsEchoArgs(c, s, "ovs-vsctl", 0)
	s.PatchValue(&getCommandOutput, func(cmd *exec.Cmd) ([]byte, error) {
		c.Assert(cmd.Args, gc.DeepEquals, []string{"ovs-vsctl", "list-br"}, gc.Commentf("expected ovs-vsctl to be invoked with 'list-br' as an argument"))
		return []byte("ovsbr1" + "\n"), nil
	})

	ifaces := network.InterfaceInfos{
		{InterfaceName: "eth0"},
		{InterfaceName: "eth1"},
		{InterfaceName: "lxdbr0"},
		{InterfaceName: "ovsbr1"},
	}

	ovsIfaces, err := ovsManagedBridges(ifaces)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(ovsIfaces, gc.HasLen, 1, gc.Commentf("expected ovs-managed bridge list to contain a single entry"))
	c.Assert(ovsIfaces[0].InterfaceName, gc.Equals, "ovsbr1", gc.Commentf("expected ovs-managed bridge list to contain iface 'ovsbr1'"))
}

func (s *brokerSuite) TestNonExistingOVSManagedBridges(c *gc.C) {
	// Patch output for "ovs-vsctl list-br" and make sure exec.LookPath can
	// detect it in the path
	testing.PatchExecutableAsEchoArgs(c, s, "ovs-vsctl", 0)
	s.PatchValue(&getCommandOutput, func(cmd *exec.Cmd) ([]byte, error) {
		c.Assert(cmd.Args, gc.DeepEquals, []string{"ovs-vsctl", "list-br"}, gc.Commentf("expected ovs-vsctl to be invoked with 'list-br' as an argument"))
		return []byte("\n"), nil
	})

	ifaces := network.InterfaceInfos{
		{InterfaceName: "eth0"},
		{InterfaceName: "eth1"},
		{InterfaceName: "lxdbr0"},
	}

	ovsIfaces, err := ovsManagedBridges(ifaces)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(ovsIfaces, gc.HasLen, 0, gc.Commentf("expected ovs-managed bridge list to be empty"))
}

func (s *brokerSuite) TestMissingOVSTools(c *gc.C) {
	ifaces := network.InterfaceInfos{{InterfaceName: "eth0"}}
	ovsIfaces, err := ovsManagedBridges(ifaces)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(ovsIfaces, gc.HasLen, 0, gc.Commentf("expected ovs-managed bridge list to be empty"))
}
