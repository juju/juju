// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package undertaker

import (
	"time"

	"github.com/juju/errors"
	"github.com/juju/names"

	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/state"
)

// State defines the needed methods of state.State
// for the work of the undertaker API.
type State interface {

	// Model returns the model entity.
	Model() (Model, error)

	// IsController returns true if this state instance has the bootstrap
	// model UUID.
	IsController() bool

	// ProcessDyingModel checks if there are any machines or services left in
	// state. If there are none, the model's life is changed from dying to dead.
	ProcessDyingModel() (err error)

	// RemoveAllModelDocs removes all documents from multi-environment
	// collections.
	RemoveAllModelDocs() error

	// AllMachines returns all machines in the model ordered by id.
	AllMachines() ([]Machine, error)

	// AllServices returns all deployed services in the model.
	AllServices() ([]Service, error)

	// ModelConfig retrieves the model configuration.
	ModelConfig() (*config.Config, error)
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

func (s *stateShim) Model() (Model, error) {
	return s.State.Model()
}

// Model defines the needed methods of state.Model for
// the work of the undertaker API.
type Model interface {

	// TimeOfDeath returns when the model Life was set to Dead.
	TimeOfDeath() time.Time

	// Owner returns tag representing the owner of the model.
	// The owner is the user that created the model.
	Owner() names.UserTag

	// Life returns whether the model is Alive, Dying or Dead.
	Life() state.Life

	// Name returns the human friendly name of the model.
	Name() string

	// UUID returns the universally unique identifier of the model.
	UUID() string

	// Destroy sets the model's lifecycle to Dying, preventing
	// addition of services or machines to state.
	Destroy() error
}
