// Copyright 2021 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package services

import (
	"context"

	"github.com/juju/errors"
	"github.com/juju/loggo/v2"

	corecharm "github.com/juju/juju/core/charm"
	charmdownloader "github.com/juju/juju/core/charm/downloader"
	corelogger "github.com/juju/juju/core/logger"
	"github.com/juju/juju/internal/charmhub"
)

// CharmDownloaderConfig encapsulates the information required for creating a
// new CharmDownloader instance.
type CharmDownloaderConfig struct {
	// The logger to use.
	LoggerFactory LoggerFactory

	// An HTTP client that is injected when making Charmhub API calls.
	CharmhubHTTPClient charmhub.HTTPClient

	// ObjectStore provides access to the object store for a
	// given model.
	ObjectStore Storage

	StateBackend StateBackend
	ModelBackend ModelBackend
}

// NewCharmDownloader wires the provided configuration options into a new
// charmdownloader.Downloader instance.
func NewCharmDownloader(cfg CharmDownloaderConfig) (*charmdownloader.Downloader, error) {
	storage := NewCharmStorage(CharmStorageConfig{
		Logger:       cfg.LoggerFactory.Child("charmstorage"),
		StateBackend: cfg.StateBackend,
		ObjectStore:  cfg.ObjectStore,
	})

	repoFactory := repoFactoryShim{
		factory: NewCharmRepoFactory(CharmRepoFactoryConfig{
			LoggerFactory:      cfg.LoggerFactory.ForNamespace("charmrepofactory"),
			CharmhubHTTPClient: cfg.CharmhubHTTPClient,
			StateBackend:       cfg.StateBackend,
			ModelBackend:       cfg.ModelBackend,
		}),
	}

	return charmdownloader.NewDownloader(cfg.LoggerFactory.ChildWithLabels("charmdownloader", corelogger.CHARMHUB), storage, repoFactory), nil
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

type loggoLoggerFactory struct {
	Logger loggo.Logger
}

// LoggoLoggerFactory is a LoggerFactory that creates loggers using
// the loggo package.
func LoggoLoggerFactory(logger loggo.Logger) LoggerFactory {
	return loggoLoggerFactory{Logger: logger}
}

func (s loggoLoggerFactory) ForNamespace(name string) LoggerFactory {
	return LoggoLoggerFactory(s.Logger.Child(name))
}

func (s loggoLoggerFactory) Child(name string) charmhub.Logger {
	return s.Logger.Child(name)
}

func (s loggoLoggerFactory) ChildWithLabels(name string, labels ...string) charmhub.Logger {
	return s.Logger.ChildWithLabels(name, labels...)
}
