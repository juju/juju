// Copyright 2021 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package charmdownloader

import (
	"github.com/juju/charm/v11"

	"github.com/juju/juju/apiserver/facades/client/charms/services"
	"github.com/juju/juju/controller"
	corecharm "github.com/juju/juju/core/charm"
	"github.com/juju/juju/core/status"
	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/state"
)

// StateBackend describes an API for accessing/mutating information in state.
type StateBackend interface {
	WatchApplicationsWithPendingCharms() state.StringsWatcher
	ControllerConfig() (controller.Config, error)
	UpdateUploadedCharm(info state.CharmInfo) (services.UploadedCharm, error)
	PrepareCharmUpload(curl *charm.URL) (services.UploadedCharm, error)
	ModelUUID() string
	Application(string) (Application, error)
}

// ModelBackend describes an API for accessing model-specific details.
type ModelBackend interface {
	Config() (*config.Config, error)
}

// Application provides an API for querying application-specific details.
type Application interface {
	CharmPendingToBeDownloaded() bool
	SetStatus(status.StatusInfo) error
	CharmOrigin() *corecharm.Origin
	Charm() (Charm, bool, error)
	SetDownloadedIDAndHash(id, hash string) error
}

// Charm provides an API for querying charm details.
type Charm interface {
	URL() *charm.URL
}

// Downloader defines an API for downloading and storing charms.
type Downloader interface {
	DownloadAndStore(charmURL *charm.URL, requestedOrigin corecharm.Origin, force bool) (corecharm.Origin, error)
}

// AuthChecker provides an API for checking if the API client is a controller.
type AuthChecker interface {
	// AuthController returns true if the entity performing the current API
	// call is a machine acting as a controller.
	AuthController() bool
}

// ResourcesBackend handles the registration of a stoppable resource and
// controls its lifecycle.
type ResourcesBackend interface {
	Register(StoppableResource) string
}

// StoppableResource is implemented by resources that can be stopped.
type StoppableResource interface {
	Stop() error
}
