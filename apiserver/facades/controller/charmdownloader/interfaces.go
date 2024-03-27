// Copyright 2021 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package charmdownloader

import (
	"context"

	"github.com/juju/charm/v13"
	"github.com/juju/worker/v4"

	"github.com/juju/juju/controller"
	corecharm "github.com/juju/juju/core/charm"
	"github.com/juju/juju/core/status"
	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/internal/charm/services"
	"github.com/juju/juju/state"
)

// StateBackend describes an API for accessing/mutating information in state.
type StateBackend interface {
	WatchApplicationsWithPendingCharms() state.StringsWatcher
	ControllerConfig() (controller.Config, error)
	UpdateUploadedCharm(info state.CharmInfo) (services.UploadedCharm, error)
	PrepareCharmUpload(curl string) (services.UploadedCharm, error)
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
	SetStatus(status.StatusInfo, status.StatusHistoryRecorder) error
	CharmOrigin() *corecharm.Origin
	Charm() (Charm, bool, error)
	SetDownloadedIDAndHash(id, hash string) error
}

// Charm provides an API for querying charm details.
type Charm interface {
	URL() string
}

// Downloader defines an API for downloading and storing charms.
type Downloader interface {
	DownloadAndStore(ctx context.Context, charmURL *charm.URL, requestedOrigin corecharm.Origin, force bool) (corecharm.Origin, error)
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
	Register(worker.Worker) string
}
