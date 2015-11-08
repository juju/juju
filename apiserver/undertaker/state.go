// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package undertaker

import (
	"time"

	"github.com/juju/names"

	"github.com/juju/juju/state"
)

// State defines the needed methods of state.State
// for the work of the undertaker API.
type State interface {

	// Environment returns the environment entity.
	Environment() (Environment, error)

	// IsStateServer returns true if this state instance has the bootstrap
	// environment UUID.
	IsStateServer() bool

	// ProcessDyingEnviron checks if there are any machines or services left in
	// state. If there are none, the environment's life is changed from dying to dead.
	ProcessDyingEnviron() (err error)

	// RemoveAllEnvironDocs removes all documents from multi-environment
	// collections.
	RemoveAllEnvironDocs() error
}

type stateShim struct {
	*state.State
}

func (s *stateShim) Environment() (Environment, error) {
	return s.State.Environment()
}

// Environment defines the needed methods of state.Environment for
// the work of the undertaker API.
type Environment interface {

	// TimeOfDeath returns when the environment Life was set to Dead.
	TimeOfDeath() time.Time

	// Owner returns tag representing the owner of the environment.
	// The owner is the user that created the environment.
	Owner() names.UserTag

	// Life returns whether the environment is Alive, Dying or Dead.
	Life() state.Life

	// Name returns the human friendly name of the environment.
	Name() string

	// UUID returns the universally unique identifier of the environment.
	UUID() string

	// Destroy sets the environment's lifecycle to Dying, preventing
	// addition of services or machines to state.
	Destroy() error
}
