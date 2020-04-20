// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package caasmodeloperator

import (
	"github.com/juju/names/v4"

	"github.com/juju/juju/controller"
	"github.com/juju/juju/core/network"
	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/state"
)

// CAASModelOperatorState provides the subset of global state required by the
// model operator provisioner
type CAASModelOperatorState interface {
	Addresses() ([]string, error)
	APIHostPortsForAgents() ([]network.SpaceHostPorts, error)
	ControllerConfig() (controller.Config, error)
	FindEntity(tag names.Tag) (state.Entity, error)
	Model() (Model, error)
	ModelUUID() string
	WatchAPIHostPortsForAgents() state.NotifyWatcher
}

type Model interface {
	ModelConfig() (*config.Config, error)
}

type stateShim struct {
	*state.State
}

func (s stateShim) Model() (Model, error) {
	model, err := s.State.Model()
	if err != nil {
		return nil, err
	}
	return model.CAASModel()
}
