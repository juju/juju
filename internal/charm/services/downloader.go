// Copyright 2021 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package services

import (
	"context"

	"github.com/juju/errors"

	corecharm "github.com/juju/juju/core/charm"
	corelogger "github.com/juju/juju/core/logger"
	charmdownloader "github.com/juju/juju/internal/charm/downloader"
	"github.com/juju/juju/internal/charmhub"
)

// CharmDownloaderConfig encapsulates the information required for creating a
// new CharmDownloader instance.
type CharmDownloaderConfig struct {
	Logger corelogger.Logger

	// An HTTP client that is injected when making Charmhub API calls.
	CharmhubHTTPClient charmhub.HTTPClient

	// ObjectStore provides access to the object store for a
	// given model.
	ObjectStore Storage

	StateBackend       StateBackend
	ModelConfigService ModelConfigService
}

// NewCharmDownloader wires the provided configuration options into a new
// charmdownloader.Downloader instance.
func NewCharmDownloader(cfg CharmDownloaderConfig) (*charmdownloader.Downloader, error) {
	storage := NewCharmStorage(CharmStorageConfig{
		Logger:       cfg.Logger.Child("charmstorage"),
		StateBackend: cfg.StateBackend,
		ObjectStore:  cfg.ObjectStore,
	})

	repoFactory := repoFactoryShim{
		factory: NewCharmRepoFactory(CharmRepoFactoryConfig{
			Logger:             cfg.Logger.Child("charmrepofactory"),
			CharmhubHTTPClient: cfg.CharmhubHTTPClient,
			ModelConfigService: cfg.ModelConfigService,
		}),
	}

	return charmdownloader.NewDownloader(cfg.Logger.Child("charmdownloader", corelogger.CHARMHUB), storage, repoFactory), nil
}

// repoFactoryShim wraps a CharmRepoFactory and is compatible with the
// charmdownloader.RepositoryGetter interface.
type repoFactoryShim struct {
	factory *CharmRepoFactory
}

// GetCharmRepository implements charmdownloader.RepositoryGetter.
func (s repoFactoryShim) GetCharmRepository(ctx context.Context, src corecharm.Source) (charmdownloader.CharmRepository, error) {
	repo, err := s.factory.GetCharmRepository(ctx, src)
	if err != nil {
		return nil, errors.Trace(err)
	}

	return repo, err
}
