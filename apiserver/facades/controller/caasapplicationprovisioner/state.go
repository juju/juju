// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package caasapplicationprovisioner

import (
	"github.com/juju/charm/v8"
	"github.com/juju/names/v4"

	"github.com/juju/juju/controller"
	"github.com/juju/juju/core/constraints"
	"github.com/juju/juju/core/network"
	"github.com/juju/juju/core/status"
	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/state"
)

// CAASApplicationProvisionerState provides the subset of global state
// required by the CAAS operator provisioner facade.
type CAASApplicationProvisionerState interface {
	ControllerConfig() (controller.Config, error)
	StateServingInfo() (controller.StateServingInfo, error)
	WatchApplications() state.StringsWatcher
	Addresses() ([]string, error)
	ModelUUID() string
	Model() (Model, error)
	APIHostPortsForAgents() ([]network.SpaceHostPorts, error)
	WatchAPIHostPortsForAgents() state.NotifyWatcher
	Application(string) (Application, error)
	ResolveConstraints(cons constraints.Value) (constraints.Value, error)
}

type Model interface {
	UUID() string
	ModelConfig() (*config.Config, error)
	Containers(providerIds ...string) ([]state.CloudContainer, error)
}

type Application interface {
	Charm() (ch Charm, force bool, err error)
	SetOperatorStatus(status.StatusInfo) error
	AllUnits() ([]Unit, error)
	UpdateUnits(unitsOp *state.UpdateUnitsOperation) error
	StorageConstraints() (map[string]state.StorageConstraints, error)
	DeviceConstraints() (map[string]state.DeviceConstraints, error)
	Name() string
	Constraints() (constraints.Value, error)
	Life() state.Life
}

type Charm interface {
	Meta() *charm.Meta
	URL() *charm.URL
}

type Unit interface {
	Tag() names.Tag
	DestroyOperation() *state.DestroyUnitOperation
	EnsureDead() error
	ContainerInfo() (state.CloudContainer, error)
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

func (a *applicationShim) AllUnits() ([]Unit, error) {
	units, err := a.Application.AllUnits()
	if err != nil {
		return nil, err
	}
	res := make([]Unit, 0, len(units))
	for _, unit := range units {
		res = append(res, unit)
	}
	return res, nil
}
