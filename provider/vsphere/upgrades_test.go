// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package vsphere_test

import (
	jc "github.com/juju/testing/checkers"
	"github.com/vmware/govmomi/vim25/mo"
	"github.com/vmware/govmomi/vim25/types"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/environs"
)

type environUpgradeSuite struct {
	EnvironFixture
}

var _ = gc.Suite(&environUpgradeSuite{})

func (s *environUpgradeSuite) TestEnvironImplementsUpgrader(c *gc.C) {
	c.Assert(s.env, gc.Implements, new(environs.Upgrader))
}

func (s *environUpgradeSuite) TestEnvironUpgradeOperations(c *gc.C) {
	upgrader := s.env.(environs.Upgrader)
	ops := upgrader.UpgradeOperations(s.callCtx, environs.UpgradeOperationsParams{})
	c.Assert(ops, gc.HasLen, 1)
	c.Assert(ops[0].TargetVersion, gc.Equals, 1)
	c.Assert(ops[0].Steps, gc.HasLen, 2)
	c.Assert(ops[0].Steps[0].Description(), gc.Equals, "Update ExtraConfig properties with standard Juju tags")
	c.Assert(ops[0].Steps[1].Description(), gc.Equals, "Move VMs into controller/model folders")
}

func (s *environUpgradeSuite) TestEnvironUpgradeOperationUpdateExtraConfig(c *gc.C) {
	upgrader := s.env.(environs.Upgrader)
	step := upgrader.UpgradeOperations(s.callCtx,
		environs.UpgradeOperationsParams{
			ControllerUUID: "foo",
		})[0].Steps[0]

	vm1 := buildVM("vm-1").extraConfig("juju_controller_uuid_key", "old").vm()
	vm2 := buildVM("vm-1").extraConfig("juju_controller_uuid_key", "old").extraConfig("juju_is_controller_key", "yep").vm()
	vm3 := buildVM("vm-2").vm()
	s.client.virtualMachines = []*mo.VirtualMachine{vm1, vm2, vm3}

	err := step.Run(s.callCtx)
	c.Assert(err, jc.ErrorIsNil)

	s.client.CheckCallNames(c, "VirtualMachines", "UpdateVirtualMachineExtraConfig", "UpdateVirtualMachineExtraConfig", "Close")

	updateCall1 := s.client.Calls()[1]
	c.Assert(updateCall1.Args[1], gc.Equals, vm1)
	c.Assert(updateCall1.Args[2], jc.DeepEquals, map[string]string{
		"juju-controller-uuid": "foo",
		"juju-model-uuid":      "2d02eeac-9dbb-11e4-89d3-123b93f75cba",
	})

	updateCall2 := s.client.Calls()[2]
	c.Assert(updateCall2.Args[1], gc.Equals, vm2)
	c.Assert(updateCall2.Args[2], jc.DeepEquals, map[string]string{
		"juju-controller-uuid": "foo",
		"juju-model-uuid":      "2d02eeac-9dbb-11e4-89d3-123b93f75cba",
		"juju-is-controller":   "true",
	})
}

func (s *environUpgradeSuite) TestEnvironUpgradeOperationModelFolders(c *gc.C) {
	upgrader := s.env.(environs.Upgrader)
	step := upgrader.UpgradeOperations(s.callCtx,
		environs.UpgradeOperationsParams{
			ControllerUUID: "foo",
		})[0].Steps[1]

	vm1 := buildVM("vm-1").vm()
	vm2 := buildVM("vm-2").vm()
	vm3 := buildVM("vm-3").vm()
	s.client.virtualMachines = []*mo.VirtualMachine{vm1, vm2, vm3}

	err := step.Run(s.callCtx)
	c.Assert(err, jc.ErrorIsNil)

	s.client.CheckCallNames(c, "EnsureVMFolder", "VirtualMachines", "MoveVMsInto", "Close")
	ensureVMFolderCall := s.client.Calls()[0]
	moveVMsIntoCall := s.client.Calls()[2]
	c.Assert(ensureVMFolderCall.Args[1], gc.Equals,
		`Juju Controller (foo)/Model "testenv" (2d02eeac-9dbb-11e4-89d3-123b93f75cba)`)
	c.Assert(moveVMsIntoCall.Args[1], gc.Equals,
		`Juju Controller (foo)/Model "testenv" (2d02eeac-9dbb-11e4-89d3-123b93f75cba)`)
	c.Assert(moveVMsIntoCall.Args[2], jc.DeepEquals,
		[]types.ManagedObjectReference{vm1.Reference(), vm2.Reference(), vm3.Reference()},
	)
}
