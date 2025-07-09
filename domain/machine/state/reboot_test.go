// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"fmt"

	"github.com/juju/tc"

	coremachine "github.com/juju/juju/core/machine"
	"github.com/juju/juju/domain/deployment"
	domainmachine "github.com/juju/juju/domain/machine"
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
	// Setup: Create a machine.
	mUUID := s.createMachine(c)

	// Call the function under test
	err := s.state.RequireMachineReboot(c.Context(), mUUID)
	c.Assert(err, tc.ErrorIsNil)

	// Verify: Check if the machine needs reboot
	isRebootNeeded, err := s.state.IsMachineRebootRequired(c.Context(), mUUID)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(isRebootNeeded, tc.IsTrue)
}

func (s *stateSuite) TestRequireMachineRebootIdempotent(c *tc.C) {
	// Setup: Create a machine.
	mUUID := s.createMachine(c)

	// Call the function under test, twice (idempotency)
	err := s.state.RequireMachineReboot(c.Context(), mUUID)
	c.Assert(err, tc.ErrorIsNil)
	err = s.state.RequireMachineReboot(c.Context(), mUUID)
	c.Assert(err, tc.ErrorIsNil)

	// Verify: Check if the machine needs reboot
	isRebootNeeded, err := s.state.IsMachineRebootRequired(c.Context(), mUUID)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(isRebootNeeded, tc.IsTrue)
}

func (s *stateSuite) TestRequireMachineRebootSeveralMachine(c *tc.C) {
	// Setup: Create several machines.
	machineUUID0, _ := s.addMachine(c)
	machineUUID1, _ := s.addMachine(c)

	// Call the function under test
	err := s.state.RequireMachineReboot(c.Context(), machineUUID1)
	c.Assert(err, tc.ErrorIsNil)

	// Verify: Check which machine needs reboot
	isRebootNeeded, err := s.state.IsMachineRebootRequired(c.Context(), machineUUID0)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(isRebootNeeded, tc.IsFalse)
	isRebootNeeded, err = s.state.IsMachineRebootRequired(c.Context(), machineUUID1)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(isRebootNeeded, tc.IsTrue)
}

func (s *stateSuite) TestCancelMachineReboot(c *tc.C) {
	// Setup: Create a machine and add its ID to the reboot table.
	mUUID := s.createMachine(c)
	err := s.runQuery(c, fmt.Sprintf(`INSERT INTO machine_requires_reboot (machine_uuid) VALUES ("%s")`, mUUID))
	c.Assert(err, tc.ErrorIsNil)

	// Call the function under test
	err = s.state.ClearMachineReboot(c.Context(), mUUID)
	c.Assert(err, tc.ErrorIsNil)

	// Verify: Check if the machine needs reboot
	isRebootNeeded, err := s.state.IsMachineRebootRequired(c.Context(), mUUID)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(isRebootNeeded, tc.IsFalse)
}

func (s *stateSuite) TestCancelMachineRebootIdempotent(c *tc.C) {
	// Setup: Create a machine and add its ID to the reboot table.
	mUUID := s.createMachine(c)
	err := s.runQuery(c, fmt.Sprintf(`INSERT INTO machine_requires_reboot (machine_uuid) VALUES ("%s")`, mUUID))
	c.Assert(err, tc.ErrorIsNil)

	// Call the function under test, twice (idempotency)
	err = s.state.ClearMachineReboot(c.Context(), mUUID)
	c.Assert(err, tc.ErrorIsNil)
	err = s.state.ClearMachineReboot(c.Context(), mUUID)
	c.Assert(err, tc.ErrorIsNil)

	// Verify: Check if the machine needs reboot
	isRebootNeeded, err := s.state.IsMachineRebootRequired(c.Context(), mUUID)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(isRebootNeeded, tc.IsFalse)
}

func (s *stateSuite) TestCancelMachineRebootSeveralMachine(c *tc.C) {
	// Setup: Create several machine with a given IDs,  add both ids in the reboot table
	machineUUID0, _ := s.addMachine(c)
	err := s.runQuery(c, fmt.Sprintf(`INSERT INTO machine_requires_reboot (machine_uuid) VALUES ("%s")`, machineUUID0))
	c.Assert(err, tc.ErrorIsNil)
	machineUUID1, _ := s.addMachine(c)
	err = s.runQuery(c, fmt.Sprintf(`INSERT INTO machine_requires_reboot (machine_uuid) VALUES ("%s")`, machineUUID1))
	c.Assert(err, tc.ErrorIsNil)

	// Call the function under test
	err = s.state.ClearMachineReboot(c.Context(), machineUUID0)
	c.Assert(err, tc.ErrorIsNil)

	// Verify: Check which machine needs reboot
	isRebootNeeded, err := s.state.IsMachineRebootRequired(c.Context(), machineUUID0)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(isRebootNeeded, tc.IsFalse)
	isRebootNeeded, err = s.state.IsMachineRebootRequired(c.Context(), machineUUID1)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(isRebootNeeded, tc.IsTrue)
}

func (s *stateSuite) TestRebootNotOrphan(c *tc.C) {
	description := "orphan, non-rebooting machine, should do nothing"

	// Setup: machines and parent if any and setup reboot if required in test case
	mUUID := s.createMachine(c)

	// Call the function under test
	rebootAction, err := s.state.ShouldRebootOrShutdown(c.Context(), mUUID)
	c.Assert(err, tc.ErrorIsNil, tc.Commentf("use case: %s", description))

	// Verify: Check which machine needs reboot
	c.Check(rebootAction, tc.Equals, coremachine.ShouldDoNothing, tc.Commentf("use case: %s", description))
}

func (s *stateSuite) TestRebootOrphan(c *tc.C) {
	description := "orphan, rebooting machine, should reboot"

	// Setup: machines and parent if any and setup reboot if required in test case
	mUUID := s.createMachine(c)
	err := s.runQuery(c, fmt.Sprintf(`INSERT INTO machine_requires_reboot (machine_uuid) VALUES ("%s")`, mUUID))
	c.Assert(err, tc.ErrorIsNil)

	// Call the function under test
	rebootAction, err := s.state.ShouldRebootOrShutdown(c.Context(), mUUID)
	c.Assert(err, tc.ErrorIsNil, tc.Commentf("use case: %s", description))

	// Verify: Check which machine needs reboot
	c.Check(rebootAction, tc.Equals, coremachine.ShouldReboot, tc.Commentf("use case: %s", description))
}

func (s *stateSuite) TestRebootNotParentChild(c *tc.C) {
	description := "non-rebooting machine with non-rebooting parent, should do nothing"

	// Setup: machines and parent if any and setup reboot if required in test case
	_, childUUID := s.createContainer(c)

	// Call the function under test
	rebootAction, err := s.state.ShouldRebootOrShutdown(c.Context(), childUUID)
	c.Assert(err, tc.ErrorIsNil, tc.Commentf("use case: %s", description))

	// Verify: Check which machine needs reboot
	c.Check(rebootAction, tc.Equals, coremachine.ShouldDoNothing, tc.Commentf("use case: %s", description))
}

func (s *stateSuite) TestRebootChildNotParent(c *tc.C) {
	description := "rebooting machine with non-rebooting parent, should reboot"

	// Setup: machines and parent if any and setup reboot if required in test case
	_, childUUID := s.createContainer(c)

	err := s.runQuery(c, fmt.Sprintf(`INSERT INTO machine_requires_reboot (machine_uuid) VALUES ("%s")`, childUUID))
	c.Assert(err, tc.ErrorIsNil)

	// Call the function under test
	rebootAction, err := s.state.ShouldRebootOrShutdown(c.Context(), childUUID)
	c.Assert(err, tc.ErrorIsNil, tc.Commentf("use case: %s", description))

	// Verify: Check which machine needs reboot
	c.Check(rebootAction, tc.Equals, coremachine.ShouldReboot, tc.Commentf("use case: %s", description))
}

func (s *stateSuite) TestRebootParentNotChild(c *tc.C) {
	description := "non-rebooting machine with rebooting parent, should shutdown"

	// Setup: machines and parent if any and setup reboot if required in test case
	parentUUID, childUUID := s.createContainer(c)

	err := s.runQuery(c, fmt.Sprintf(`INSERT INTO machine_requires_reboot (machine_uuid) VALUES ("%s")`, parentUUID))
	c.Assert(err, tc.ErrorIsNil)

	// Call the function under test
	rebootAction, err := s.state.ShouldRebootOrShutdown(c.Context(), childUUID)
	c.Assert(err, tc.ErrorIsNil, tc.Commentf("use case: %s", description))

	// Verify: Check which machine needs reboot
	c.Check(rebootAction, tc.Equals, coremachine.ShouldShutdown, tc.Commentf("use case: %s", description))
}

func (s *stateSuite) TestRebootParentChild(c *tc.C) {
	description := "rebooting machine with rebooting parent, should shutdown"

	// Setup: machines and parent if any and setup reboot if required in test case
	parentUUID, childUUID := s.createContainer(c)

	err := s.runQuery(c, fmt.Sprintf(`INSERT INTO machine_requires_reboot (machine_uuid) VALUES ("%s")`, parentUUID))
	c.Assert(err, tc.ErrorIsNil)
	err = s.runQuery(c, fmt.Sprintf(`INSERT INTO machine_requires_reboot (machine_uuid) VALUES ("%s")`, childUUID))
	c.Assert(err, tc.ErrorIsNil)

	// Call the function under test
	rebootAction, err := s.state.ShouldRebootOrShutdown(c.Context(), childUUID)
	c.Assert(err, tc.ErrorIsNil, tc.Commentf("use case: %s", description))

	// Verify: Check which machine needs reboot
	c.Check(rebootAction, tc.Equals, coremachine.ShouldShutdown, tc.Commentf("use case: %s", description))
}

func (s *stateSuite) createMachine(c *tc.C) coremachine.UUID {
	_, mNames, err := s.state.AddMachine(c.Context(), domainmachine.AddMachineArgs{})
	c.Assert(err, tc.ErrorIsNil)
	uuid, err := s.state.GetMachineUUID(c.Context(), mNames[0])
	c.Assert(err, tc.ErrorIsNil)
	return uuid
}

func (s *stateSuite) createContainer(c *tc.C) (coremachine.UUID, coremachine.UUID) {
	_, mNames, err := s.state.AddMachine(c.Context(), domainmachine.AddMachineArgs{
		Directive: deployment.Placement{
			Type: deployment.PlacementTypeContainer,
		},
	})
	c.Assert(err, tc.ErrorIsNil)
	parentUUID, err := s.state.GetMachineUUID(c.Context(), mNames[0])
	c.Assert(err, tc.ErrorIsNil)
	childUUID, err := s.state.GetMachineUUID(c.Context(), mNames[1])
	c.Assert(err, tc.ErrorIsNil)
	return parentUUID, childUUID
}
