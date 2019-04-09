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

func (i *instanceMutaterStateShim) Model() (Model, error) {
	model, err := i.State.Model()
	if err != nil {
		return nil, err
	}
	return &modelShim{Model: model}, nil
}

func (i *instanceMutaterStateShim) Unit(name string) (Unit, error) {
	unit, err := i.State.Unit(name)
	if err != nil {
		return nil, err
	}
	return &unitShim{Unit: unit}, nil
}

type modelShim struct {
	*state.Model
}

type unitShim struct {
	*state.Unit
}

func (u *unitShim) Application() (Application, error) {
	app, err := u.Unit.Application()
	if err != nil {
		return nil, err
	}
	return &applicationShim{Application: app}, nil
}

type applicationShim struct {
	*state.Application
}

func (a *applicationShim) Charm() (Charm, error) {
	ch, _, err := a.Application.Charm()
	if err != nil {
		return nil, err
	}
	return &charmShim{Charm: ch}, nil
}

type charmShim struct {
	Charm *state.Charm
}

func (c *charmShim) LXDProfile() LXDProfile {
	return lxdProfileShim{
		LXDProfile: c.Charm.LXDProfile(),
	}
}

func (c *charmShim) Revision() int {
	return c.Charm.Revision()
}

type lxdProfileShim struct {
	LXDProfile *charm.LXDProfile
}

func (l lxdProfileShim) Config() map[string]string {
	return l.LXDProfile.Config
}

func (l lxdProfileShim) Description() string {
	return l.LXDProfile.Description
}

func (l lxdProfileShim) Devices() map[string]map[string]string {
	return l.LXDProfile.Devices
}

func (l lxdProfileShim) Empty() bool {
	return l.LXDProfile.Empty()
}

func (l lxdProfileShim) ValidateConfigDevices() error {
	return l.LXDProfile.ValidateConfigDevices()
}

// lxdCharmProfiler massages a *state.Charm into a LXDProfiler
// inside of the core package.
type lxdCharmProfiler struct {
	Charm Charm
}

// LXDProfile implements core.lxdprofile.LXDProfiler
func (p lxdCharmProfiler) LXDProfile() lxdprofile.LXDProfile {
	if p.Charm == nil {
		return nil
	}
	return p.Charm.LXDProfile()
}

// modelCacheShim is used as a shim between the
// cache.PredicateStringsWatcher and cache.StringsWatcher to enable better mock testing.
type modelCacheShim struct {
	*cache.Model
}

func (s *modelCacheShim) WatchMachines() cache.StringsWatcher {
	return s.Model.WatchMachines()
}

func (s modelCacheShim) Charm(charmURL string) (ModelCacheCharm, error) {
	ch, err := s.Model.Charm(charmURL)
	if err != nil {
		return nil, err
	}
	return &modelCacheCharm{
		Charm: ch,
	}, nil
}

func (s modelCacheShim) Application(appName string) (ModelCacheApplication, error) {
	app, err := s.Model.Application(appName)
	if err != nil {
		return nil, err
	}
	return &modelCacheApplication{
		Application: app,
	}, nil
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
	*cache.Machine
}

func (m *modelCacheMachine) WatchApplicationLXDProfiles() (cache.NotifyWatcher, error) {
	return m.Machine.WatchApplicationLXDProfiles()
}

func (m *modelCacheMachine) WatchContainers() (cache.StringsWatcher, error) {
	return m.Machine.WatchContainers()
}

func (m *modelCacheMachine) Units() ([]ModelCacheUnit, error) {
	units, err := m.Machine.Units()
	if err != nil {
		return nil, err
	}
	result := make([]ModelCacheUnit, len(units))
	for k, v := range units {
		result[k] = &modelCacheUnit{
			Unit: v,
		}
	}
	return result, nil
}

type modelCacheCharm struct {
	*cache.Charm
}

type modelCacheUnit struct {
	*cache.Unit
}

type modelCacheApplication struct {
	*cache.Application
}
