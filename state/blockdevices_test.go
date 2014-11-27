// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state_test

import (
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/state"
)

type BlockDevicesSuite struct {
	ConnSuite
	machine *state.Machine
}

var _ = gc.Suite(&BlockDevicesSuite{})

func (s *BlockDevicesSuite) SetUpTest(c *gc.C) {
	s.ConnSuite.SetUpTest(c)
	var err error
	s.machine, err = s.State.AddMachine("quantal", state.JobHostUnits)
	c.Assert(err, jc.ErrorIsNil)
}

func (s *BlockDevicesSuite) assertBlockDevices(c *gc.C, expected map[string]state.BlockDeviceInfo) {
	devices, err := s.machine.BlockDevices()
	c.Assert(err, gc.IsNil)
	info := make(map[string]state.BlockDeviceInfo)
	for _, dev := range devices {
		info[dev.Name()] = dev.Info()
	}
	c.Assert(info, gc.DeepEquals, expected)
}

func (s *BlockDevicesSuite) TestSetMachineBlockDevices(c *gc.C) {
	sda := state.BlockDeviceInfo{DeviceName: "sda"}
	err := s.machine.SetMachineBlockDevices(sda)
	c.Assert(err, gc.IsNil)
	s.assertBlockDevices(c, map[string]state.BlockDeviceInfo{"0": sda})
}

func (s *BlockDevicesSuite) TestSetMachineBlockDevicesReplaces(c *gc.C) {
	sda := state.BlockDeviceInfo{DeviceName: "sda"}
	err := s.machine.SetMachineBlockDevices(sda)
	c.Assert(err, gc.IsNil)

	sdb := state.BlockDeviceInfo{DeviceName: "sdb"}
	err = s.machine.SetMachineBlockDevices(sdb)
	c.Assert(err, gc.IsNil)
	s.assertBlockDevices(c, map[string]state.BlockDeviceInfo{"1": sdb})
}

func (s *BlockDevicesSuite) TestSetMachineBlockDevicesUpdates(c *gc.C) {
	sda := state.BlockDeviceInfo{DeviceName: "sda"}
	sdb := state.BlockDeviceInfo{DeviceName: "sdb"}
	err := s.machine.SetMachineBlockDevices(sda, sdb)
	c.Assert(err, gc.IsNil)
	s.assertBlockDevices(c, map[string]state.BlockDeviceInfo{"0": sda, "1": sdb})

	sdb.Label = "root"
	err = s.machine.SetMachineBlockDevices(sdb)
	c.Assert(err, gc.IsNil)
	s.assertBlockDevices(c, map[string]state.BlockDeviceInfo{"1": sdb})

	// If a device is attached, unattached, then attached again,
	// then it gets a new name.
	sdb.Label = "" // Label should be reset.
	err = s.machine.SetMachineBlockDevices(sda, sdb)
	c.Assert(err, gc.IsNil)
	s.assertBlockDevices(c, map[string]state.BlockDeviceInfo{
		"2": sda,
		"1": sdb,
	})
}

func (s *BlockDevicesSuite) TestSetMachineBlockDevicesConcurrently(c *gc.C) {
	sdaInner := state.BlockDeviceInfo{DeviceName: "sda"}
	defer state.SetBeforeHooks(c, s.State, func() {
		err := s.machine.SetMachineBlockDevices(sdaInner)
		c.Assert(err, gc.IsNil)
	}).Check()

	sdaOuter := state.BlockDeviceInfo{
		DeviceName: "sda",
		Label:      "root",
	}
	err := s.machine.SetMachineBlockDevices(sdaOuter)
	c.Assert(err, gc.IsNil)

	// SetMachineBlockDevices will not remove concurrently added
	// block devices. This is fine in practice, because there is
	// a single worker responsible for populating machine block
	// devices.
	s.assertBlockDevices(c, map[string]state.BlockDeviceInfo{
		"1": sdaInner,
		// The outer call gets 0 because it's called first;
		// the before-hook call is called second but completes
		// first.
		"0": sdaOuter,
	})
}

func (s *BlockDevicesSuite) TestSetMachineBlockDevicesEmpty(c *gc.C) {
	sda := state.BlockDeviceInfo{DeviceName: "sda"}
	err := s.machine.SetMachineBlockDevices(sda)
	c.Assert(err, gc.IsNil)
	s.assertBlockDevices(c, map[string]state.BlockDeviceInfo{"0": sda})

	err = s.machine.SetMachineBlockDevices()
	c.Assert(err, gc.IsNil)
	s.assertBlockDevices(c, map[string]state.BlockDeviceInfo{})
}

func (s *BlockDevicesSuite) TestBlockDevicesMachineRemove(c *gc.C) {
	sda := state.BlockDeviceInfo{DeviceName: "sda"}
	err := s.machine.SetMachineBlockDevices(sda)
	c.Assert(err, gc.IsNil)

	err = s.machine.EnsureDead()
	c.Assert(err, jc.ErrorIsNil)
	err = s.machine.Remove()
	c.Assert(err, jc.ErrorIsNil)

	s.assertBlockDevices(c, map[string]state.BlockDeviceInfo{})
}
