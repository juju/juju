// Copyright 2021 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package services

import (
	"context"
	"sync"

	"github.com/juju/errors"

	corecharm "github.com/juju/juju/core/charm"
	corelogger "github.com/juju/juju/core/logger"
	charmrepo "github.com/juju/juju/internal/charm/repository"
	"github.com/juju/juju/internal/charmhub"
)

// CharmRepoFactoryConfig encapsulates the information required for creating a
// new CharmRepoFactory instance.
type CharmRepoFactoryConfig struct {
	Logger corelogger.Logger

	// An HTTP client that is injected when making Charmhub API calls.
	CharmhubHTTPClient charmhub.HTTPClient

	ModelConfigService ModelConfigService
}

// CharmRepoFactory instantiates charm repositories. It memoizes created
// repositories allowing them to be reused by subsequent GetCharmRepository
// calls.
type CharmRepoFactory struct {
	logger             corelogger.Logger
	charmhubHTTPClient charmhub.HTTPClient
	modelConfigService ModelConfigService

	mu            sync.Mutex
	memoizedRepos map[corecharm.Source]corecharm.Repository
}

// NewCharmRepoFactory returns a new factory instance with the provided configuration.
func NewCharmRepoFactory(cfg CharmRepoFactoryConfig) *CharmRepoFactory {
	return &CharmRepoFactory{
		logger:             cfg.Logger,
		charmhubHTTPClient: cfg.CharmhubHTTPClient,
		modelConfigService: cfg.ModelConfigService,
		memoizedRepos:      make(map[corecharm.Source]corecharm.Repository),
	}
}

// GetCharmRepository returns a suitable corecharm.Repository instance for the
// requested source. Lookups are memoized for future requests.
func (f *CharmRepoFactory) GetCharmRepository(ctx context.Context, src corecharm.Source) (corecharm.Repository, error) {
	f.mu.Lock()
	defer f.mu.Unlock()

	if repo, isCached := f.memoizedRepos[src]; isCached {
		return repo, nil
	}

	var repo corecharm.Repository

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

		repo = charmrepo.NewCharmHubRepository(
			f.logger.Child("charmhubrepo", corelogger.CHARMHUB),
			chClient,
		)
	default:
		return nil, errors.NotSupportedf("charm repository for source %q", src)
	}

	// Memoize for future lookups.
	f.memoizedRepos[src] = repo
	return repo, nil
}
