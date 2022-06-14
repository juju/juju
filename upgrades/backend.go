// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package upgrades

import (
	"github.com/juju/errors"

	"github.com/juju/juju/controller"
	environscloudspec "github.com/juju/juju/environs/cloudspec"
	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/state"
)

// StateBackend provides an interface for upgrading the global state database.
type StateBackend interface {
	ControllerUUID() (string, error)
	StateServingInfo() (controller.StateServingInfo, error)
	ControllerConfig() (controller.Config, error)
}

// Model is an interface providing access to the details of a model within the
// controller.
type Model interface {
	Config() (*config.Config, error)
	CloudSpec() (environscloudspec.CloudSpec, error)
}

// NewStateBackend returns a new StateBackend using a *state.StatePool object.
func NewStateBackend(pool *state.StatePool) StateBackend {
	return stateBackend{pool}
}

type stateBackend struct {
	pool *state.StatePool
}

func (s stateBackend) ControllerUUID() (string, error) {
	systemState, err := s.pool.SystemState()
	return systemState.ControllerUUID(), err
}

func (s stateBackend) StateServingInfo() (controller.StateServingInfo, error) {
	systemState, err := s.pool.SystemState()
	if err != nil {
		return controller.StateServingInfo{}, errors.Trace(err)
	}
	ssi, errS := systemState.StateServingInfo()
	if errS != nil {
		return controller.StateServingInfo{}, errors.Trace(err)
	}
	return ssi, err
}

func (s stateBackend) ControllerConfig() (controller.Config, error) {
	systemState, err := s.pool.SystemState()
	if err != nil {
		return nil, errors.Trace(err)
	}
	return systemState.ControllerConfig()
}
