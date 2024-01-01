// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package caasoperatorprovisioner

import (
	"github.com/juju/names/v5"

	"github.com/juju/juju/apiserver/common"
	charmscommon "github.com/juju/juju/apiserver/common/charms"
	"github.com/juju/juju/controller"
	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/state"
)

// CAASOperatorProvisionerState provides the subset of model state
// required by the CAAS operator provisioner facade.
type CAASOperatorProvisionerState interface {
	WatchApplications() state.StringsWatcher
	FindEntity(tag names.Tag) (state.Entity, error)
	Model() (Model, error)
	Application(string) (Application, error)
}

// CAASControllerState provides the subset of controller state
// required by the CAAS operator provisioner facade.
type CAASControllerState interface {
	common.APIAddressAccessor
	ControllerConfig() (controller.Config, error)
	StateServingInfo() (controller.StateServingInfo, error)
}

type Model interface {
	UUID() string
	ModelConfig() (*config.Config, error)
}

type Application interface {
	Charm() (ch charmscommon.Charm, force bool, err error)
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

func (a *applicationShim) Charm() (charmscommon.Charm, bool, error) {
	return a.Application.Charm()
}
