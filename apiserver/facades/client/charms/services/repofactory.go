// Copyright 2021 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package services

import (
	"context"
	"sync"

	"github.com/juju/errors"

	corecharm "github.com/juju/juju/core/charm"
	charmrepo "github.com/juju/juju/core/charm/repository"
	corelogger "github.com/juju/juju/core/logger"
	"github.com/juju/juju/internal/charmhub"
)

// LoggerFactory is the interface that is used to create loggers.
type LoggerFactory interface {
	charmhub.LoggerFactory

	// ForNamespace returns a new logger with the provided namespace. This
	// provides a child logger factory that can be used to create loggers
	// with a common namespace prefix.
	ForNamespace(string) LoggerFactory
}

// CharmRepoFactoryConfig encapsulates the information required for creating a
// new CharmRepoFactory instance.
type CharmRepoFactoryConfig struct {
	// LoggerFactory is the logger factory to use when creating new loggers.
	LoggerFactory LoggerFactory

	// An HTTP client that is injected when making Charmhub API calls.
	CharmhubHTTPClient charmhub.HTTPClient

	StateBackend StateBackend
	ModelBackend ModelBackend
}

// CharmRepoFactory instantiates charm repositories. It memoizes created
// repositories allowing them to be reused by subsequent GetCharmRepository
// calls.
type CharmRepoFactory struct {
	loggerFactory      LoggerFactory
	logger             charmhub.Logger
	charmhubHTTPClient charmhub.HTTPClient
	stateBackend       StateBackend
	modelBackend       ModelBackend

	mu            sync.Mutex
	memoizedRepos map[corecharm.Source]corecharm.Repository
}

// NewCharmRepoFactory returns a new factory instance with the provided configuration.
func NewCharmRepoFactory(cfg CharmRepoFactoryConfig) *CharmRepoFactory {
	return &CharmRepoFactory{
		loggerFactory:      cfg.LoggerFactory,
		charmhubHTTPClient: cfg.CharmhubHTTPClient,
		stateBackend:       cfg.StateBackend,
		modelBackend:       cfg.ModelBackend,
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
		cfg, err := f.modelBackend.Config()
		if err != nil {
			return nil, errors.Trace(err)
		}
		chURL, _ := cfg.CharmHubURL()
		chClient, err := charmhub.NewClient(charmhub.Config{
			URL:           chURL,
			HTTPClient:    f.charmhubHTTPClient,
			LoggerFactory: f.loggerFactory.ForNamespace("charmhub"),
		})
		if err != nil {
			return nil, errors.Trace(err)
		}

		repo = charmrepo.NewCharmHubRepository(
			f.loggerFactory.ChildWithLabels("charmhubrepo", corelogger.CHARMHUB),
			chClient,
		)
	default:
		return nil, errors.NotSupportedf("charm repository for source %q", src)
	}

	// Memoize for future lookups.
	f.memoizedRepos[src] = repo
	return repo, nil
}
