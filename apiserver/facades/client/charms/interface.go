// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package charms

import (
	"github.com/juju/charm/v8"
	"github.com/juju/errors"
	"github.com/juju/juju/core/lxdprofile"
	"github.com/juju/names/v4"

	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/controller"
	corecharm "github.com/juju/juju/core/charm"
	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/state"
)

type BackendState interface {
	AllCharms() ([]*state.Charm, error)
	Charm(curl *charm.URL) (*state.Charm, error)
	ControllerConfig() (controller.Config, error)
	ControllerTag() names.ControllerTag
	CharmState
	state.MongoSessioner
	ModelUUID() string
}

type csStateShim struct {
	*state.State
}

func newStateShim(st *state.State) BackendState {
	return csStateShim{
		State: st,
	}
}

func (s csStateShim) PrepareCharmUpload(curl *charm.URL) (corecharm.StateCharm, error) {
	ch, err := s.State.PrepareCharmUpload(curl)
	if err != nil {
		return nil, errors.Trace(err)
	}
	return csStateCharmShim{Charm: ch}, nil
}

type BackendModel interface {
	Config() (*config.Config, error)
	ModelTag() names.ModelTag
}

// CharmState represents directives for accessing charm methods
type CharmState interface {
	UpdateUploadedCharm(info state.CharmInfo) (*state.Charm, error)
	PrepareCharmUpload(curl *charm.URL) (corecharm.StateCharm, error)
}

type csStateCharmShim struct {
	*state.Charm
}

func (s csStateCharmShim) IsUploaded() bool {
	return s.Charm.IsUploaded()
}

// Repository is the part of charmrepo.Charmstore that we need to
// resolve a charm url and get a charm archive.
type Repository interface {
	// Get reads the charm referenced by curl into a file
	// with the given path, which will be created if needed. Note that
	// the path's parent directory must already exist.
	Get(curl *charm.URL, archivePath string) (*charm.CharmArchive, error)
	ResolveWithPreferredChannel(*charm.URL, params.CharmOrigin) (*charm.URL, params.CharmOrigin, []string, error)
}

// StoreCharm represents a store charm.
type StoreCharm interface {
	charm.Charm
	charm.LXDProfiler
	Version() string
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
	Run(corecharm.State, corecharm.JujuVersionValidator) (corecharm.DownloadResult, bool, error)
	Validate() error
}
