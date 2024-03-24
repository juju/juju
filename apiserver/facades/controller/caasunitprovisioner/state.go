// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package caasunitprovisioner

import (
	coreconfig "github.com/juju/juju/core/config"
	"github.com/juju/juju/core/network"
	"github.com/juju/juju/state"
)

// CAASUnitProvisionerState provides the subset of global state
// required by the CAAS unit provisioner facade.
type CAASUnitProvisionerState interface {
	Application(string) (Application, error)
}

// Application provides the subset of application state
// required by the CAAS unit provisioner facade.
type Application interface {
	GetScale() int
	SetScale(int, int64, bool) error
	WatchConfigSettingsHash() state.StringsWatcher
	WatchScale() state.NotifyWatcher
	ApplicationConfig() (coreconfig.ConfigAttributes, error)
	UpdateCloudService(providerId string, addresses []network.SpaceAddress) error
}

type stateShim struct {
	*state.State
}

type applicationShim struct {
	*state.Application
}

func (s stateShim) Application(id string) (Application, error) {
	app, err := s.State.Application(id)
	if err != nil {
		return nil, err
	}
	return applicationShim{app}, nil
}
