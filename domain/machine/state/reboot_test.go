// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"context"

	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
)

func (s *stateSuite) TestIsMachineRebootRequiredNoMachine(c *gc.C) {
	// Setup: No machine with this uuid

	// Call the function under test
	isRebootNeeded, err := s.state.IsMachineRebootRequired(context.Background(), "u-u-i-d")
	c.Assert(err, jc.ErrorIsNil)

	// Verify: Check that no machine need reboot
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(isRebootNeeded, jc.IsFalse)
}

func (s *stateSuite) TestRequireMachineReboot(c *gc.C) {
	// Setup: Create a machine with a given ID
	err := s.state.CreateMachine(context.Background(), "", "", "u-u-i-d")
	c.Assert(err, jc.ErrorIsNil)

	// Call the function under test
	err = s.state.RequireMachineReboot(context.Background(), "u-u-i-d")
	c.Assert(err, jc.ErrorIsNil)

	// Verify: Check if the machine needs reboot
	isRebootNeeded, err := s.state.IsMachineRebootRequired(context.Background(), "u-u-i-d")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(isRebootNeeded, jc.IsTrue)
}

func (s *stateSuite) TestRequireMachineRebootIdempotent(c *gc.C) {
	// Setup: Create a machine with a given ID
	err := s.state.CreateMachine(context.Background(), "", "", "u-u-i-d")
	c.Assert(err, jc.ErrorIsNil)

	// Call the function under test, twice (idempotency)
	err = s.state.RequireMachineReboot(context.Background(), "u-u-i-d")
	c.Assert(err, jc.ErrorIsNil)
	err = s.state.RequireMachineReboot(context.Background(), "u-u-i-d")
	c.Assert(err, jc.ErrorIsNil)

	// Verify: Check if the machine needs reboot
	isRebootNeeded, err := s.state.IsMachineRebootRequired(context.Background(), "u-u-i-d")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(isRebootNeeded, jc.IsTrue)
}

func (s *stateSuite) TestRequireMachineRebootSeveralMachine(c *gc.C) {
	// Setup: Create several machine with a given IDs
	err := s.state.CreateMachine(context.Background(), "alive", "a-l-i-ve", "a-l-i-ve")
	c.Assert(err, jc.ErrorIsNil)
	err = s.state.CreateMachine(context.Background(), "dead", "d-e-a-d", "d-e-a-d")
	c.Assert(err, jc.ErrorIsNil)

	// Call the function under test
	err = s.state.RequireMachineReboot(context.Background(), "d-e-a-d")
	c.Assert(err, jc.ErrorIsNil)

	// Verify: Check which machine needs reboot
	isRebootNeeded, err := s.state.IsMachineRebootRequired(context.Background(), "a-l-i-ve")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(isRebootNeeded, jc.IsFalse)
	isRebootNeeded, err = s.state.IsMachineRebootRequired(context.Background(), "d-e-a-d")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(isRebootNeeded, jc.IsTrue)
}

func (s *stateSuite) TestCancelMachineReboot(c *gc.C) {
	// Setup: Create a machine with a given ID and add its ID to the reboot table.
	err := s.state.CreateMachine(context.Background(), "", "", "u-u-i-d")
	c.Assert(err, jc.ErrorIsNil)
	err = s.runQuery(`INSERT INTO machine_requires_reboot (machine_uuid) VALUES ("u-u-i-d")`)
	c.Assert(err, jc.ErrorIsNil)

	// Call the function under test
	err = s.state.CancelMachineReboot(context.Background(), "u-u-i-d")
	c.Assert(err, jc.ErrorIsNil)

	// Verify: Check if the machine needs reboot
	isRebootNeeded, err := s.state.IsMachineRebootRequired(context.Background(), "u-u-i-d")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(isRebootNeeded, jc.IsFalse)
}

func (s *stateSuite) TestCancelMachineRebootIdempotent(c *gc.C) {
	// Setup: Create a machine with a given ID  add its ID to the reboot table.
	err := s.state.CreateMachine(context.Background(), "", "", "u-u-i-d")
	c.Assert(err, jc.ErrorIsNil)
	err = s.runQuery(`INSERT INTO machine_requires_reboot (machine_uuid) VALUES ("u-u-i-d")`)
	c.Assert(err, jc.ErrorIsNil)

	// Call the function under test, twice (idempotency)
	err = s.state.CancelMachineReboot(context.Background(), "u-u-i-d")
	c.Assert(err, jc.ErrorIsNil)
	err = s.state.CancelMachineReboot(context.Background(), "u-u-i-d")
	c.Assert(err, jc.ErrorIsNil)

	// Verify: Check if the machine needs reboot
	isRebootNeeded, err := s.state.IsMachineRebootRequired(context.Background(), "u-u-i-d")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(isRebootNeeded, jc.IsFalse)
}

func (s *stateSuite) TestCancelMachineRebootSeveralMachine(c *gc.C) {
	// Setup: Create several machine with a given IDs,  add both ids in the reboot table
	err := s.state.CreateMachine(context.Background(), "alive", "a-l-i-ve", "a-l-i-ve")
	c.Assert(err, jc.ErrorIsNil)
	err = s.state.CreateMachine(context.Background(), "dead", "d-e-a-d", "d-e-a-d")
	c.Assert(err, jc.ErrorIsNil)
	err = s.runQuery(`INSERT INTO machine_requires_reboot (machine_uuid) VALUES ("a-l-i-ve")`)
	c.Assert(err, jc.ErrorIsNil)
	err = s.runQuery(`INSERT INTO machine_requires_reboot (machine_uuid) VALUES ("d-e-a-d")`)
	c.Assert(err, jc.ErrorIsNil)

	// Call the function under test
	err = s.state.CancelMachineReboot(context.Background(), "a-l-i-ve")
	c.Assert(err, jc.ErrorIsNil)

	// Verify: Check which machine needs reboot
	isRebootNeeded, err := s.state.IsMachineRebootRequired(context.Background(), "a-l-i-ve")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(isRebootNeeded, jc.IsFalse)
	isRebootNeeded, err = s.state.IsMachineRebootRequired(context.Background(), "d-e-a-d")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(isRebootNeeded, jc.IsTrue)
}
