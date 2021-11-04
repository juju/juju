// Copyright 2021 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package services

import (
	"sync"

	"github.com/juju/errors"
	"github.com/juju/loggo"

	"github.com/juju/juju/charmhub"
	corecharm "github.com/juju/juju/core/charm"
	charmrepo "github.com/juju/juju/core/charm/repository"
)

// CharmRepoFactoryConfig encapsulates the information required for creating a
// new CharmRepoFactory instance.
type CharmRepoFactoryConfig struct {
	// The logger to use.
	Logger loggo.Logger

	// A transport that is injected when making charmhub API calls.
	Transport charmhub.Transport

	StateBackend StateBackend
	ModelBackend ModelBackend
}

// CharmRepoFactory instantitates charm repositories. It memoizes created
// repositories allowing them to be reused by subsequent GetCharmRepository
// calls.
type CharmRepoFactory struct {
	logger            loggo.Logger
	charmhubTransport charmhub.Transport
	stateBackend      StateBackend
	modelBackend      ModelBackend

	mu            sync.Mutex
	memoizedRepos map[corecharm.Source]corecharm.Repository
}

// NewCharmRepoFactory returns a new factory instance with the provided configuration.
func NewCharmRepoFactory(cfg CharmRepoFactoryConfig) *CharmRepoFactory {
	return &CharmRepoFactory{
		logger:            cfg.Logger,
		charmhubTransport: cfg.Transport,
		stateBackend:      cfg.StateBackend,
		modelBackend:      cfg.ModelBackend,
		memoizedRepos:     make(map[corecharm.Source]corecharm.Repository),
	}
}

// GetCharmRepository returns a suitable corecharm.Repository instance for the
// requested source. Lookups are memoized for future requests.
func (f *CharmRepoFactory) GetCharmRepository(src corecharm.Source) (corecharm.Repository, error) {
	f.mu.Lock()
	defer f.mu.Unlock()

	if repo, isCached := f.memoizedRepos[src]; isCached {
		return repo, nil
	}

	var repo corecharm.Repository

	switch src {
	case corecharm.CharmStore:
		repo = charmrepo.NewCharmStoreRepository(
			f.logger.Child("charmstorerepo"),
		)

	case corecharm.CharmHub:
		cfg, err := f.modelBackend.Config()
		if err != nil {
			return nil, errors.Trace(err)
		}

		options := []charmhub.Option{
			charmhub.WithHTTPTransport(func(charmhub.Logger) charmhub.Transport {
				return f.charmhubTransport
			}),
		}

		var chCfg charmhub.Config
		chURL, ok := cfg.CharmHubURL()
		if ok {
			chCfg, err = charmhub.CharmHubConfigFromURL(chURL, f.logger, options...)
		} else {
			chCfg, err = charmhub.CharmHubConfig(f.logger, options...)
		}
		if err != nil {
			return nil, errors.Trace(err)
		}

		chClient, err := charmhub.NewClient(chCfg)
		if err != nil {
			return nil, errors.Trace(err)
		}

		repo = charmrepo.NewCharmHubRepository(
			f.logger.Child("charmhubrepo"),
			chClient,
		)
	default:
		return nil, errors.NotSupportedf("charm repository for source %q", src)
	}

	// Memoize for future lookups.
	f.memoizedRepos[src] = repo
	return repo, nil
}
