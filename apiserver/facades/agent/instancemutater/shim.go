// Copyright 2019 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package instancemutater

import (
	"gopkg.in/juju/charm.v6"

	"github.com/juju/juju/core/cache"
	"github.com/juju/juju/core/lxdprofile"
	"github.com/juju/juju/state"
)

type instanceMutaterStateShim struct {
	*state.State
}

func (s *instanceMutaterStateShim) Application(appName string) (Application, error) {
	app, err := s.State.Application(appName)
	if err != nil {
		return nil, err
	}
	return &application{
		Application: app,
	}, nil
}

func (s *instanceMutaterStateShim) Charm(curl *charm.URL) (Charm, error) {
	ch, err := s.State.Charm(curl)
	if err != nil {
		return nil, err
	}
	return &stateCharm{
		Charm: ch,
	}, nil
}

func (s instanceMutaterStateShim) Machine(machineId string) (Machine, error) {
	m, err := s.State.Machine(machineId)
	if err != nil {
		return nil, err
	}
	return &machine{
		Machine: m,
	}, nil
}

// modelCacheShim is used as a shim between the
// cache.PredicateStringsWatcher and cache.StringsWatcher to enable better mock testing.
type modelCacheShim struct {
	*cache.Model
}

func (s *modelCacheShim) WatchMachines() (cache.StringsWatcher, error) {
	return s.Model.WatchMachines()
}

func (s modelCacheShim) Machine(machineId string) (ModelCacheMachine, error) {
	machine, err := s.Model.Machine(machineId)
	if err != nil {
		return nil, err
	}
	return &modelCacheMachine{
		Machine: machine,
	}, nil
}

type modelCacheMachine struct {
	cache.Machine
}

func (m *modelCacheMachine) WatchLXDProfileVerificationNeeded() (cache.NotifyWatcher, error) {
	return m.Machine.WatchLXDProfileVerificationNeeded()
}

func (m *modelCacheMachine) WatchContainers() (cache.StringsWatcher, error) {
	return m.Machine.WatchContainers()
}

type stateCharm struct {
	*state.Charm
}

func (s *stateCharm) LXDProfile() lxdprofile.Profile {
	profile := s.Charm.LXDProfile()
	return lxdprofile.Profile{
		Config:      profile.Config,
		Description: profile.Description,
		Devices:     profile.Devices,
	}
}

type unit struct {
	*state.Unit
}

func (u *unit) Application() string {
	return u.Unit.ApplicationName()
}

type application struct {
	*state.Application
}

func (a *application) CharmURL() *charm.URL {
	curl, _ := a.Application.CharmURL()
	return curl
}

type machine struct {
	*state.Machine
}

func (m *machine) Units() ([]Unit, error) {
	units, err := m.Machine.Units()
	if err != nil {
		return nil, err
	}
	result := make([]Unit, len(units))
	for k, v := range units {
		result[k] = &unit{
			Unit: v,
		}
	}
	return result, nil
}
