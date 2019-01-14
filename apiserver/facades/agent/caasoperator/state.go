// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package caasoperator

import (
	"gopkg.in/juju/charm.v6"
	"gopkg.in/juju/names.v2"

	"github.com/juju/juju/core/status"
	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/network"
	"github.com/juju/juju/state"
)

// CAASOperatorState provides the subset of global state
// required by the CAAS operator facade.
type CAASOperatorState interface {
	Application(string) (Application, error)
	Model() (Model, error)
	ModelUUID() string
	FindEntity(names.Tag) (state.Entity, error)
	APIHostPortsForAgents() ([][]network.HostPort, error)
	Addresses() ([]string, error)
	WatchAPIHostPortsForAgents() state.NotifyWatcher
}

// Model provides the subset of CAAS model state required
// by the CAAS operator facade.
type Model interface {
	SetPodSpec(names.ApplicationTag, string) error
	Name() string
	UUID() string
	Type() state.ModelType
	ModelConfig() (*config.Config, error)
}

// Application provides the subset of application state
// required by the CAAS operator facade.
type Application interface {
	Charm() (Charm, bool, error)
	CharmModifiedVersion() int
	SetOperatorStatus(status.StatusInfo) error
	WatchUnits() state.StringsWatcher
	AllUnits() ([]Unit, error)
}

// Charm provides the subset of charm state required by the
// CAAS operator facade.
type Charm interface {
	URL() *charm.URL
	BundleSha256() string
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

type Unit interface {
	Tag() names.Tag
}
