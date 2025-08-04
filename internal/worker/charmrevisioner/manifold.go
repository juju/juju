// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package charmrevisioner

import (
	"context"
	"time"

	"github.com/juju/clock"
	jujuerrors "github.com/juju/errors"
	"github.com/juju/names/v6"
	"github.com/juju/worker/v4"
	"github.com/juju/worker/v4/dependency"

	corehttp "github.com/juju/juju/core/http"
	"github.com/juju/juju/core/logger"
	"github.com/juju/juju/internal/charmhub"
	"github.com/juju/juju/internal/charmhub/transport"
	"github.com/juju/juju/internal/errors"
	"github.com/juju/juju/internal/services"
	internalworker "github.com/juju/juju/internal/worker"
)

// CharmhubClient is responsible for refreshing charms from the charm store.
type CharmhubClient interface {
	// RefreshWithMetricsOnly defines a client making a refresh API call with no
	// action, whose purpose is to send metrics data for models without current
	// units.  E.G. the controller model.
	RefreshWithMetricsOnly(context.Context, charmhub.Metrics) error

	// RefreshWithRequestMetrics defines a client making a refresh API call with
	// a request and metrics data.
	RefreshWithRequestMetrics(context.Context, charmhub.RefreshConfig, charmhub.Metrics) ([]transport.RefreshResponse, error)
}

// NewCharmhubClientFunc is a function that creates a new CharmhubClient.
type NewCharmhubClientFunc func(charmhub.HTTPClient, string, logger.Logger) (CharmhubClient, error)

// NewHTTPClientFunc is a function that creates a new HTTP client.
type NewHTTPClientFunc func(context.Context, corehttp.HTTPClientGetter) (corehttp.HTTPClient, error)

// ManifoldConfig describes how to create a worker that checks for updates
// available to deployed charms in an environment.
type ManifoldConfig struct {
	DomainServicesName string
	HTTPClientName     string
	Period             time.Duration
	NewWorker          func(Config) (worker.Worker, error)
	ModelTag           names.ModelTag
	NewHTTPClient      NewHTTPClientFunc
	NewCharmhubClient  NewCharmhubClientFunc
	Logger             logger.Logger
	Clock              clock.Clock
}

// Validate is called by start to check for bad configuration.
func (cfg ManifoldConfig) Validate() error {
	if cfg.DomainServicesName == "" {
		return jujuerrors.NotValidf("empty DomainServicesName")
	}
	if cfg.HTTPClientName == "" {
		return jujuerrors.NotValidf("empty HTTPClientName")
	}
	if cfg.NewWorker == nil {
		return jujuerrors.NotValidf("nil NewWorker")
	}
	if cfg.NewHTTPClient == nil {
		return jujuerrors.NotValidf("nil NewHTTPClient")
	}
	if cfg.NewCharmhubClient == nil {
		return jujuerrors.NotValidf("nil NewCharmhubClient")
	}
	if cfg.Period <= 0 {
		return jujuerrors.NotValidf("invalid Period")
	}
	if !names.IsValidModel(cfg.ModelTag.Id()) {
		return jujuerrors.NotValidf("invalid ModelTag")
	}
	if cfg.Logger == nil {
		return jujuerrors.NotValidf("nil Logger")
	}
	if cfg.Clock == nil {
		return jujuerrors.NotValidf("nil Clock")
	}
	return nil
}

// Manifold returns a dependency.Manifold that runs a charm revision worker
// according to the supplied configuration.
func Manifold(cfg ManifoldConfig) dependency.Manifold {
	return dependency.Manifold{
		Inputs: []string{
			cfg.DomainServicesName,
			cfg.HTTPClientName,
		},
		Start: func(ctx context.Context, getter dependency.Getter) (worker.Worker, error) {
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

			worker, err := cfg.NewWorker(Config{
				ModelConfigService: domainServices.Config(),
				ApplicationService: domainServices.Application(),
				ModelService:       domainServices.ModelInfo(),
				ResourceService:    domainServices.Resource(),
				ModelTag:           cfg.ModelTag,
				HTTPClientGetter:   httpClientGetter,
				NewHTTPClient:      cfg.NewHTTPClient,
				NewCharmhubClient:  cfg.NewCharmhubClient,
				Clock:              cfg.Clock,
				Period:             cfg.Period,
				Logger:             cfg.Logger,
			})
			if err != nil {
				return nil, errors.Errorf("creating worker: %w", err)
			}
			return worker, nil
		},
		Filter: internalworker.ShouldWorkerUninstall,
	}
}

// NewHTTPClient creates a new HTTP client.
func NewHTTPClient(ctx context.Context, getter corehttp.HTTPClientGetter) (corehttp.HTTPClient, error) {
	return getter.GetHTTPClient(ctx, corehttp.CharmhubPurpose)
}

// NewCharmhubClient creates a new CharmhubClient.
func NewCharmhubClient(httpClient charmhub.HTTPClient, url string, logger logger.Logger) (CharmhubClient, error) {
	return charmhub.NewClient(charmhub.Config{
		URL:        url,
		Logger:     logger,
		HTTPClient: httpClient,
		FileSystem: charmhub.DefaultFileSystem(),
	})
}
