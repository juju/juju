// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"context"

	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	coremachine "github.com/juju/juju/core/machine"
	machineerrors "github.com/juju/juju/domain/machine/errors"
	"github.com/juju/juju/internal/errors"
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
	err = s.state.ClearMachineReboot(context.Background(), "u-u-i-d")
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
	err = s.state.ClearMachineReboot(context.Background(), "u-u-i-d")
	c.Assert(err, jc.ErrorIsNil)
	err = s.state.ClearMachineReboot(context.Background(), "u-u-i-d")
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
	err = s.state.ClearMachineReboot(context.Background(), "a-l-i-ve")
	c.Assert(err, jc.ErrorIsNil)

	// Verify: Check which machine needs reboot
	isRebootNeeded, err := s.state.IsMachineRebootRequired(context.Background(), "a-l-i-ve")
	c.Assert(err, jc.ErrorIsNil)
	c.Check(isRebootNeeded, jc.IsFalse)
	isRebootNeeded, err = s.state.IsMachineRebootRequired(context.Background(), "d-e-a-d")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(isRebootNeeded, jc.IsTrue)
}

func (s *stateSuite) TestRebootLogic(c *gc.C) {
	for _, testCase := range []struct {
		description     string
		hasParent       bool
		isParentReboot  bool
		isMachineReboot bool
		expectedAction  coremachine.RebootAction
	}{
		{
			description:    "orphan, non-rebooting machine, should do nothing",
			expectedAction: coremachine.ShouldDoNothing,
		},
		{
			description:     "orphan, rebooting machine, should reboot",
			isMachineReboot: true,
			expectedAction:  coremachine.ShouldReboot,
		},
		{
			description:    "non-rebooting machine with non-rebooting parent, should do nothing",
			hasParent:      true,
			expectedAction: coremachine.ShouldDoNothing,
		},
		{
			description:     "rebooting machine with non-rebooting parent, should reboot",
			hasParent:       true,
			isMachineReboot: true,
			expectedAction:  coremachine.ShouldReboot,
		},
		{
			description:    "non-rebooting machine with rebooting parent, should shutdown",
			hasParent:      true,
			isParentReboot: true,
			expectedAction: coremachine.ShouldShutdown,
		},
		{
			description:     "rebooting machine with rebooting parent, should shutdown",
			hasParent:       true,
			isParentReboot:  true,
			isMachineReboot: true,
			expectedAction:  coremachine.ShouldShutdown,
		},
	} {
		s.SetUpTest(c) // reset db
		// Setup: machines and parent if any and setup reboot if required in test case
		err := s.state.CreateMachine(context.Background(), "machine", "machine", "machine")
		c.Assert(err, jc.ErrorIsNil)
		if testCase.isMachineReboot {
			err = s.runQuery(`INSERT INTO machine_requires_reboot (machine_uuid) VALUES ("machine")`)
			c.Assert(err, jc.ErrorIsNil)
		}
		if testCase.hasParent {
			err := s.state.CreateMachine(context.Background(), "parent", "parent", "parent")
			c.Assert(err, jc.ErrorIsNil)
			err = s.runQuery(`INSERT INTO machine_parent (machine_uuid, parent_uuid) VALUES ("machine", "parent")`)
			c.Assert(err, jc.ErrorIsNil)
			if testCase.isParentReboot {
				err = s.runQuery(`INSERT INTO machine_requires_reboot (machine_uuid) VALUES ("parent")`)
				c.Assert(err, jc.ErrorIsNil)
			}
		}

		// Call the function under test
		rebootAction, err := s.state.ShouldRebootOrShutdown(context.Background(), "machine")
		c.Assert(err, jc.ErrorIsNil, gc.Commentf("use case: %s", testCase.description))

		// Verify: Check which machine needs reboot
		c.Check(rebootAction, gc.Equals, testCase.expectedAction, gc.Commentf("use case: %s", testCase.description))

		s.TearDownTest(c)
	}
}

func (s *stateSuite) TestRebootLogicGrandParentNotSupported(c *gc.C) {
	// Setup: Create a machine hierarchy
	err := s.state.CreateMachine(context.Background(), "machine", "machine", "machine")
	c.Assert(err, jc.ErrorIsNil)
	err = s.state.CreateMachine(context.Background(), "parent", "parent", "parent")
	c.Assert(err, jc.ErrorIsNil)
	err = s.state.CreateMachine(context.Background(), "grandparent", "grandparent", "grandparent")
	c.Assert(err, jc.ErrorIsNil)
	err = s.runQuery(`INSERT INTO machine_parent (machine_uuid, parent_uuid) VALUES ("machine", "parent")`)
	c.Assert(err, jc.ErrorIsNil)
	err = s.runQuery(`INSERT INTO machine_parent (machine_uuid, parent_uuid) VALUES ("parent", "grandparent")`)
	c.Assert(err, jc.ErrorIsNil)

	// Call the function under test
	_, err = s.state.ShouldRebootOrShutdown(context.Background(), "machine")

	// Verify: grand parent are not supported
	c.Assert(errors.Is(err, machineerrors.GrandParentNotSupported), gc.Equals, true, gc.Commentf("obtained error: %v", err))
}
