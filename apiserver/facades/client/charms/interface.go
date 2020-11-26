// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package charms

import (
	"github.com/juju/charm/v8"
	"github.com/juju/errors"

	"github.com/juju/juju/apiserver/facades/client/charms/interfaces"
	corecharm "github.com/juju/juju/core/charm"
	"github.com/juju/juju/core/lxdprofile"
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

func (s stateShim) PrepareCharmUpload(curl *charm.URL) (corecharm.StateCharm, error) {
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

// storeCharmShim massages a *charm.CharmArchive into a LXDProfiler
// inside of the core package.
type storeCharmShim struct {
	*charm.CharmArchive
}

func newStoreCharmShim(archive *charm.CharmArchive) *storeCharmShim {
	return &storeCharmShim{
		CharmArchive: archive,
	}
}

// LXDProfile implements core.lxdprofile.LXDProfiler
func (p *storeCharmShim) LXDProfile() *charm.LXDProfile {
	if p.CharmArchive == nil {
		return nil
	}

	profile := p.CharmArchive.LXDProfile()
	if profile == nil {
		return nil
	}
	return profile
}

// StoreCharm represents a store charm.
type StoreCharm interface {
	charm.Charm
	charm.LXDProfiler
	Version() string
}

// storeCharmLXDProfiler massages a *charm.CharmArchive into a LXDProfiler
// inside of the core package.
type storeCharmLXDProfiler struct {
	StoreCharm
}

func makeStoreCharmLXDProfiler(shim StoreCharm) storeCharmLXDProfiler {
	return storeCharmLXDProfiler{
		StoreCharm: shim,
	}
}

// LXDProfile implements core.lxdprofile.LXDProfiler
func (p storeCharmLXDProfiler) LXDProfile() lxdprofile.LXDProfile {
	if p.StoreCharm == nil {
		return nil
	}
	profile := p.StoreCharm.LXDProfile()
	if profile == nil {
		return nil
	}
	return profile
}

// Strategy represents a core charm Strategy
type Strategy interface {
	CharmURL() *charm.URL
	Finish() error
	Run(corecharm.State, corecharm.JujuVersionValidator, corecharm.Origin) (corecharm.DownloadResult, bool, corecharm.Origin, error)
	Validate() error
}
