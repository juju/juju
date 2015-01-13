// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state_test

import (
	"github.com/juju/errors"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
	"gopkg.in/mgo.v2/txn"

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
	c.Assert(err, jc.ErrorIsNil)
}

func (s *BlockDevicesSuite) assertBlockDevices(c *gc.C, m *state.Machine, expected map[string]state.BlockDeviceInfo) {
	devices, err := m.BlockDevices()
	c.Assert(err, gc.IsNil)
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
	c.Assert(err, gc.IsNil)
	s.assertBlockDevices(c, s.machine, map[string]state.BlockDeviceInfo{"0": sda})
}

func (s *BlockDevicesSuite) TestSetMachineBlockDevicesReplaces(c *gc.C) {
	sda := state.BlockDeviceInfo{DeviceName: "sda"}
	err := s.machine.SetMachineBlockDevices(sda)
	c.Assert(err, gc.IsNil)

	sdb := state.BlockDeviceInfo{DeviceName: "sdb"}
	err = s.machine.SetMachineBlockDevices(sdb)
	c.Assert(err, gc.IsNil)
	s.assertBlockDevices(c, s.machine, map[string]state.BlockDeviceInfo{"1": sdb})
}

func (s *BlockDevicesSuite) TestSetMachineBlockDevicesUpdates(c *gc.C) {
	sda := state.BlockDeviceInfo{DeviceName: "sda"}
	sdb := state.BlockDeviceInfo{DeviceName: "sdb"}
	err := s.machine.SetMachineBlockDevices(sda, sdb)
	c.Assert(err, gc.IsNil)
	s.assertBlockDevices(c, s.machine, map[string]state.BlockDeviceInfo{"0": sda, "1": sdb})

	sdb.Label = "root"
	err = s.machine.SetMachineBlockDevices(sdb)
	c.Assert(err, gc.IsNil)
	s.assertBlockDevices(c, s.machine, map[string]state.BlockDeviceInfo{"1": sdb})

	// If a device is attached, unattached, then attached again,
	// then it gets a new name.
	sdb.Label = "" // Label should be reset.
	err = s.machine.SetMachineBlockDevices(sda, sdb)
	c.Assert(err, gc.IsNil)
	s.assertBlockDevices(c, s.machine, map[string]state.BlockDeviceInfo{
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
	c.Assert(err, gc.IsNil)
	s.assertBlockDevices(c, s.machine, map[string]state.BlockDeviceInfo{"0": sda})

	err = s.machine.SetMachineBlockDevices()
	c.Assert(err, gc.IsNil)
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
	c.Assert(err, gc.IsNil)
	s.assertBlockDevices(c, m, map[string]state.BlockDeviceInfo{
		"0": state.BlockDeviceInfo{}, // unprovisioned
		"1": sda,
	})
}

func (s *BlockDevicesSuite) TestBlockDevicesMachineRemove(c *gc.C) {
	sda := state.BlockDeviceInfo{DeviceName: "sda"}
	err := s.machine.SetMachineBlockDevices(sda)
	c.Assert(err, gc.IsNil)

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

func (s *BlockDevicesSuite) TestBlockDeviceParams(c *gc.C) {
	test := func(cons storage.Constraints, expect []state.BlockDeviceParams) {
		params, err := s.State.BlockDeviceParams(cons, nil, "")
		c.Assert(err, jc.ErrorIsNil)
		c.Assert(params, gc.DeepEquals, expect)
	}
	test(storage.Constraints{Size: 1024, Count: 1}, []state.BlockDeviceParams{
		{Size: 1024},
	})
	test(storage.Constraints{Size: 2048, Count: 2}, []state.BlockDeviceParams{
		{Size: 2048}, {Size: 2048},
	})
}

func (s *BlockDevicesSuite) TestBlockDeviceParamsErrors(c *gc.C) {
	test := func(cons storage.Constraints, ch *state.Charm, store string, expectErr string) {
		_, err := s.State.BlockDeviceParams(cons, ch, store)
		c.Assert(err, gc.ErrorMatches, expectErr)
	}
	wordpress := s.AddTestingCharm(c, "wordpress")
	test(storage.Constraints{Count: 1}, nil, "", "invalid size 0")
	test(storage.Constraints{Size: 1}, nil, "", "invalid count 0")
	test(storage.Constraints{Size: 1, Count: 1}, wordpress, "", "charm storage metadata not implemented")
	test(storage.Constraints{Size: 1, Count: 1}, nil, "shared-fs", "charm storage metadata not implemented")
	test(storage.Constraints{Size: 1, Count: 1, Pool: "bingo"}, nil, "", "storage pools not implemented")
}
