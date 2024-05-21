// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package charmrevisionupdater

import (
	"github.com/juju/errors"
	"github.com/juju/juju/internal/charm"
	"github.com/juju/names/v5"

	"github.com/juju/juju/apiserver/common"
	"github.com/juju/juju/core/objectstore"
	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/state"
)

// State is the subset of *state.State that we need.
type State interface {
	AddCharmPlaceholder(curl *charm.URL) error
	AllApplications() ([]Application, error)
	Charm(curl string) (*state.Charm, error)
	ControllerUUID() string
	Model() (Model, error)
	Resources(objectstore.ObjectStore) state.Resources
	AliveRelationKeys() []string
}

// Application is the subset of *state.Application that we need.
type Application interface {
	CharmURL() (curl *string, force bool)
	CharmOrigin() *state.CharmOrigin
	ApplicationTag() names.ApplicationTag
	UnitCount() int
}

// Model is the subset of *state.Model that we need.
type Model interface {
	CloudName() string
	CloudRegion() string
	Config() (*config.Config, error)
	IsControllerModel() bool
	Metrics() (state.ModelMetrics, error)
	ModelTag() names.ModelTag
	UUID() string
}

// StateShim takes a *state.State and implements this package's State interface.
type StateShim struct {
	*state.State
}

func (s StateShim) AllApplications() ([]Application, error) {
	stateApps, err := s.State.AllApplications()
	if err != nil {
		return nil, errors.Trace(err)
	}
	apps := make([]Application, len(stateApps))
	for i, a := range stateApps {
		apps[i] = a
	}
	return apps, nil
}

func (s StateShim) Model() (Model, error) {
	return s.State.Model()
}

// charmhubClientStateShim takes a *state.State and implements common.ModelGetter.
type charmhubClientStateShim struct {
	state State
}

func (s charmhubClientStateShim) Model() (common.ConfigModel, error) {
	return s.state.Model()
}
