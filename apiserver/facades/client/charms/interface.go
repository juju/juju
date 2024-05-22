// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package charms

import (
	"github.com/juju/errors"

	"github.com/juju/juju/apiserver/facades/client/charms/interfaces"
	"github.com/juju/juju/internal/charm"
	"github.com/juju/juju/internal/charm/services"
	"github.com/juju/juju/state"
)

type stateShim struct {
	*state.State
}

func newStateShim(st *state.State) interfaces.BackendState {
	return stateShim{
		State: st,
	}
}

func (s stateShim) UpdateUploadedCharm(charmInfo state.CharmInfo) (services.UploadedCharm, error) {
	ch, err := s.State.UpdateUploadedCharm(charmInfo)
	if err != nil {
		return nil, errors.Trace(err)
	}
	return stateCharmShim{Charm: ch}, nil
}

func (s stateShim) PrepareCharmUpload(curl string) (services.UploadedCharm, error) {
	ch, err := s.State.PrepareCharmUpload(curl)
	if err != nil {
		return nil, errors.Trace(err)
	}
	return stateCharmShim{Charm: ch}, nil
}

func (s stateShim) Application(name string) (interfaces.Application, error) {
	app, err := s.State.Application(name)
	if err != nil {
		return nil, errors.Trace(err)
	}
	return stateApplicationShim{Application: app}, nil
}

func (s stateShim) Machine(machineID string) (interfaces.Machine, error) {
	machine, err := s.State.Machine(machineID)
	return machine, errors.Trace(err)
}

type stateApplicationShim struct {
	*state.Application
}

func (s stateApplicationShim) AllUnits() ([]interfaces.Unit, error) {
	units, err := s.Application.AllUnits()
	if err != nil {
		return nil, errors.Trace(err)
	}
	results := make([]interfaces.Unit, len(units))
	for i, unit := range units {
		results[i] = unit
	}
	return results, nil
}

type stateCharmShim struct {
	*state.Charm
}

func (s stateCharmShim) IsUploaded() bool {
	return s.Charm.IsUploaded()
}

// StoreCharm represents a store charm.
type StoreCharm interface {
	charm.Charm
	charm.LXDProfiler
	Version() string
}
