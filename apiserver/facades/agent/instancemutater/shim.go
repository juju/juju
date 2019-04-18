// Copyright 2019 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package instancemutater

import (
	"github.com/juju/juju/core/cache"
	"github.com/juju/juju/state"
)

type instanceMutaterStateShim struct {
	*state.State
}

// modelCacheShim is used as a shim between the
// cache.PredicateStringsWatcher and cache.StringsWatcher to enable better mock testing.
type modelCacheShim struct {
	*cache.Model
}

func (s *modelCacheShim) WatchMachines() (cache.StringsWatcher, error) {
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
