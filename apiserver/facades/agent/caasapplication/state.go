// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package caasapplication

import (
	"github.com/juju/charm/v12"
	"github.com/juju/names/v4"
	"github.com/juju/version/v2"

	"github.com/juju/juju/caas"
	jujucontroller "github.com/juju/juju/controller"
	"github.com/juju/juju/core/network"
	"github.com/juju/juju/state"
)

// State provides the subset of model state
// required by the CAAS application facade.
type State interface {
	Application(string) (Application, error)
	Model() (Model, error)
	Unit(name string) (Unit, error)
}

// ControllerState provides the subset of controller state
// required by the CAAS application facade.
type ControllerState interface {
	ControllerConfig() (jujucontroller.Config, error)
	APIHostPortsForAgents() ([]network.SpaceHostPorts, error)
}

// Model provides the subset of CAAS model state required
// by the CAAS application facade.
type Model interface {
	AgentVersion() (version.Number, error)
	ControllerTag() names.ControllerTag
	Tag() names.Tag
}

// Application provides the subset of application state
// required by the CAAS application facade.
type Application interface {
	Life() state.Life
	Name() string
	UpsertCAASUnit(args state.UpsertCAASUnitParams) (Unit, error)
	GetScale() int
}

// Charm provides the subset of charm state required by the
// CAAS application facade.
type Charm interface {
	Meta() *charm.Meta
}

type stateShim struct {
	*state.State
}

func (s stateShim) Application(id string) (Application, error) {
	app, err := s.State.Application(id)
	if err != nil {
		return nil, err
	}
	return applicationShim{app}, nil
}

func (s stateShim) Model() (Model, error) {
	model, err := s.State.Model()
	if err != nil {
		return nil, err
	}
	return model.CAASModel()
}

func (s stateShim) Unit(name string) (Unit, error) {
	u, err := s.State.Unit(name)
	if err != nil {
		return nil, err
	}
	return Unit(u), nil
}

type applicationShim struct {
	*state.Application
}

func (a applicationShim) Charm() (Charm, bool, error) {
	return a.Application.Charm()
}

func (a applicationShim) AllUnits() ([]Unit, error) {
	all, err := a.Application.AllUnits()
	if err != nil {
		return nil, err
	}
	result := make([]Unit, len(all))
	for i, u := range all {
		result[i] = u
	}
	return result, nil
}

func (a applicationShim) UpsertCAASUnit(args state.UpsertCAASUnitParams) (Unit, error) {
	u, err := a.Application.UpsertCAASUnit(args)
	if err != nil {
		return nil, err
	}
	return Unit(u), nil
}

type Unit interface {
	Tag() names.Tag
	ContainerInfo() (state.CloudContainer, error)
	Life() state.Life
	Refresh() error
	ApplicationName() string
}

// Broker contains methods from the caas.Broker interface used by the caasapplication facade.
type Broker interface {
	Application(string, caas.DeploymentType) caas.Application
}
