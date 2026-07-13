// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package modeldbdeleter

import (
	"context"

	"github.com/juju/clock"
	jujuerrors "github.com/juju/errors"
	"github.com/juju/worker/v5"
	"github.com/juju/worker/v5/dependency"

	coredatabase "github.com/juju/juju/core/database"
	"github.com/juju/juju/core/logger"
	"github.com/juju/juju/core/watcher"
	"github.com/juju/juju/internal/errors"
	"github.com/juju/juju/internal/services"
)

// ModelDatabaseDeletionService provides the staged model database deletions
// recorded when a model is removed from this controller (currently by
// source-side migration REAP) while its dqlite database still exists.
type ModelDatabaseDeletionService interface {
	// WatchModelDatabaseDeletions returns a watcher that fires when model
	// database deletions are staged or removed.
	WatchModelDatabaseDeletions(ctx context.Context) (watcher.NotifyWatcher, error)

	// GetPendingModelDatabaseDeletions returns the dqlite namespaces staged
	// for deletion.
	GetPendingModelDatabaseDeletions(ctx context.Context) ([]string, error)

	// RemoveModelDatabaseDeletion removes the staged deletion for the given
	// namespace once its database has been deleted.
	RemoveModelDatabaseDeletion(ctx context.Context, namespace string) error
}

// GetModelDatabaseDeletionServiceFunc retrieves the deletion service from the
// dependency getter using the provided domain services name.
type GetModelDatabaseDeletionServiceFunc func(ctx context.Context, getter dependency.Getter, domainServicesName string) (ModelDatabaseDeletionService, error)

// ManifoldConfig holds the information necessary to run a model DB deleter
// worker in a dependency.Engine.
type ManifoldConfig struct {
	DBAccessorName     string
	DomainServicesName string
	Logger             logger.Logger
	Clock              clock.Clock
	NewWorker          func(Config) (worker.Worker, error)
	GetDeletionService GetModelDatabaseDeletionServiceFunc
}

// Validate validates the manifold configuration.
func (config ManifoldConfig) Validate() error {
	if config.DBAccessorName == "" {
		return jujuerrors.NotValidf("empty DBAccessorName")
	}
	if config.DomainServicesName == "" {
		return jujuerrors.NotValidf("empty DomainServicesName")
	}
	if config.GetDeletionService == nil {
		return jujuerrors.NotValidf("nil GetDeletionService")
	}
	if config.NewWorker == nil {
		return jujuerrors.NotValidf("nil NewWorker")
	}
	if config.Logger == nil {
		return jujuerrors.NotValidf("nil Logger")
	}
	if config.Clock == nil {
		return jujuerrors.NotValidf("nil Clock")
	}
	return nil
}

// Manifold returns a dependency.Manifold that will run a model DB deleter
// worker.
func Manifold(config ManifoldConfig) dependency.Manifold {
	return dependency.Manifold{
		Inputs: []string{
			config.DBAccessorName,
			config.DomainServicesName,
		},
		Start: config.start,
	}
}

// start is a method on ManifoldConfig because it's more readable than a closure.
func (config ManifoldConfig) start(ctx context.Context, getter dependency.Getter) (worker.Worker, error) {
	if err := config.Validate(); err != nil {
		return nil, errors.Capture(err)
	}

	// Take the database deleter directly from the dbaccessor worker so that
	// deleting a model database does not go through the domain services.
	var dbDeleter coredatabase.DBDeleter
	if err := getter.Get(config.DBAccessorName, &dbDeleter); err != nil {
		return nil, errors.Capture(err)
	}

	deletionService, err := config.GetDeletionService(ctx, getter, config.DomainServicesName)
	if err != nil {
		return nil, errors.Capture(err)
	}

	return config.NewWorker(Config{
		DBDeleter:       dbDeleter,
		DeletionService: deletionService,
		Logger:          config.Logger,
		Clock:           config.Clock,
	})
}

// GetModelDatabaseDeletionService retrieves the controller model service from
// the dependency getter using the provided domain services name.
func GetModelDatabaseDeletionService(ctx context.Context, getter dependency.Getter, domainServicesName string) (ModelDatabaseDeletionService, error) {
	var domainServices services.ControllerDomainServices
	if err := getter.Get(domainServicesName, &domainServices); err != nil {
		return nil, errors.Capture(err)
	}
	return domainServices.Model(), nil
}
