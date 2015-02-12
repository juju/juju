// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state_test

import (
	"github.com/juju/errors"
	"github.com/juju/names"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/state"
	"github.com/juju/juju/state/testing"
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

func (s *BlockDevicesSuite) assertBlockDevices(c *gc.C, tag names.MachineTag, expected []state.BlockDeviceInfo) {
	info, err := s.State.BlockDevices(tag)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(info, gc.DeepEquals, expected)
}

func (s *BlockDevicesSuite) TestSetMachineBlockDevices(c *gc.C) {
	sda := state.BlockDeviceInfo{DeviceName: "sda"}
	err := s.machine.SetMachineBlockDevices(sda)
	c.Assert(err, jc.ErrorIsNil)
	s.assertBlockDevices(c, s.machine.MachineTag(), []state.BlockDeviceInfo{sda})
}

func (s *BlockDevicesSuite) TestSetMachineBlockDevicesReplaces(c *gc.C) {
	sda := state.BlockDeviceInfo{DeviceName: "sda"}
	err := s.machine.SetMachineBlockDevices(sda)
	c.Assert(err, jc.ErrorIsNil)

	sdb := state.BlockDeviceInfo{DeviceName: "sdb"}
	err = s.machine.SetMachineBlockDevices(sdb)
	c.Assert(err, jc.ErrorIsNil)
	s.assertBlockDevices(c, s.machine.MachineTag(), []state.BlockDeviceInfo{sdb})
}

func (s *BlockDevicesSuite) TestSetMachineBlockDevicesUpdates(c *gc.C) {
	sda := state.BlockDeviceInfo{DeviceName: "sda"}
	sdb := state.BlockDeviceInfo{DeviceName: "sdb"}
	err := s.machine.SetMachineBlockDevices(sda, sdb)
	c.Assert(err, jc.ErrorIsNil)
	s.assertBlockDevices(c, s.machine.MachineTag(), []state.BlockDeviceInfo{sda, sdb})

	sdb.Label = "root"
	err = s.machine.SetMachineBlockDevices(sdb)
	c.Assert(err, jc.ErrorIsNil)
	s.assertBlockDevices(c, s.machine.MachineTag(), []state.BlockDeviceInfo{sdb})

	// If a device is attached, unattached, then attached again,
	// then it gets a new name.
	sdb.Label = "" // Label should be reset.
	sdb.FilesystemType = "ext4"
	err = s.machine.SetMachineBlockDevices(sda, sdb)
	c.Assert(err, jc.ErrorIsNil)
	s.assertBlockDevices(c, s.machine.MachineTag(), []state.BlockDeviceInfo{
		sda,
		sdb,
	})
}

func (s *BlockDevicesSuite) TestSetMachineBlockDevicesUnchanged(c *gc.C) {
	sda := state.BlockDeviceInfo{DeviceName: "sda"}
	err := s.machine.SetMachineBlockDevices(sda)
	c.Assert(err, jc.ErrorIsNil)
	s.assertBlockDevices(c, s.machine.MachineTag(), []state.BlockDeviceInfo{sda})

	// Setting the same should not change txn-revno.
	docID := state.DocID(s.State, s.machine.Id())
	before, err := state.TxnRevno(s.State, state.BlockDevicesC, docID)
	c.Assert(err, jc.ErrorIsNil)

	err = s.machine.SetMachineBlockDevices(sda)
	c.Assert(err, jc.ErrorIsNil)

	after, err := state.TxnRevno(s.State, state.BlockDevicesC, docID)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(after, gc.Equals, before)
}

func (s *BlockDevicesSuite) TestSetMachineBlockDevicesConcurrently(c *gc.C) {
	sdaInner := state.BlockDeviceInfo{DeviceName: "sda"}
	defer state.SetBeforeHooks(c, s.State, func() {
		err := s.machine.SetMachineBlockDevices(sdaInner)
		c.Assert(err, jc.ErrorIsNil)
	}).Check()

	sdaOuter := state.BlockDeviceInfo{
		DeviceName: "sda",
		Label:      "root",
	}
	err := s.machine.SetMachineBlockDevices(sdaOuter)
	c.Assert(err, jc.ErrorIsNil)

	// the outer call should wipe out the inner one's update.
	s.assertBlockDevices(c, s.machine.MachineTag(), []state.BlockDeviceInfo{
		sdaOuter,
	})
}

func (s *BlockDevicesSuite) TestSetMachineBlockDevicesEmpty(c *gc.C) {
	sda := state.BlockDeviceInfo{DeviceName: "sda"}
	err := s.machine.SetMachineBlockDevices(sda)
	c.Assert(err, jc.ErrorIsNil)
	s.assertBlockDevices(c, s.machine.MachineTag(), []state.BlockDeviceInfo{sda})

	err = s.machine.SetMachineBlockDevices()
	c.Assert(err, jc.ErrorIsNil)
	s.assertBlockDevices(c, s.machine.MachineTag(), []state.BlockDeviceInfo{})
}

func (s *BlockDevicesSuite) TestBlockDevicesMachineRemove(c *gc.C) {
	sda := state.BlockDeviceInfo{DeviceName: "sda"}
	err := s.machine.SetMachineBlockDevices(sda)
	c.Assert(err, jc.ErrorIsNil)

	err = s.machine.EnsureDead()
	c.Assert(err, jc.ErrorIsNil)
	err = s.machine.Remove()
	c.Assert(err, jc.ErrorIsNil)

	_, err = s.State.BlockDevices(s.machine.MachineTag())
	c.Assert(err, jc.Satisfies, errors.IsNotFound)
}

func (s *BlockDevicesSuite) TestWatchBlockDevices(c *gc.C) {
	sda := state.BlockDeviceInfo{DeviceName: "sda"}
	sdb := state.BlockDeviceInfo{DeviceName: "sdb"}
	sdc := state.BlockDeviceInfo{DeviceName: "sdc"}
	err := s.machine.SetMachineBlockDevices(sda, sdb, sdc)
	c.Assert(err, jc.ErrorIsNil)

	// Start block device watcher.
	w := s.State.WatchBlockDevices(s.machine.MachineTag())
	defer testing.AssertStop(c, w)
	wc := testing.NewNotifyWatcherC(c, s.State, w)
	wc.AssertOneChange()

	// Setting the same should not trigger the watcher.
	err = s.machine.SetMachineBlockDevices(sdc, sdb, sda)
	c.Assert(err, jc.ErrorIsNil)
	wc.AssertNoChange()

	// change sdb's label.
	sdb.Label = "fatty"
	err = s.machine.SetMachineBlockDevices(sda, sdb, sdc)
	c.Assert(err, jc.ErrorIsNil)
	wc.AssertOneChange()

	// change sda's label and sdb's UUID at once.
	sda.Label = "giggly"
	sdb.UUID = "4c062658-6225-4f4b-96f3-debf00b964b4"
	err = s.machine.SetMachineBlockDevices(sda, sdb, sdc)
	c.Assert(err, jc.ErrorIsNil)
	wc.AssertOneChange()

	// drop sdc.
	err = s.machine.SetMachineBlockDevices(sda, sdb)
	c.Assert(err, jc.ErrorIsNil)
	wc.AssertOneChange()

	// add sdc again: should get a new name.
	err = s.machine.SetMachineBlockDevices(sda, sdb, sdc)
	c.Assert(err, jc.ErrorIsNil)
	wc.AssertOneChange()
}
