// Copyright 2021 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package services

import (
	"github.com/juju/errors"
	"github.com/juju/loggo"

	"github.com/juju/juju/charmhub"
	corecharm "github.com/juju/juju/core/charm"
	charmdownloader "github.com/juju/juju/core/charm/downloader"
)

// CharmDownloaderConfig encapsulates the information required for creating a
// new CharmDownloader instance.
type CharmDownloaderConfig struct {
	// The logger to use.
	Logger loggo.Logger

	// A transport that is injected when making charmhub API calls.
	Transport charmhub.Transport

	// A factory for accessing model-scoped storage for charm blobs.
	StorageFactory func(modelUUID string) Storage

	StateBackend StateBackend
	ModelBackend ModelBackend
}

// NewCharmDownloader wires the provided configuration options into a new
// charmdownloader.Downloader instance.
func NewCharmDownloader(cfg CharmDownloaderConfig) (*charmdownloader.Downloader, error) {
	storage := NewCharmStorage(CharmStorageConfig{
		Logger:         cfg.Logger.Child("charmstorage"),
		StateBackend:   cfg.StateBackend,
		StorageFactory: cfg.StorageFactory,
	})

	repoFactory := repoFactoryShim{
		factory: NewCharmRepoFactory(CharmRepoFactoryConfig{
			Logger:       cfg.Logger.Child("charmrepofactory"),
			Transport:    cfg.Transport,
			StateBackend: cfg.StateBackend,
			ModelBackend: cfg.ModelBackend,
		}),
	}

	return charmdownloader.NewDownloader(cfg.Logger.Child("charmdownloader"), storage, repoFactory), nil
}

// repoFactoryShim wraps a CharmRepoFactory and is compatible with the
// charmdownloader.RepositoryGetter interface.
type repoFactoryShim struct {
	factory *CharmRepoFactory
}

// GetCharmRepository implements charmdownloader.RepositoryGetter.
func (s repoFactoryShim) GetCharmRepository(src corecharm.Source) (charmdownloader.CharmRepository, error) {
	repo, err := s.factory.GetCharmRepository(src)
	if err != nil {
		return nil, errors.Trace(err)
	}

	return repo, err
}
