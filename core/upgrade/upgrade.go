// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package upgrade

import "github.com/juju/errors"

var ErrAlreadyAtState = errors.ConstError("already at state")

// State represents the state of the upgrade.
type State int

const (
	Created State = iota
	Started
	DBCompleted
	StepsCompleted
)

// ParseState returns the state from a string.
func ParseState(str string) (State, error) {
	switch str {
	case "created":
		return Created, nil
	case "started":
		return Started, nil
	case "db-completed":
		return DBCompleted, nil
	case "steps-completed":
		return StepsCompleted, nil
	default:
		return 0, errors.Errorf("unknown state %q", str)
	}
}

// TransitionTo returns an error if the transition is not allowed.
func (s State) TransitionTo(target State) error {
	if target == s {
		return ErrAlreadyAtState
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
	return errors.Errorf("cannot transition from %q to %q", s, target)
}

// IsTerminal returns true if the state is terminal.
func (s State) IsTerminal() bool {
	return s == StepsCompleted
}

func (s State) String() string {
	switch s {
	case Created:
		return "created"
	case Started:
		return "started"
	case DBCompleted:
		return "db-completed"
	case StepsCompleted:
		return "steps-completed"
	default:
		return "unknown"
	}
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
