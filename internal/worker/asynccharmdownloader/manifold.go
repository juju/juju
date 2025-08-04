// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package asynccharmdownloader

import (
	"context"
	"net/url"

	"github.com/juju/clock"
	jujuerrors "github.com/juju/errors"
	"github.com/juju/worker/v4"
	"github.com/juju/worker/v4/dependency"

	"github.com/juju/juju/core/application"
	corehttp "github.com/juju/juju/core/http"
	"github.com/juju/juju/core/logger"
	"github.com/juju/juju/internal/charm/charmdownloader"
	"github.com/juju/juju/internal/charmhub"
	"github.com/juju/juju/internal/errors"
	"github.com/juju/juju/internal/services"
	internalworker "github.com/juju/juju/internal/worker"
)

// Downloader is responsible for downloading charms from the charm store.
type Downloader interface {
	// Download looks up the requested charm using the appropriate store, downloads
	// it to a temporary file and passes it to the configured storage API so it can
	// be persisted.
	//
	// The resulting charm is verified to be the right hash. It expected that the
	// origin will always have the correct hash following this call.
	//
	// Returns [ErrInvalidHash] if the hash of the downloaded charm does not match
	// the expected hash.
	Download(ctx context.Context, curl *url.URL, hash string) (*charmdownloader.DownloadResult, error)
}

// NewDownloaderFunc is a function that creates a new Downloader.
type NewDownloaderFunc func(charmhub.HTTPClient, logger.Logger) Downloader

// NewHTTPClientFunc is a function that creates a new HTTP client.
type NewHTTPClientFunc func(context.Context, corehttp.HTTPClientGetter) (corehttp.HTTPClient, error)

// NewAsyncDownloadWorkerFunc is a function that creates a new async worker.
type NewAsyncDownloadWorkerFunc func(
	appID application.ID,
	applicationService ApplicationService,
	downloader Downloader,
	clock clock.Clock,
	logger logger.Logger,
) worker.Worker

// ManifoldConfig describes the resources used by the charmdownloader worker.
type ManifoldConfig struct {
	DomainServicesName     string
	HTTPClientName         string
	NewDownloader          NewDownloaderFunc
	NewHTTPClient          NewHTTPClientFunc
	NewAsyncDownloadWorker NewAsyncDownloadWorkerFunc
	Logger                 logger.Logger
	Clock                  clock.Clock
}

// Manifold returns a Manifold that encapsulates the charmdownloader worker.
func Manifold(cfg ManifoldConfig) dependency.Manifold {
	return dependency.Manifold{
		Inputs: []string{
			cfg.DomainServicesName,
			cfg.HTTPClientName,
		},
		Start:  cfg.start,
		Filter: internalworker.ShouldWorkerUninstall,
	}
}

// Validate is called by start to check for bad configuration.
func (cfg ManifoldConfig) Validate() error {
	if cfg.DomainServicesName == "" {
		return jujuerrors.NotValidf("empty DomainServicesName")
	}
	if cfg.HTTPClientName == "" {
		return jujuerrors.NotValidf("empty HTTPClientName")
	}
	if cfg.NewDownloader == nil {
		return jujuerrors.NotValidf("nil NewDownloader")
	}
	if cfg.NewHTTPClient == nil {
		return jujuerrors.NotValidf("nil NewHTTPClient")
	}
	if cfg.NewAsyncDownloadWorker == nil {
		return jujuerrors.NotValidf("nil NewAsyncDownloadWorker")
	}
	if cfg.Logger == nil {
		return jujuerrors.NotValidf("nil Logger")
	}
	if cfg.Clock == nil {
		return jujuerrors.NotValidf("nil Clock")
	}
	return nil
}

// start is a StartFunc for a Worker manifold.
func (cfg ManifoldConfig) start(ctx context.Context, getter dependency.Getter) (worker.Worker, error) {
	if err := cfg.Validate(); err != nil {
		return nil, errors.Capture(err)
	}

	var domainServices services.DomainServices
	if err := getter.Get(cfg.DomainServicesName, &domainServices); err != nil {
		return nil, errors.Capture(err)
	}

	var httpClientGetter corehttp.HTTPClientGetter
	if err := getter.Get(cfg.HTTPClientName, &httpClientGetter); err != nil {
		return nil, errors.Capture(err)
	}

	w, err := NewWorker(Config{
		ApplicationService:     domainServices.Application(),
		HTTPClientGetter:       httpClientGetter,
		NewHTTPClient:          cfg.NewHTTPClient,
		NewDownloader:          cfg.NewDownloader,
		NewAsyncDownloadWorker: cfg.NewAsyncDownloadWorker,
		Logger:                 cfg.Logger,
		Clock:                  cfg.Clock,
	})
	if err != nil {
		return nil, errors.Capture(err)
	}
	return w, nil
}

// NewHTTPClient creates a new HTTP client.
func NewHTTPClient(ctx context.Context, getter corehttp.HTTPClientGetter) (corehttp.HTTPClient, error) {
	return getter.GetHTTPClient(ctx, corehttp.CharmhubPurpose)
}

// NewDownloader creates a new Downloader instance.
func NewDownloader(httpClient charmhub.HTTPClient, logger logger.Logger) Downloader {
	downloadClient := charmhub.NewDownloadClient(httpClient, charmhub.DefaultFileSystem(), logger)
	return charmdownloader.NewCharmDownloader(downloadClient, logger)
}
