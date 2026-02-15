// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package provisionertask

import (
	"fmt"
)

// MachineState represents the FSM state of a machine worker.
type MachineState string

const (
	// StatePending is the initial state. Machine exists but has no instance.
	// Worker is ready to begin provisioning.
	StatePending MachineState = "pending"

	// StateRequestingZone indicates the worker has requested an availability zone
	// from the AZ Coordinator and is waiting for the response.
	StateRequestingZone MachineState = "requesting zone"

	// StateProvisioning indicates the worker has acquired a semaphore slot and is
	// executing StartInstance followed by SetInstanceInfo. Both calls happen
	// within this single state.
	StateProvisioning MachineState = "provisioning"

	// StateRunning indicates the instance is created and registered. Worker idles,
	// waiting for the machine to die.
	StateRunning MachineState = "running"

	// StateStopping indicates the worker is executing StopInstances to terminate
	// the instance.
	StateStopping MachineState = "stopping"

	// StateRemoving indicates the worker is removing the machine record from state.
	StateRemoving MachineState = "removing"

	// StateComplete is a terminal state. Worker exits after successfully
	// completing all operations (machine removed).
	StateComplete MachineState = "complete"

	// StateError is a terminal state. Worker exits after encountering
	// unrecoverable errors (e.g., retries exhausted).
	StateError MachineState = "error"
)

// String returns a human-readable representation of the state.
func (s MachineState) String() string {
	return string(s)
}

// IsTerminal returns true if the state is a terminal state (Complete or Error).
func (s MachineState) IsTerminal() bool {
	return s == StateComplete || s == StateError
}

// validTransitions defines all valid state transitions in the FSM.
// The map key is the source state, and the value is a set of valid target states.
var validTransitions = map[MachineState]map[MachineState]struct{}{
	StatePending: {
		StateRequestingZone: {}, // Start provisioning.
		StateRunning:        {}, // Already has instance.
		StateRemoving:       {}, // Machine is dead (no instance).
	},
	StateRequestingZone: {
		StateProvisioning: {}, // Zone assigned.
		StatePending:      {}, // Zone request failed or machine dying, will retry.
		StateRemoving:     {}, // Machine died while requesting zone.
		StateError:        {}, // Retries exhausted.
	},
	StateProvisioning: {
		StateRunning: {}, // Provisioning succeeded.
		StatePending: {}, // Provisioning failed, will retry on next life event.
		StateError:   {}, // Retries exhausted.
	},
	StateRunning: {
		StateStopping: {}, // Machine died, stopping instance.
		StateRemoving: {}, // Machine died with keep-instance=true.
	},
	StateStopping: {
		StateRemoving: {}, // Instance stopped successfully.
	},
	StateRemoving: {
		StateComplete: {}, // Machine record removed successfully.
	},
	StateComplete: {
		// Terminal state - no valid transitions.
	},
	StateError: {
		// Terminal state - no valid transitions.
	},
}

// CanTransitionTo returns true if transitioning from the current state to
// the target state is valid according to the FSM rules.
func (s MachineState) CanTransitionTo(target MachineState) bool {
	targets, ok := validTransitions[s]
	if !ok {
		return false
	}
	_, ok = targets[target]
	return ok
}

// ValidTargets returns the list of valid target states from the current state.
func (s MachineState) ValidTargets() []MachineState {
	targets, ok := validTransitions[s]
	if !ok {
		return nil
	}
	result := make([]MachineState, 0, len(targets))
	for target := range targets {
		result = append(result, target)
	}
	return result
}

// FSM encapsulates the finite state machine logic for a machine worker.
// It tracks the current state and enforces valid transitions.
type FSM struct {
	state MachineState
}

// NewFSM creates a new FSM starting in StatePending.
func NewFSM() *FSM {
	return &FSM{state: StatePending}
}

// State returns the current state.
func (f *FSM) State() MachineState {
	return f.state
}

// TransitionTo attempts to transition to the target state.
// Returns an error if the transition is invalid.
func (f *FSM) TransitionTo(target MachineState) error {
	if !f.state.CanTransitionTo(target) {
		return fmt.Errorf("invalid state transition from %s to %s (valid targets: %v)",
			f.state, target, f.state.ValidTargets())
	}
	f.state = target
	return nil
}

// IsTerminal returns true if the FSM is in a terminal state.
func (f *FSM) IsTerminal() bool {
	return f.state.IsTerminal()
}
