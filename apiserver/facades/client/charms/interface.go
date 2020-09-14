// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package charms

import (
	"net/url"

	"github.com/juju/charm/v8"
	"github.com/juju/errors"
	"github.com/juju/names/v4"

	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/controller"
	corecharm "github.com/juju/juju/core/charm"
	"github.com/juju/juju/core/lxdprofile"
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

// Repository represents the necessary methods to resolve and download
// charms from a repository where they reside.
type Repository interface {
	// FindDownloadURL returns a url from which a charm can be downloaded
	// based on the given charm url and charm origin.  A charm origin
	// updated with the ID and hash for the download is also returned.
	FindDownloadURL(curl *charm.URL, origin corecharm.Origin) (*url.URL, corecharm.Origin, error)

	// DownloadCharm reads the charm referenced by curl or downloadURL into
	// a file with the given path, which will be created if needed. Note
	// that the path's parent directory must already exist.
	DownloadCharm(curl *charm.URL, downloadURL *url.URL, archivePath string) (*charm.CharmArchive, error)

	// ResolveWithPreferredChannel verified that the charm with the requested
	// channel exists.  If no channel is specified, the latests, most stable is
	// is used. It returns a charm URL which includes the most current revision,
	// if none was provided, a charm origin, and a slice of series supported by
	// this charm.
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
	Run(corecharm.State, corecharm.JujuVersionValidator, corecharm.Origin) (corecharm.DownloadResult, bool, corecharm.Origin, error)
	Validate() error
}
