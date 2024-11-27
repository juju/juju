// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package asynccharmdownloader

import (
	"context"

	jujuerrors "github.com/juju/errors"
	"github.com/juju/worker/v4"
	"github.com/juju/worker/v4/dependency"

	corecharm "github.com/juju/juju/core/charm"
	corehttp "github.com/juju/juju/core/http"
	"github.com/juju/juju/core/logger"
	"github.com/juju/juju/internal/charm/charmdownloader"
	charmservices "github.com/juju/juju/internal/charm/services"
	"github.com/juju/juju/internal/charmhub"
	"github.com/juju/juju/internal/errors"
	"github.com/juju/juju/internal/services"
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
	Download(ctx context.Context, name string, requestedOrigin corecharm.Origin) (*charmdownloader.DownloadResult, error)
}

// NewDownloaderFunc is a function that creates a new Downloader.
type NewDownloaderFunc func(charmhub.HTTPClient, ModelConfigService, logger.Logger) Downloader

// NewHTTPClientFunc is a function that creates a new HTTP client.
type NewHTTPClientFunc func(context.Context, corehttp.HTTPClientGetter) (corehttp.HTTPClient, error)

// ManifoldConfig describes the resources used by the charmdownloader worker.
type ManifoldConfig struct {
	DomainServicesName string
	HTTPClientName     string
	NewDownloader      NewDownloaderFunc
	NewHTTPClient      NewHTTPClientFunc
	Logger             logger.Logger
}

// Manifold returns a Manifold that encapsulates the charmdownloader worker.
func Manifold(cfg ManifoldConfig) dependency.Manifold {
	return dependency.Manifold{
		Inputs: []string{
			cfg.DomainServicesName,
			cfg.HTTPClientName,
		},
		Start: cfg.start,
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
	if cfg.Logger == nil {
		return jujuerrors.NotValidf("nil Logger")
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
		ApplicationService: domainServices.Application(),
		ModelConfigService: domainServices.Config(),
		HTTPClientGetter:   httpClientGetter,
		NewHTTPClient:      cfg.NewHTTPClient,
		NewDownloader:      cfg.NewDownloader,
		Logger:             cfg.Logger,
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
func NewDownloader(httpClient charmhub.HTTPClient, modelConfigService ModelConfigService, logger logger.Logger) Downloader {
	factory := charmservices.NewCharmRepoFactory(charmservices.CharmRepoFactoryConfig{
		CharmhubHTTPClient: httpClient,
		ModelConfigService: modelConfigService,
		Logger:             logger,
	})

	return charmdownloader.NewCharmDownloader(repoFactory{
		factory: factory,
	}, logger)
}

type repoFactory struct {
	factory *charmservices.CharmRepoFactory
}

func (s repoFactory) GetCharmRepository(ctx context.Context, src corecharm.Source) (charmdownloader.CharmRepository, error) {
	return s.factory.GetCharmRepository(ctx, src)
}
