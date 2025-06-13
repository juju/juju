// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"github.com/juju/tc"

	coremachine "github.com/juju/juju/core/machine"
	machineerrors "github.com/juju/juju/domain/machine/errors"
	"github.com/juju/juju/internal/errors"
)

func (s *stateSuite) TestIsMachineRebootRequiredNoMachine(c *tc.C) {
	// Setup: No machine with this uuid

	// Call the function under test
	isRebootNeeded, err := s.state.IsMachineRebootRequired(c.Context(), "u-u-i-d")
	c.Assert(err, tc.ErrorIsNil)

	// Verify: Check that no machine need reboot
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(isRebootNeeded, tc.IsFalse)
}

func (s *stateSuite) TestRequireMachineReboot(c *tc.C) {
	// Setup: Create a machine with a given ID
	err := s.state.CreateMachine(c.Context(), "", "", "u-u-i-d", nil)
	c.Assert(err, tc.ErrorIsNil)

	// Call the function under test
	err = s.state.RequireMachineReboot(c.Context(), "u-u-i-d")
	c.Assert(err, tc.ErrorIsNil)

	// Verify: Check if the machine needs reboot
	isRebootNeeded, err := s.state.IsMachineRebootRequired(c.Context(), "u-u-i-d")
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(isRebootNeeded, tc.IsTrue)
}

func (s *stateSuite) TestRequireMachineRebootIdempotent(c *tc.C) {
	// Setup: Create a machine with a given ID
	err := s.state.CreateMachine(c.Context(), "", "", "u-u-i-d", nil)
	c.Assert(err, tc.ErrorIsNil)

	// Call the function under test, twice (idempotency)
	err = s.state.RequireMachineReboot(c.Context(), "u-u-i-d")
	c.Assert(err, tc.ErrorIsNil)
	err = s.state.RequireMachineReboot(c.Context(), "u-u-i-d")
	c.Assert(err, tc.ErrorIsNil)

	// Verify: Check if the machine needs reboot
	isRebootNeeded, err := s.state.IsMachineRebootRequired(c.Context(), "u-u-i-d")
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(isRebootNeeded, tc.IsTrue)
}

func (s *stateSuite) TestRequireMachineRebootSeveralMachine(c *tc.C) {
	// Setup: Create several machine with a given IDs
	err := s.state.CreateMachine(c.Context(), "alive", "a-l-i-ve", "a-l-i-ve", nil)
	c.Assert(err, tc.ErrorIsNil)
	err = s.state.CreateMachine(c.Context(), "dead", "d-e-a-d", "d-e-a-d", nil)
	c.Assert(err, tc.ErrorIsNil)

	// Call the function under test
	err = s.state.RequireMachineReboot(c.Context(), "d-e-a-d")
	c.Assert(err, tc.ErrorIsNil)

	// Verify: Check which machine needs reboot
	isRebootNeeded, err := s.state.IsMachineRebootRequired(c.Context(), "a-l-i-ve")
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(isRebootNeeded, tc.IsFalse)
	isRebootNeeded, err = s.state.IsMachineRebootRequired(c.Context(), "d-e-a-d")
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(isRebootNeeded, tc.IsTrue)
}

func (s *stateSuite) TestCancelMachineReboot(c *tc.C) {
	// Setup: Create a machine with a given ID and add its ID to the reboot table.
	err := s.state.CreateMachine(c.Context(), "", "", "u-u-i-d", nil)
	c.Assert(err, tc.ErrorIsNil)
	err = s.runQuery(c, `INSERT INTO machine_requires_reboot (machine_uuid) VALUES ("u-u-i-d")`)
	c.Assert(err, tc.ErrorIsNil)

	// Call the function under test
	err = s.state.ClearMachineReboot(c.Context(), "u-u-i-d")
	c.Assert(err, tc.ErrorIsNil)

	// Verify: Check if the machine needs reboot
	isRebootNeeded, err := s.state.IsMachineRebootRequired(c.Context(), "u-u-i-d")
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(isRebootNeeded, tc.IsFalse)
}

func (s *stateSuite) TestCancelMachineRebootIdempotent(c *tc.C) {
	// Setup: Create a machine with a given ID  add its ID to the reboot table.
	err := s.state.CreateMachine(c.Context(), "", "", "u-u-i-d", nil)
	c.Assert(err, tc.ErrorIsNil)
	err = s.runQuery(c, `INSERT INTO machine_requires_reboot (machine_uuid) VALUES ("u-u-i-d")`)
	c.Assert(err, tc.ErrorIsNil)

	// Call the function under test, twice (idempotency)
	err = s.state.ClearMachineReboot(c.Context(), "u-u-i-d")
	c.Assert(err, tc.ErrorIsNil)
	err = s.state.ClearMachineReboot(c.Context(), "u-u-i-d")
	c.Assert(err, tc.ErrorIsNil)

	// Verify: Check if the machine needs reboot
	isRebootNeeded, err := s.state.IsMachineRebootRequired(c.Context(), "u-u-i-d")
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(isRebootNeeded, tc.IsFalse)
}

func (s *stateSuite) TestCancelMachineRebootSeveralMachine(c *tc.C) {
	// Setup: Create several machine with a given IDs,  add both ids in the reboot table
	err := s.state.CreateMachine(c.Context(), "alive", "a-l-i-ve", "a-l-i-ve", nil)
	c.Assert(err, tc.ErrorIsNil)
	err = s.state.CreateMachine(c.Context(), "dead", "d-e-a-d", "d-e-a-d", nil)
	c.Assert(err, tc.ErrorIsNil)
	err = s.runQuery(c, `INSERT INTO machine_requires_reboot (machine_uuid) VALUES ("a-l-i-ve")`)
	c.Assert(err, tc.ErrorIsNil)
	err = s.runQuery(c, `INSERT INTO machine_requires_reboot (machine_uuid) VALUES ("d-e-a-d")`)
	c.Assert(err, tc.ErrorIsNil)

	// Call the function under test
	err = s.state.ClearMachineReboot(c.Context(), "a-l-i-ve")
	c.Assert(err, tc.ErrorIsNil)

	// Verify: Check which machine needs reboot
	isRebootNeeded, err := s.state.IsMachineRebootRequired(c.Context(), "a-l-i-ve")
	c.Assert(err, tc.ErrorIsNil)
	c.Check(isRebootNeeded, tc.IsFalse)
	isRebootNeeded, err = s.state.IsMachineRebootRequired(c.Context(), "d-e-a-d")
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(isRebootNeeded, tc.IsTrue)
}

func (s *stateSuite) TestRebootNotOrphan(c *tc.C) {
	description := "orphan, non-rebooting machine, should do nothing"

	// Setup: machines and parent if any and setup reboot if required in test case
	err := s.state.CreateMachine(c.Context(), "machine", "machine", "machine", nil)
	c.Assert(err, tc.ErrorIsNil)

	// Call the function under test
	rebootAction, err := s.state.ShouldRebootOrShutdown(c.Context(), "machine")
	c.Assert(err, tc.ErrorIsNil, tc.Commentf("use case: %s", description))

	// Verify: Check which machine needs reboot
	c.Check(rebootAction, tc.Equals, coremachine.ShouldDoNothing, tc.Commentf("use case: %s", description))
}

func (s *stateSuite) TestRebootOrphan(c *tc.C) {
	description := "orphan, rebooting machine, should reboot"

	// Setup: machines and parent if any and setup reboot if required in test case
	err := s.state.CreateMachine(c.Context(), "machine", "machine", "machine", nil)
	c.Assert(err, tc.ErrorIsNil)
	err = s.runQuery(c, `INSERT INTO machine_requires_reboot (machine_uuid) VALUES ("machine")`)
	c.Assert(err, tc.ErrorIsNil)

	// Call the function under test
	rebootAction, err := s.state.ShouldRebootOrShutdown(c.Context(), "machine")
	c.Assert(err, tc.ErrorIsNil, tc.Commentf("use case: %s", description))

	// Verify: Check which machine needs reboot
	c.Check(rebootAction, tc.Equals, coremachine.ShouldReboot, tc.Commentf("use case: %s", description))
}

func (s *stateSuite) TestRebootNotParentChild(c *tc.C) {
	description := "non-rebooting machine with non-rebooting parent, should do nothing"

	// Setup: machines and parent if any and setup reboot if required in test case
	err := s.state.CreateMachine(c.Context(), "machine", "machine", "machine", nil)
	c.Assert(err, tc.ErrorIsNil)
	err = s.state.CreateMachine(c.Context(), "parent", "parent", "parent", nil)
	c.Assert(err, tc.ErrorIsNil)
	err = s.runQuery(c, `INSERT INTO machine_parent (machine_uuid, parent_uuid) VALUES ("machine", "parent")`)
	c.Assert(err, tc.ErrorIsNil)

	// Call the function under test
	rebootAction, err := s.state.ShouldRebootOrShutdown(c.Context(), "machine")
	c.Assert(err, tc.ErrorIsNil, tc.Commentf("use case: %s", description))

	// Verify: Check which machine needs reboot
	c.Check(rebootAction, tc.Equals, coremachine.ShouldDoNothing, tc.Commentf("use case: %s", description))
}

func (s *stateSuite) TestRebootChildNotParent(c *tc.C) {
	description := "rebooting machine with non-rebooting parent, should reboot"

	// Setup: machines and parent if any and setup reboot if required in test case
	err := s.state.CreateMachine(c.Context(), "machine", "machine", "machine", nil)
	c.Assert(err, tc.ErrorIsNil)
	err = s.runQuery(c, `INSERT INTO machine_requires_reboot (machine_uuid) VALUES ("machine")`)
	c.Assert(err, tc.ErrorIsNil)
	err = s.state.CreateMachine(c.Context(), "parent", "parent", "parent", nil)
	c.Assert(err, tc.ErrorIsNil)
	err = s.runQuery(c, `INSERT INTO machine_parent (machine_uuid, parent_uuid) VALUES ("machine", "parent")`)
	c.Assert(err, tc.ErrorIsNil)

	// Call the function under test
	rebootAction, err := s.state.ShouldRebootOrShutdown(c.Context(), "machine")
	c.Assert(err, tc.ErrorIsNil, tc.Commentf("use case: %s", description))

	// Verify: Check which machine needs reboot
	c.Check(rebootAction, tc.Equals, coremachine.ShouldReboot, tc.Commentf("use case: %s", description))
}

func (s *stateSuite) TestRebootParentNotChild(c *tc.C) {
	description := "non-rebooting machine with rebooting parent, should shutdown"

	// Setup: machines and parent if any and setup reboot if required in test case
	err := s.state.CreateMachine(c.Context(), "machine", "machine", "machine", nil)
	c.Assert(err, tc.ErrorIsNil)
	err = s.state.CreateMachine(c.Context(), "parent", "parent", "parent", nil)
	c.Assert(err, tc.ErrorIsNil)
	err = s.runQuery(c, `INSERT INTO machine_parent (machine_uuid, parent_uuid) VALUES ("machine", "parent")`)
	c.Assert(err, tc.ErrorIsNil)
	err = s.runQuery(c, `INSERT INTO machine_requires_reboot (machine_uuid) VALUES ("parent")`)
	c.Assert(err, tc.ErrorIsNil)

	// Call the function under test
	rebootAction, err := s.state.ShouldRebootOrShutdown(c.Context(), "machine")
	c.Assert(err, tc.ErrorIsNil, tc.Commentf("use case: %s", description))

	// Verify: Check which machine needs reboot
	c.Check(rebootAction, tc.Equals, coremachine.ShouldShutdown, tc.Commentf("use case: %s", description))
}

func (s *stateSuite) TestRebootParentChild(c *tc.C) {
	description := "rebooting machine with rebooting parent, should shutdown"

	// Setup: machines and parent if any and setup reboot if required in test case
	err := s.state.CreateMachine(c.Context(), "machine", "machine", "machine", nil)
	c.Assert(err, tc.ErrorIsNil)
	err = s.runQuery(c, `INSERT INTO machine_requires_reboot (machine_uuid) VALUES ("machine")`)
	c.Assert(err, tc.ErrorIsNil)
	err = s.state.CreateMachine(c.Context(), "parent", "parent", "parent", nil)
	c.Assert(err, tc.ErrorIsNil)
	err = s.runQuery(c, `INSERT INTO machine_parent (machine_uuid, parent_uuid) VALUES ("machine", "parent")`)
	c.Assert(err, tc.ErrorIsNil)
	err = s.runQuery(c, `INSERT INTO machine_requires_reboot (machine_uuid) VALUES ("parent")`)
	c.Assert(err, tc.ErrorIsNil)

	// Call the function under test
	rebootAction, err := s.state.ShouldRebootOrShutdown(c.Context(), "machine")
	c.Assert(err, tc.ErrorIsNil, tc.Commentf("use case: %s", description))

	// Verify: Check which machine needs reboot
	c.Check(rebootAction, tc.Equals, coremachine.ShouldShutdown, tc.Commentf("use case: %s", description))
}

func (s *stateSuite) TestRebootLogicGrandParentNotSupported(c *tc.C) {
	// Setup: Create a machine hierarchy
	err := s.state.CreateMachine(c.Context(), "machine", "machine", "machine", nil)
	c.Assert(err, tc.ErrorIsNil)
	err = s.state.CreateMachine(c.Context(), "parent", "parent", "parent", nil)
	c.Assert(err, tc.ErrorIsNil)
	err = s.state.CreateMachine(c.Context(), "grandparent", "grandparent", "grandparent", nil)
	c.Assert(err, tc.ErrorIsNil)
	err = s.runQuery(c, `INSERT INTO machine_parent (machine_uuid, parent_uuid) VALUES ("machine", "parent")`)
	c.Assert(err, tc.ErrorIsNil)
	err = s.runQuery(c, `INSERT INTO machine_parent (machine_uuid, parent_uuid) VALUES ("parent", "grandparent")`)
	c.Assert(err, tc.ErrorIsNil)

	// Call the function under test
	_, err = s.state.ShouldRebootOrShutdown(c.Context(), "machine")

	// Verify: grand parent are not supported
	c.Assert(errors.Is(err, machineerrors.GrandParentNotSupported), tc.Equals, true, tc.Commentf("obtained error: %v", err))
}
