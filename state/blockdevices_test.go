// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state_test

import (
	"github.com/juju/errors"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
	"gopkg.in/mgo.v2/txn"

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

func (s *BlockDevicesSuite) assertBlockDevices(c *gc.C, m *state.Machine, expected map[string]state.BlockDeviceInfo) {
	devices, err := m.BlockDevices()
	c.Assert(err, jc.ErrorIsNil)
	info := make(map[string]state.BlockDeviceInfo)
	for _, dev := range devices {
		devInfo, err := dev.Info()
		if err != nil {
			c.Assert(err, jc.Satisfies, errors.IsNotProvisioned)
			devInfo = state.BlockDeviceInfo{}
		}
		info[dev.Name()] = devInfo
	}
	c.Assert(info, gc.DeepEquals, expected)
}

func (s *BlockDevicesSuite) TestSetMachineBlockDevices(c *gc.C) {
	sda := state.BlockDeviceInfo{DeviceName: "sda"}
	err := s.machine.SetMachineBlockDevices(sda)
	c.Assert(err, jc.ErrorIsNil)
	s.assertBlockDevices(c, s.machine, map[string]state.BlockDeviceInfo{"0": sda})
}

func (s *BlockDevicesSuite) TestSetMachineBlockDevicesReplaces(c *gc.C) {
	sda := state.BlockDeviceInfo{DeviceName: "sda"}
	err := s.machine.SetMachineBlockDevices(sda)
	c.Assert(err, jc.ErrorIsNil)

	sdb := state.BlockDeviceInfo{DeviceName: "sdb"}
	err = s.machine.SetMachineBlockDevices(sdb)
	c.Assert(err, jc.ErrorIsNil)
	s.assertBlockDevices(c, s.machine, map[string]state.BlockDeviceInfo{"1": sdb})
}

func (s *BlockDevicesSuite) TestSetMachineBlockDevicesUpdates(c *gc.C) {
	sda := state.BlockDeviceInfo{DeviceName: "sda"}
	sdb := state.BlockDeviceInfo{DeviceName: "sdb"}
	err := s.machine.SetMachineBlockDevices(sda, sdb)
	c.Assert(err, jc.ErrorIsNil)
	s.assertBlockDevices(c, s.machine, map[string]state.BlockDeviceInfo{"0": sda, "1": sdb})

	sdb.Label = "root"
	err = s.machine.SetMachineBlockDevices(sdb)
	c.Assert(err, jc.ErrorIsNil)
	s.assertBlockDevices(c, s.machine, map[string]state.BlockDeviceInfo{"1": sdb})

	// If a device is attached, unattached, then attached again,
	// then it gets a new name.
	sdb.Label = "" // Label should be reset.
	err = s.machine.SetMachineBlockDevices(sda, sdb)
	c.Assert(err, jc.ErrorIsNil)
	s.assertBlockDevices(c, s.machine, map[string]state.BlockDeviceInfo{
		"2": sda,
		"1": sdb,
	})
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

	// SetMachineBlockDevices will not remove concurrently added
	// block devices. This is fine in practice, because there is
	// a single worker responsible for populating machine block
	// devices.
	s.assertBlockDevices(c, s.machine, map[string]state.BlockDeviceInfo{
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
	c.Assert(err, jc.ErrorIsNil)
	s.assertBlockDevices(c, s.machine, map[string]state.BlockDeviceInfo{"0": sda})

	err = s.machine.SetMachineBlockDevices()
	c.Assert(err, jc.ErrorIsNil)
	s.assertBlockDevices(c, s.machine, map[string]state.BlockDeviceInfo{})
}

func (s *BlockDevicesSuite) TestSetMachineBlockDevicesLeavesUnprovisioned(c *gc.C) {
	m, err := s.State.AddOneMachine(state.MachineTemplate{
		Series: "quantal",
		Jobs:   []state.MachineJob{state.JobHostUnits},
		BlockDevices: []state.BlockDeviceParams{
			{Size: 123},
		},
	})
	c.Assert(err, jc.ErrorIsNil)

	sda := state.BlockDeviceInfo{DeviceName: "sda"}
	err = m.SetMachineBlockDevices(sda)
	c.Assert(err, jc.ErrorIsNil)
	s.assertBlockDevices(c, m, map[string]state.BlockDeviceInfo{
		"0": state.BlockDeviceInfo{}, // unprovisioned
		"1": sda,
	})
}

func (s *BlockDevicesSuite) TestSetMachineBlockDevicesUpdatesProvisioned(c *gc.C) {
	m, err := s.State.AddOneMachine(state.MachineTemplate{
		Series: "quantal",
		Jobs:   []state.MachineJob{state.JobHostUnits},
		BlockDevices: []state.BlockDeviceParams{
			{Size: 123},
		},
	})
	c.Assert(err, jc.ErrorIsNil)

	sda := state.BlockDeviceInfo{
		DeviceName: "sda", Size: 123,
	}
	err = state.SetProvisionedBlockDeviceInfo(s.State, m.Id(), map[string]state.BlockDeviceInfo{
		"0": sda,
	})
	c.Assert(err, jc.ErrorIsNil)

	sda.UUID = "feedface"
	err = m.SetMachineBlockDevices(sda)
	c.Assert(err, jc.ErrorIsNil)
	s.assertBlockDevices(c, m, map[string]state.BlockDeviceInfo{
		"0": sda,
	})

	devices, err := m.BlockDevices()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(devices[0].Attached(), jc.IsTrue)
}

func (s *BlockDevicesSuite) TestBlockDevicesMachineRemove(c *gc.C) {
	sda := state.BlockDeviceInfo{DeviceName: "sda"}
	err := s.machine.SetMachineBlockDevices(sda)
	c.Assert(err, jc.ErrorIsNil)

	err = s.machine.EnsureDead()
	c.Assert(err, jc.ErrorIsNil)
	err = s.machine.Remove()
	c.Assert(err, jc.ErrorIsNil)

	s.assertBlockDevices(c, s.machine, map[string]state.BlockDeviceInfo{})
}

func (s *BlockDevicesSuite) TestBlockDeviceInfo(c *gc.C) {
	sdaInfo := state.BlockDeviceInfo{DeviceName: "sda"}
	err := state.RunTransaction(s.State, []txn.Op{{
		C:      state.BlockDevicesC,
		Id:     "0",
		Insert: &state.BlockDeviceDoc{Name: "0"},
	}, {
		C:      state.BlockDevicesC,
		Id:     "1",
		Insert: &state.BlockDeviceDoc{Name: "1", Info: &sdaInfo},
	}})
	c.Assert(err, jc.ErrorIsNil)

	b0, err := s.State.BlockDevice("0")
	c.Assert(err, jc.ErrorIsNil)
	_, err = b0.Info()
	c.Assert(err, jc.Satisfies, errors.IsNotProvisioned)
	b1, err := s.State.BlockDevice("1")
	c.Assert(err, jc.ErrorIsNil)
	info, err := b1.Info()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(info, gc.DeepEquals, sdaInfo)
}

func (s *BlockDevicesSuite) TestBlockDeviceParamsMethod(c *gc.C) {
	disk1Params := state.BlockDeviceParams{Size: 123}
	err := state.RunTransaction(s.State, []txn.Op{{
		C:      state.BlockDevicesC,
		Id:     "0",
		Insert: &state.BlockDeviceDoc{Name: "0"},
	}, {
		C:      state.BlockDevicesC,
		Id:     "1",
		Insert: &state.BlockDeviceDoc{Name: "1", Params: &disk1Params},
	}})
	c.Assert(err, jc.ErrorIsNil)

	b0, err := s.State.BlockDevice("0")
	c.Assert(err, jc.ErrorIsNil)
	_, ok := b0.Params()
	c.Assert(ok, jc.IsFalse)
	b1, err := s.State.BlockDevice("1")
	c.Assert(err, jc.ErrorIsNil)
	params, ok := b1.Params()
	c.Assert(ok, jc.IsTrue)
	c.Assert(params, gc.DeepEquals, disk1Params)
}

func (s *BlockDevicesSuite) TestMachineWatchBlockDevices(c *gc.C) {
	sda := state.BlockDeviceInfo{DeviceName: "sda"}
	sdb := state.BlockDeviceInfo{DeviceName: "sdb"}
	sdc := state.BlockDeviceInfo{DeviceName: "sdc"}
	err := s.machine.SetMachineBlockDevices(sda, sdb, sdc)
	c.Assert(err, jc.ErrorIsNil)

	// Start block device watcher.
	w := s.machine.WatchBlockDevices()
	defer testing.AssertStop(c, w)
	wc := testing.NewStringsWatcherC(c, s.State, w)
	assertOneChange := func(names ...string) {
		wc.AssertChangeInSingleEvent(names...)
		wc.AssertNoChange()
	}
	assertOneChange("0", "1", "2")

	// Setting the same should not trigger the watcher.
	err = s.machine.SetMachineBlockDevices(sdc, sdb, sda)
	c.Assert(err, jc.ErrorIsNil)
	wc.AssertNoChange()

	// change sdb's label.
	sdb.Label = "fatty"
	err = s.machine.SetMachineBlockDevices(sda, sdb, sdc)
	c.Assert(err, jc.ErrorIsNil)
	assertOneChange("1")

	// change sda's label and sdb's UUID at once.
	sda.Label = "giggly"
	sdb.UUID = "4c062658-6225-4f4b-96f3-debf00b964b4"
	err = s.machine.SetMachineBlockDevices(sda, sdb, sdc)
	c.Assert(err, jc.ErrorIsNil)
	assertOneChange("0", "1")

	// drop sdc.
	err = s.machine.SetMachineBlockDevices(sda, sdb)
	c.Assert(err, jc.ErrorIsNil)
	assertOneChange("2")

	// add sdc again: should get a new name.
	err = s.machine.SetMachineBlockDevices(sda, sdb, sdc)
	c.Assert(err, jc.ErrorIsNil)
	assertOneChange("3")
}
