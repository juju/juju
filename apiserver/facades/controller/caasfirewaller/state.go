// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package caasfirewaller

import (
	"github.com/juju/names/v5"

	charmscommon "github.com/juju/juju/apiserver/common/charms"
	"github.com/juju/juju/core/config"
	"github.com/juju/juju/core/network"
	"github.com/juju/juju/state"
)

// CAASFirewallerState provides the subset of global state
// required by the CAAS operator facade.
type CAASFirewallerState interface {
	FindEntity(tag names.Tag) (state.Entity, error)
	Application(string) (Application, error)

	WatchApplications() state.StringsWatcher
	WatchOpenedPorts() state.StringsWatcher
}

// Application provides the subset of application state
// required by the CAAS operator facade.
type Application interface {
	IsExposed() bool
	ApplicationConfig() (config.ConfigAttributes, error)
	Watch() state.NotifyWatcher
	Charm() (ch charmscommon.Charm, force bool, err error)
	OpenedPortRanges() (network.GroupedPortRanges, error)
}

type stateShim struct {
	*state.State
}

func (s *stateShim) Application(id string) (Application, error) {
	app, err := s.State.Application(id)
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

func (a *applicationShim) OpenedPortRanges() (network.GroupedPortRanges, error) {
	pg, err := a.Application.OpenedPortRanges()
	if err != nil {
		return nil, err
	}
	return pg.ByEndpoint(), nil
}
