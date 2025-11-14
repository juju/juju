// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package provisionertask_test

import (
	"testing"

	"github.com/juju/errors"
	"github.com/juju/tc"

	"github.com/juju/juju/internal/provisionertask"
)

type MachineFSMSuite struct{}

func TestMachineFSMSuite(t *testing.T) {
	tc.Run(t, &MachineFSMSuite{})
}

func (s *MachineFSMSuite) TestNewMachineFSM(c *tc.C) {
	fsm := provisionertask.NewMachineFSM("42")
	c.Assert(fsm, tc.NotNil)
	c.Assert(fsm.ID, tc.Equals, "42")
	c.Assert(fsm.State, tc.Equals, provisionertask.StatePending)
}

func (s *MachineFSMSuite) TestIdempotentTransitions(c *tc.C) {
	testCases := []struct {
		state       provisionertask.MachineState
		description string
	}{
		{provisionertask.StatePending, "Pending to Pending"},
		{provisionertask.StateStarting, "Starting to Starting"},
		{provisionertask.StateStopping, "Stopping to Stopping"},
		{provisionertask.StateDead, "Dead to Dead"},
		{provisionertask.StateFailed, "Failed to Failed"},
	}

	for _, tt := range testCases {
		fsm := provisionertask.NewMachineFSM("test")
		fsm.State = tt.state
		err := fsm.TransitionTo(tt.state)
		c.Assert(err, tc.IsNil, tc.Commentf("Expected idempotent transition for %s", tt.description))
		c.Assert(fsm.State, tc.Equals, tt.state)
	}
}

func (s *MachineFSMSuite) TestAllowedTransitions(c *tc.C) {
	testCases := []struct {
		from        provisionertask.MachineState
		to          provisionertask.MachineState
		description string
	}{
		// From Pending.
		{provisionertask.StatePending, provisionertask.StateStarting, "Pending to Starting"},
		{provisionertask.StatePending, provisionertask.StateDead, "Pending to Dead"},

		// From Starting.
		{provisionertask.StateStarting, provisionertask.StateRunning, "Starting to Running"},
		{provisionertask.StateStarting, provisionertask.StateFailed, "Starting to Failed"},
		{provisionertask.StateStarting, provisionertask.StateCancellingStart, "Starting to CancellingStart"},

		// From CancellingStart.
		{provisionertask.StateCancellingStart, provisionertask.StateStopping, "CancellingStart to Stopping"},
		{provisionertask.StateCancellingStart, provisionertask.StateDead, "CancellingStart to Dead"},

		// From Running.
		{provisionertask.StateRunning, provisionertask.StateStopping, "Running to Stopping"},

		// From Stopping.
		{provisionertask.StateStopping, provisionertask.StateDead, "Stopping to Dead"},
		{provisionertask.StateStopping, provisionertask.StateFailed, "Stopping to Failed"},
	}

	for _, tt := range testCases {
		fsm := provisionertask.NewMachineFSM("test")
		fsm.State = tt.from
		err := fsm.TransitionTo(tt.to)
		c.Assert(err, tc.IsNil, tc.Commentf("Expected allowed transition for %s", tt.description))
		c.Assert(fsm.State, tc.Equals, tt.to, tc.Commentf("State should be %s after %s", tt.to, tt.description))
	}
}

func (s *MachineFSMSuite) TestIllegalTransitions(c *tc.C) {
	testCases := []struct {
		from        provisionertask.MachineState
		to          provisionertask.MachineState
		description string
	}{
		// From Pending - illegal transitions.
		{provisionertask.StatePending, provisionertask.StateRunning, "Pending to Running (should go through Starting)"},
		{provisionertask.StatePending, provisionertask.StateStopping, "Pending to Stopping"},
		{provisionertask.StatePending, provisionertask.StateFailed, "Pending to Failed"},

		// From Starting - illegal transitions.
		{provisionertask.StateStarting, provisionertask.StatePending, "Starting to Pending (backwards)"},
		{provisionertask.StateStarting, provisionertask.StateDead, "Starting to Dead (should go through CancellingStart/Stopping)"},

		// From CancellingStart - illegal transitions.
		{provisionertask.StateCancellingStart, provisionertask.StateStarting, "CancellingStart to Starting (backwards)"},
		{provisionertask.StateCancellingStart, provisionertask.StateRunning, "CancellingStart to Running"},
		{provisionertask.StateCancellingStart, provisionertask.StateFailed, "CancellingStart to Failed"},

		// From Running - illegal transitions.
		{provisionertask.StateRunning, provisionertask.StatePending, "Running to Pending (backwards)"},
		{provisionertask.StateRunning, provisionertask.StateStarting, "Running to Starting (backwards)"},
		{provisionertask.StateRunning, provisionertask.StateDead, "Running to Dead (should go through Stopping)"},
		{provisionertask.StateRunning, provisionertask.StateFailed, "Running to Failed (should go through Stopping)"},

		// From Stopping - illegal transitions.
		{provisionertask.StateStopping, provisionertask.StatePending, "Stopping to Pending"},
		{provisionertask.StateStopping, provisionertask.StateStarting, "Stopping to Starting"},
		{provisionertask.StateStopping, provisionertask.StateRunning, "Stopping to Running"},

		// From Dead - terminal state, only Dead is allowed (tested in
		// idempotent).
		{provisionertask.StateDead, provisionertask.StatePending, "Dead to Pending (terminal state)"},
		{provisionertask.StateDead, provisionertask.StateStarting, "Dead to Starting (terminal state)"},
		{provisionertask.StateDead, provisionertask.StateRunning, "Dead to Running (terminal state)"},

		// From Failed - terminal state, only Failed is allowed (tested in
		// idempotent).
		{provisionertask.StateFailed, provisionertask.StatePending, "Failed to Pending (terminal state)"},
		{provisionertask.StateFailed, provisionertask.StateStarting, "Failed to Starting (terminal state)"},
		{provisionertask.StateFailed, provisionertask.StateRunning, "Failed to Running (terminal state)"},
	}

	for _, tt := range testCases {
		fsm := provisionertask.NewMachineFSM("test")
		fsm.State = tt.from
		err := fsm.TransitionTo(tt.to)
		c.Assert(err, tc.NotNil, tc.Commentf("Expected error for illegal transition: %s", tt.description))
		c.Assert(errors.Is(err, provisionertask.ErrIllegalTransition), tc.IsTrue,
			tc.Commentf("Expected ErrIllegalTransition for %s, got: %v", tt.description, err))
		c.Assert(fsm.State, tc.Equals, tt.from, tc.Commentf("State should remain %s after failed transition", tt.from))
	}
}

func (s *MachineFSMSuite) TestTerminalStates(c *tc.C) {
	c.Assert(provisionertask.IsTerminal(provisionertask.StateDead), tc.IsTrue)
	c.Assert(provisionertask.IsTerminal(provisionertask.StateFailed), tc.IsTrue)

	// Non-terminal states.
	c.Assert(provisionertask.IsTerminal(provisionertask.StatePending), tc.IsFalse)
	c.Assert(provisionertask.IsTerminal(provisionertask.StateStarting), tc.IsFalse)
	c.Assert(provisionertask.IsTerminal(provisionertask.StateCancellingStart), tc.IsFalse)
	c.Assert(provisionertask.IsTerminal(provisionertask.StateRunning), tc.IsFalse)
	c.Assert(provisionertask.IsTerminal(provisionertask.StateStopping), tc.IsFalse)
}

func (s *MachineFSMSuite) TestFSMString(c *tc.C) {
	fsm := provisionertask.NewMachineFSM("42")
	c.Assert(fsm.String(), tc.Equals, "MachineFSM[42: Pending]")

	fsm.State = provisionertask.StateRunning
	c.Assert(fsm.String(), tc.Equals, "MachineFSM[42: Running]")
}

func (s *MachineFSMSuite) TestCompleteLifecycle(c *tc.C) {
	// Test a complete lifecycle:
	// Pending -> Starting -> Running -> Stopping -> Dead
	fsm := provisionertask.NewMachineFSM("lifecycle-test")

	// Start in Pending.
	c.Assert(fsm.State, tc.Equals, provisionertask.StatePending)

	// Transition to Starting.
	err := fsm.TransitionTo(provisionertask.StateStarting)
	c.Assert(err, tc.IsNil)
	c.Assert(fsm.State, tc.Equals, provisionertask.StateStarting)

	// Transition to Running.
	err = fsm.TransitionTo(provisionertask.StateRunning)
	c.Assert(err, tc.IsNil)
	c.Assert(fsm.State, tc.Equals, provisionertask.StateRunning)

	// Transition to Stopping.
	err = fsm.TransitionTo(provisionertask.StateStopping)
	c.Assert(err, tc.IsNil)
	c.Assert(fsm.State, tc.Equals, provisionertask.StateStopping)

	// Transition to Dead.
	err = fsm.TransitionTo(provisionertask.StateDead)
	c.Assert(err, tc.IsNil)
	c.Assert(fsm.State, tc.Equals, provisionertask.StateDead)
	c.Assert(provisionertask.IsTerminal(fsm.State), tc.IsTrue)
}

func (s *MachineFSMSuite) TestCancellationLifecycle(c *tc.C) {
	// Test cancellation during start:
	// Pending -> Starting -> CancellingStart -> Stopping -> Dead
	fsm := provisionertask.NewMachineFSM("cancellation-test")

	// Transition to Starting.
	err := fsm.TransitionTo(provisionertask.StateStarting)
	c.Assert(err, tc.IsNil)

	// Transition to CancellingStart.
	err = fsm.TransitionTo(provisionertask.StateCancellingStart)
	c.Assert(err, tc.IsNil)
	c.Assert(fsm.State, tc.Equals, provisionertask.StateCancellingStart)

	// Transition to Stopping.
	err = fsm.TransitionTo(provisionertask.StateStopping)
	c.Assert(err, tc.IsNil)

	// Transition to Dead.
	err = fsm.TransitionTo(provisionertask.StateDead)
	c.Assert(err, tc.IsNil)
	c.Assert(provisionertask.IsTerminal(fsm.State), tc.IsTrue)
}
