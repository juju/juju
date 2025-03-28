// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package upgrade

import "github.com/juju/juju/internal/errors"

var (
	ErrAlreadyAtState     = errors.ConstError("already at state")
	ErrUnableToTransition = errors.ConstError("unable to transition")
)

// State represents the state of the upgrade.
type State int

const (
	// Created is the initial state of an upgrade. All upgrades should start
	// in this initial state.
	Created State = iota
	// Started is the state of an upgrade after it has been started.
	Started
	// DBCompleted is the state of an upgrade after the database has been
	// upgraded.
	DBCompleted
	// StepsCompleted is the state of an upgrade after all the steps have
	// been completed.
	StepsCompleted
	// Error is the state of an upgrade after an error has occurred.
	Error
)

// States holds all the possible states.
var States = map[State]string{
	Created:        "created",
	Started:        "started",
	DBCompleted:    "db-completed",
	StepsCompleted: "steps-completed",
	Error:          "error",
}

// ParseState returns the state from a string.
func ParseState(str string) (State, error) {
	for state, s := range States {
		if s == str {
			return state, nil
		}
	}
	return 0, errors.Errorf("unknown state %q", str)
}

// TransitionTo returns an error if the transition is not allowed.
func (s State) TransitionTo(target State) error {
	if target == s {
		return ErrAlreadyAtState
	}

	// We always allow transitions to the Error state.
	if target == Error {
		return nil
	}

	switch s {
	case Created:
		if target == Started {
			return nil
		}
	case Started:
		if target == DBCompleted {
			return nil
		}
	case DBCompleted:
		if target == StepsCompleted {
			return nil
		}
	}
	return errors.Errorf("going from %q to %q: %w", s, target, ErrUnableToTransition)
}

// IsTerminal returns true if the state is terminal.
// A terminal state is one that cannot be transitioned from.
func (s State) IsTerminal() bool {
	return s == StepsCompleted || s == Error
}

func (s State) String() string {
	if str, ok := States[s]; ok {
		return str
	}
	return "unknown"
}

// Info holds the information about database upgrade
type Info struct {
	// UUID holds the upgrader's ID.
	UUID string
	// PreviousVersion holds the previous version.
	PreviousVersion string
	// TargetVersion holds the target version.
	TargetVersion string
	// State holds the current state of the upgrade.
	State State
}
