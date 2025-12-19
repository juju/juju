// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package provisionertaskbis

import (
	"fmt"
)

// State represents the FSM state of a machine worker.
type State int

const (
	// StatePending is the initial state. Machine exists but has no instance.
	// Worker is ready to begin provisioning.
	StatePending State = iota

	// StateRequestingZone indicates the worker has requested an availability zone
	// from the AZ Coordinator and is waiting for the response.
	StateRequestingZone

	// StateProvisioning indicates the worker has acquired a semaphore slot and is
	// executing StartInstance followed by SetInstanceInfo. Both calls happen
	// within this single state.
	StateProvisioning

	// StateRunning indicates the instance is created and registered. Worker idles,
	// waiting for the machine to die.
	StateRunning

	// StateStopping indicates the worker is executing StopInstances to terminate
	// the instance.
	StateStopping

	// StateRemoving indicates the worker is removing the machine record from state.
	StateRemoving

	// StateDone is the terminal state. Worker exits cleanly.
	StateDone
)

// String returns a human-readable representation of the state.
func (s State) String() string {
	switch s {
	case StatePending:
		return "Pending"
	case StateRequestingZone:
		return "RequestingZone"
	case StateProvisioning:
		return "Provisioning"
	case StateRunning:
		return "Running"
	case StateStopping:
		return "Stopping"
	case StateRemoving:
		return "Removing"
	case StateDone:
		return "Done"
	default:
		return "Unknown"
	}
}

// IsTerminal returns true if the state is a terminal state (Done).
func (s State) IsTerminal() bool {
	return s == StateDone
}

// validTransitions defines all valid state transitions in the FSM.
// The map key is the source state, and the value is a set of valid target states.
var validTransitions = map[State]map[State]bool{
	StatePending: {
		StateRequestingZone: true, // Start provisioning.
		StateRunning:        true, // Already has instance.
		StateRemoving:       true, // Machine is dead (no instance).
	},
	StateRequestingZone: {
		StateProvisioning: true, // Zone assigned.
		StatePending:      true, // Zone request failed, will retry.
		StateRemoving:     true, // Machine died while requesting zone.
		StateDone:         true, // Retries exhausted.
	},
	StateProvisioning: {
		StateRunning: true, // Provisioning succeeded.
		StatePending: true, // Provisioning failed, will retry on next life event.
		StateDone:    true, // Retries exhausted.
	},
	StateRunning: {
		StateStopping: true, // Machine died, stopping instance.
		StateRemoving: true, // Machine died with keep-instance=true.
	},
	StateStopping: {
		StateRemoving: true, // Instance stopped successfully.
	},
	StateRemoving: {
		StateDone: true, // Machine record removed.
	},
	StateDone: {
		// Terminal state - no valid transitions.
	},
}

// CanTransitionTo returns true if transitioning from the current state to
// the target state is valid according to the FSM rules.
func (s State) CanTransitionTo(target State) bool {
	targets, ok := validTransitions[s]
	if !ok {
		return false
	}
	return targets[target]
}

// ValidTargets returns the list of valid target states from the current state.
func (s State) ValidTargets() []State {
	targets, ok := validTransitions[s]
	if !ok {
		return nil
	}
	result := make([]State, 0, len(targets))
	for target := range targets {
		result = append(result, target)
	}
	return result
}

// FSM encapsulates the finite state machine logic for a machine worker.
// It tracks the current state and enforces valid transitions.
type FSM struct {
	state State
}

// NewFSM creates a new FSM starting in StatePending.
func NewFSM() *FSM {
	return &FSM{state: StatePending}
}

// State returns the current state.
func (f *FSM) State() State {
	return f.state
}

// TransitionTo attempts to transition to the target state.
// Returns an error if the transition is invalid.
func (f *FSM) TransitionTo(target State) error {
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
