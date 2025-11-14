// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package provisionertask

import (
	"fmt"

	"github.com/juju/collections/set"
	"github.com/juju/errors"
)

// MachineState represents the state of a machine in the FSM.
type MachineState string

const (
	// StatePending indicates the machine has been created but not yet started.
	StatePending MachineState = "Pending"

	// StateStarting indicates the machine is in the process of being started.
	StateStarting MachineState = "Starting"

	// StateCancellingStart indicates a request to stop the machine was received
	// while it was still starting. The machine will transition to Stopping once
	// the start operation completes.
	StateCancellingStart MachineState = "CancellingStart"

	// StateRunning indicates the machine has been successfully started and has
	// an instance ID assigned.
	StateRunning MachineState = "Running"

	// StateStopping indicates the machine is in the process of being stopped.
	StateStopping MachineState = "Stopping"

	// StateDead indicates the machine has been stopped and removed. This is a
	// terminal state.
	StateDead MachineState = "Dead"

	// StateFailed indicates the machine encountered an error during
	// provisioning or operation. This is a terminal state.
	StateFailed MachineState = "Failed"
)

// ErrIllegalTransition is returned when an illegal state transition is
// attempted.
var ErrIllegalTransition = errors.ConstError("illegal state transition")

// allowedTransitions defines the valid state transitions in the FSM.
// Each state maps to a set of states it can transition to.
//
// These transitions follow the following (mermaid) diagram:
//
// stateDiagram-v2
//
//	[*] --> Pending
//	Pending --> Starting: machineChange(Alive & no InstanceId)
//	Pending --> Dead: machineChange(Dead|Dying & no InstanceId)
//	Starting --> Running: StartSuccess (instance created + info set)
//	Starting --> Starting: StartTransientErr / backoff (retry loop internal to task today)
//	Starting --> Failed: StartPermanentErr
//	Starting --> CancellingStart: machineChange(Dead|Dying while in-flight)
//	CancellingStart --> Stopping: StartSuccess (instance now exists → compensate)
//	CancellingStart --> Dead: StartErr(any) (no instance to stop)
//	Running --> Stopping: machineChange(Dead|Dying|Remove)
//	Stopping --> Dead: StopSuccess (MarkForRemoval done)
//	Stopping --> Stopping: StopTransientErr / backoff
//	Stopping --> Failed: StopPermanentErr
//	Dead --> [*]
//	Failed --> [*]
var allowedTransitions = map[MachineState]set.Strings{
	StatePending: set.NewStrings(
		string(StateStarting),
		string(StateDead),
	),
	StateStarting: set.NewStrings(
		string(StateRunning),
		string(StateFailed),
		string(StateCancellingStart),
		string(StateStarting), // idempotent for transient errors.
	),
	StateCancellingStart: set.NewStrings(
		string(StateStopping),
		string(StateDead),
	),
	StateRunning: set.NewStrings(
		string(StateStopping),
	),
	StateStopping: set.NewStrings(
		string(StateDead),
		string(StateFailed),
		string(StateStopping), // idempotent for transient errors.
	),
	StateDead: set.NewStrings(
		string(StateDead), // terminal, idempotent.
	),
	StateFailed: set.NewStrings(
		string(StateFailed), // terminal, idempotent.
	),
}

// MachineFSM represents a finite state machine for a single machine.
// It tracks the machine's current state and enforces valid state transitions.
type MachineFSM struct {
	// ID is the machine's unique identifier.
	ID string

	// State is the current state of the machine in the FSM.
	State MachineState
}

// NewMachineFSM creates a new MachineFSM with the given ID.
// The FSM is initialized in the Pending state.
func NewMachineFSM(id string) *MachineFSM {
	return &MachineFSM{
		ID:    id,
		State: StatePending,
	}
}

// TransitionTo attempts to transition the FSM to the target state.
// It returns an error if the transition is not allowed.
//
// If the target state is the same as the current state, this is a no-op
// and returns nil (idempotent transitions are allowed for terminal and
// transient states).
func (f *MachineFSM) TransitionTo(target MachineState) error {
	if f.State == target {
		return nil
	}

	// Check if transition is allowed.
	allowed, exists := allowedTransitions[f.State]
	if !exists {
		return errors.Annotatef(ErrIllegalTransition,
			"machine %s: unknown current state %q", f.ID, f.State)
	}

	if !allowed.Contains(string(target)) {
		return errors.Annotatef(ErrIllegalTransition,
			"machine %s: cannot transition from %q to %q", f.ID, f.State, target)
	}

	f.State = target
	return nil
}

// IsTerminal returns true if the given state is a terminal state.
// Terminal states are Dead and Failed, which represent the end of a
// machine's lifecycle in the FSM.
func IsTerminal(state MachineState) bool {
	return state == StateDead || state == StateFailed
}

// String returns a string representation of the FSM.
func (f *MachineFSM) String() string {
	return fmt.Sprintf("MachineFSM[%s: %s]", f.ID, f.State)
}
