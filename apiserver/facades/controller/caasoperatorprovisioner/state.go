// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package caasoperatorprovisioner

import (
	"gopkg.in/juju/names.v2"

	"github.com/juju/juju/controller"
	"github.com/juju/juju/core/network"
	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/state"
)

// CAASOperatorProvisionerState provides the subset of global state
// required by the CAAS operator provisioner facade.
type CAASOperatorProvisionerState interface {
	ControllerConfig() (controller.Config, error)
	WatchApplications() state.StringsWatcher
	FindEntity(tag names.Tag) (state.Entity, error)
	Addresses() ([]string, error)
	ModelUUID() string
	Model() (Model, error)
	APIHostPortsForAgents() ([][]network.HostPort, error)
	WatchAPIHostPortsForAgents() state.NotifyWatcher
}

type Model interface {
	UUID() string
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
