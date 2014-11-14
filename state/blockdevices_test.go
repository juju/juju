// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state_test

import (
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/state"
	"github.com/juju/juju/storage"
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
	c.Assert(err, gc.IsNil)
}

func (s *BlockDevicesSuite) TestSetMachineBlockDevices(c *gc.C) {
	inDevices := []storage.BlockDevice{{
		DeviceName: "sda",
	}}
	err := s.machine.SetMachineBlockDevices(inDevices)
	c.Assert(err, gc.IsNil)

	outDevices, err := s.machine.BlockDevices()
	c.Assert(err, gc.IsNil)
	c.Assert(outDevices, jc.SameContents, inDevices)
}

func (s *BlockDevicesSuite) TestSetMachineBlockDevicesReplaces(c *gc.C) {
	inDevices1 := []storage.BlockDevice{{DeviceName: "sda"}}
	err := s.machine.SetMachineBlockDevices(inDevices1)
	c.Assert(err, gc.IsNil)

	inDevices2 := []storage.BlockDevice{{DeviceName: "sdb"}}
	err = s.machine.SetMachineBlockDevices(inDevices2)
	c.Assert(err, gc.IsNil)

	outDevices, err := s.machine.BlockDevices()
	c.Assert(err, gc.IsNil)
	c.Assert(outDevices, jc.SameContents, inDevices2)
}

func (s *BlockDevicesSuite) TestSetMachineBlockDevicesConcurrently(c *gc.C) {
	inDevices := []storage.BlockDevice{{
		DeviceName: "sda",
	}}
	err := s.machine.SetMachineBlockDevices(inDevices)
	c.Assert(err, gc.IsNil)

	defer state.SetBeforeHooks(c, s.State, func() {
		err := s.machine.SetMachineBlockDevices(nil)
		c.Assert(err, gc.IsNil)
	}).Check()

	inDevices[0].Label = "root"
	err = s.machine.SetMachineBlockDevices(inDevices)
	c.Assert(err, gc.IsNil)

	outDevices, err := s.machine.BlockDevices()
	c.Assert(err, gc.IsNil)
	c.Assert(outDevices, jc.SameContents, inDevices)
}

func (s *BlockDevicesSuite) TestSetMachineBlockDevicesEmpty(c *gc.C) {
	for _, input := range [][]storage.BlockDevice{
		nil,
		[]storage.BlockDevice{},
	} {
		err := s.machine.SetMachineBlockDevices(input)
		c.Assert(err, gc.IsNil)
		outDevices, err := s.machine.BlockDevices()
		c.Assert(err, gc.IsNil)
		c.Assert(outDevices, gc.NotNil)
		c.Assert(outDevices, gc.HasLen, 0)
	}
}

func (s *BlockDevicesSuite) TestBlockDevicesMachineRemove(c *gc.C) {
	err := s.machine.SetMachineBlockDevices([]storage.BlockDevice{{
		DeviceName: "sda",
	}})
	c.Assert(err, gc.IsNil)

	err = s.machine.EnsureDead()
	c.Assert(err, gc.IsNil)
	err = s.machine.Remove()
	c.Assert(err, gc.IsNil)

	outDevices, err := s.machine.BlockDevices()
	c.Assert(err, gc.IsNil)
	c.Assert(outDevices, gc.HasLen, 0)
}
