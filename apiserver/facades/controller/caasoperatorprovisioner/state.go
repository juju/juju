// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package caasoperatorprovisioner

import (
	"gopkg.in/juju/charm.v6"
	"gopkg.in/juju/names.v3"

	"github.com/juju/juju/controller"
	"github.com/juju/juju/core/network"
	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/state"
)

// CAASOperatorProvisionerState provides the subset of global state
// required by the CAAS operator provisioner facade.
type CAASOperatorProvisionerState interface {
	ControllerConfig() (controller.Config, error)
	StateServingInfo() (controller.StateServingInfo, error)
	WatchApplications() state.StringsWatcher
	FindEntity(tag names.Tag) (state.Entity, error)
	Addresses() ([]string, error)
	ModelUUID() string
	Model() (Model, error)
	APIHostPortsForAgents() ([]network.SpaceHostPorts, error)
	WatchAPIHostPortsForAgents() state.NotifyWatcher
	Application(string) (Application, error)
}

type Model interface {
	UUID() string
	ModelConfig() (*config.Config, error)
}

type Application interface {
	Charm() (ch Charm, force bool, err error)
}

type Charm interface {
	Meta() *charm.Meta
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

func (s stateShim) Application(name string) (Application, error) {
	app, err := s.State.Application(name)
	if err != nil {
		return nil, err
	}
	return &applicationShim{app}, nil
}

type applicationShim struct {
	*state.Application
}

func (a *applicationShim) Charm() (Charm, bool, error) {
	return a.Application.Charm()
}
