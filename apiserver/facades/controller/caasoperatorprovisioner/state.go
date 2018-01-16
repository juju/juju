// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package caasoperatorprovisioner

import (
	"github.com/juju/errors"
	"github.com/juju/juju/status"
	"gopkg.in/juju/names.v2"

	"github.com/juju/juju/state"
)

// CAASOperatorProvisionerState provides the subset of global state
// required by the CAAS operator provisioner facade.
type CAASOperatorProvisionerState interface {
	WatchApplications() state.StringsWatcher
	FindEntity(names.Tag) (state.Entity, error)
	Application(string) (Application, error)
}

type stateShim struct {
	*state.State
}

func (s stateShim) Application(name string) (Application, error) {
	app, err := s.State.Application(name)
	if err != nil {
		return nil, errors.Trace(err)
	}
	return applicationShim{app}, nil
}

type Application interface {
	AllUnits() (units []Unit, err error)
	AddOperation(state.UnitUpdateProperties) *state.AddUnitOperation
	UpdateUnits(*state.UpdateUnitsOperation) error
}

type applicationShim struct {
	*state.Application
}

func (a applicationShim) AllUnits() ([]Unit, error) {
	all, err := a.Application.AllUnits()
	if err != nil {
		return nil, errors.Trace(err)
	}
	result := make([]Unit, len(all))
	for i, u := range all {
		result[i] = u
	}
	return result, nil
}

type Unit interface {
	Name() string
	Life() state.Life
	ProviderId() string
	AgentStatus() (status.StatusInfo, error)
	UpdateOperation(props state.UnitUpdateProperties) *state.UpdateUnitOperation
	DestroyOperation() *state.DestroyUnitOperation
}
