// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package resource

import "github.com/juju/juju/internal/errors"

// These are the valid resource states.
const (
	// StateAvailable represents a resource which will be used by any units at
	// this point in time
	StateAvailable State = "available"

	// StatePotential indicates there is a different revision of the resource
	// available in a repository. Used to let users know a resource can be
	// upgraded.
	StatePotential State = "potential"
)

// State identifies the resource state in an application
type State string

// ParseState converts the provided string into an State.
// If it is not a known state then an error is returned.
func ParseState(value string) (State, error) {
	state := State(value)
	return state, state.Validate()
}

// String returns the printable representation of the state.
func (o State) String() string {
	return string(o)
}

// Validate ensures that the state is correct.
func (o State) Validate() error {
	if _, ok := map[State]bool{
		StateAvailable: true,
		StatePotential: true,
	}[o]; !ok {
		return errors.Errorf("state %q invalid", o)
	}
	return nil
}
