// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package undertaker

import (
	"github.com/juju/names/v4"

	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/state"
)

// State defines the needed methods of state.State
// for the work of the undertaker API.
type State interface {
	state.EntityFinder

	// Model returns the model entity.
	Model() (Model, error)

	// IsController returns true if this state instance has the bootstrap
	// model UUID.
	IsController() bool

	// ProcessDyingModel checks if there are any machines or services left in
	// state. If there are none, the model's life is changed from dying to dead.
	ProcessDyingModel() (err error)

	// RemoveDyingModel sets current model to dead then removes all documents from
	// multi-model collections.
	RemoveDyingModel() error

	// ModelConfig retrieves the model configuration.
	ModelConfig() (*config.Config, error)

	// WatchModelEntityReferences gets a watcher capable of monitoring
	// model entity references changes.
	WatchModelEntityReferences(mUUID string) state.NotifyWatcher

	// ModelUUID returns the model UUID for the model controlled
	// by this state instance.
	ModelUUID() string
}

// TODO - CAAS(ericclaudejones): This should contain state alone, model will be
// removed once all relevant methods are moved from state to model.
type stateShim struct {
	*state.State
	model *state.Model
}

func (s *stateShim) Model() (Model, error) {
	return s.State.Model()
}

func (s *stateShim) ModelConfig() (*config.Config, error) {
	return s.model.Config()
}

// Model defines the needed methods of state.Model for
// the work of the undertaker API.
type Model interface {

	// Owner returns tag representing the owner of the model.
	// The owner is the user that created the model.
	Owner() names.UserTag

	// Life returns whether the model is Alive, Dying or Dead.
	Life() state.Life

	// ForceDestroyed returns whether the dying/dead model was
	// destroyed with --force. Always false for alive models.
	ForceDestroyed() bool

	// Name returns the human friendly name of the model.
	Name() string

	// UUID returns the universally unique identifier of the model.
	UUID() string
}
