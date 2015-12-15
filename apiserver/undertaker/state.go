// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package undertaker

import (
	"time"

	"github.com/juju/errors"
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

	// AllMachines returns all machines in the environment ordered by id.
	AllMachines() ([]Machine, error)

	// AllServices returns all deployed services in the environment.
	AllServices() ([]Service, error)
}

type stateShim struct {
	*state.State
}

func (s *stateShim) AllMachines() ([]Machine, error) {
	stateMachines, err := s.State.AllMachines()
	if err != nil {
		return nil, errors.Trace(err)
	}

	machines := make([]Machine, len(stateMachines))
	for i := range stateMachines {
		machines[i] = stateMachines[i]
	}

	return machines, nil
}

// Machine defines the needed methods of state.Machine for
// the work of the undertaker API.
type Machine interface {
	// Watch returns a watcher for observing changes to a machine.
	Watch() state.NotifyWatcher
}

func (s *stateShim) AllServices() ([]Service, error) {
	stateServices, err := s.State.AllServices()
	if err != nil {
		return nil, errors.Trace(err)
	}

	services := make([]Service, len(stateServices))
	for i := range stateServices {
		services[i] = stateServices[i]
	}

	return services, nil
}

// Service defines the needed methods of state.Service for
// the work of the undertaker API.
type Service interface {
	// Watch returns a watcher for observing changes to a service.
	Watch() state.NotifyWatcher
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
