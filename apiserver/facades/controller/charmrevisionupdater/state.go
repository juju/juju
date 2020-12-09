// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package charmrevisionupdater

import (
	"github.com/juju/charm/v8"
	csparams "github.com/juju/charmrepo/v6/csclient/params"
	"github.com/juju/errors"
	"github.com/juju/names/v4"

	"github.com/juju/juju/apiserver/common"
	"github.com/juju/juju/cloud"
	"github.com/juju/juju/controller"
	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/state"
)

// State is the subset of *state.State that we need.
type State interface {
	AddCharmPlaceholder(curl *charm.URL) error
	AllApplications() ([]Application, error)
	Charm(curl *charm.URL) (*state.Charm, error)
	Cloud(name string) (cloud.Cloud, error)
	ControllerConfig() (controller.Config, error)
	ControllerUUID() string
	Model() (Model, error)
	Resources() (state.Resources, error)
}

// Application is the subset of *state.Application that we need.
type Application interface {
	CharmURL() (curl *charm.URL, force bool)
	CharmOrigin() *state.CharmOrigin
	Channel() csparams.Channel
	ApplicationTag() names.ApplicationTag
}

// Model is the subset of *state.Model that we need.
type Model interface {
	CloudName() string
	CloudRegion() string
	Config() (*config.Config, error)
	IsControllerModel() bool
	UUID() string
}

// stateShim takes a *state.State and implements this package's State interface.
type stateShim struct {
	*state.State
}

func (s stateShim) AllApplications() ([]Application, error) {
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

func (s stateShim) Model() (Model, error) {
	return s.State.Model()
}

// charmhubClientStateShim takes a *state.State and and implements common.ModelGetter.
type charmhubClientStateShim struct {
	state State
}

func (s charmhubClientStateShim) Model() (common.ConfigModel, error) {
	return s.state.Model()
}
