// Copyright 2021 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package services

import (
	"context"

	"github.com/juju/errors"

	corecharm "github.com/juju/juju/core/charm"
	corelogger "github.com/juju/juju/core/logger"
	charmrepo "github.com/juju/juju/internal/charm/repository"
	"github.com/juju/juju/internal/charmhub"
)

// CharmRepoFactoryConfig encapsulates the information required for creating a
// new CharmRepoFactory instance.
type CharmRepoFactoryConfig struct {
	// An HTTP client that is injected when making Charmhub API calls.
	CharmhubHTTPClient charmhub.HTTPClient

	// ModelConfigService provides access to the model configuration.
	ModelConfigService ModelConfigService

	Logger corelogger.Logger
}

// CharmRepoFactory instantiates charm repositories. It memoizes created
// repositories allowing them to be reused by subsequent GetCharmRepository
// calls.
type CharmRepoFactory struct {
	logger             corelogger.Logger
	charmhubHTTPClient charmhub.HTTPClient
	modelConfigService ModelConfigService
}

// NewCharmRepoFactory returns a new factory instance with the provided configuration.
func NewCharmRepoFactory(cfg CharmRepoFactoryConfig) *CharmRepoFactory {
	return &CharmRepoFactory{
		logger:             cfg.Logger,
		charmhubHTTPClient: cfg.CharmhubHTTPClient,
		modelConfigService: cfg.ModelConfigService,
	}
}

// GetCharmRepository returns a suitable corecharm.Repository instance for the
// requested source. Lookups are memoized for future requests.
func (f *CharmRepoFactory) GetCharmRepository(ctx context.Context, src corecharm.Source) (corecharm.Repository, error) {
	switch src {
	case corecharm.CharmHub:
		cfg, err := f.modelConfigService.ModelConfig(ctx)
		if err != nil {
			return nil, errors.Trace(err)
		}

		chURL, _ := cfg.CharmHubURL()
		chClient, err := charmhub.NewClient(charmhub.Config{
			URL:        chURL,
			HTTPClient: f.charmhubHTTPClient,
			Logger:     f.logger.Child("charmhub"),
		})
		if err != nil {
			return nil, errors.Trace(err)
		}

		return charmrepo.NewCharmHubRepository(
			f.logger.Child("charmhubrepo", corelogger.CHARMHUB),
			chClient,
		), nil

	default:
		return nil, errors.NotSupportedf("charm repository for source %q", src)
	}
}
