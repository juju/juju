// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package migrationimportreconciler

import (
	"context"

	"github.com/juju/clock"
	jujuerrors "github.com/juju/errors"
	"github.com/juju/worker/v5"
	"github.com/juju/worker/v5/dependency"

	coredatabase "github.com/juju/juju/core/database"
	"github.com/juju/juju/core/logger"
	coremodel "github.com/juju/juju/core/model"
	migrationservice "github.com/juju/juju/domain/modelmigration/service"
	migrationstate "github.com/juju/juju/domain/modelmigration/state/controller"
	"github.com/juju/juju/internal/errors"
	"github.com/juju/juju/internal/migration"
)

// ManifoldConfig defines the names of the manifolds on which a Manifold will
// depend.
type ManifoldConfig struct {
	// DBAccessorName is the name of the DB accessor manifold, from which the
	// reconciler obtains the controller database and the ability to drop model
	// databases.
	DBAccessorName string

	Clock     clock.Clock
	Logger    logger.Logger
	NewWorker func(Config) (worker.Worker, error)
}

// Validate checks the manifold configuration is complete.
func (cfg ManifoldConfig) Validate() error {
	if cfg.DBAccessorName == "" {
		return jujuerrors.NotValidf("empty DBAccessorName")
	}
	if cfg.Clock == nil {
		return jujuerrors.NotValidf("nil Clock")
	}
	if cfg.Logger == nil {
		return jujuerrors.NotValidf("nil Logger")
	}
	if cfg.NewWorker == nil {
		return jujuerrors.NotValidf("nil NewWorker")
	}
	return nil
}

// Manifold returns a dependency.Manifold that runs the migration import
// reconciler.
func Manifold(config ManifoldConfig) dependency.Manifold {
	return dependency.Manifold{
		Inputs: []string{
			config.DBAccessorName,
		},
		Start: config.start,
	}
}

func (config ManifoldConfig) start(ctx context.Context, getter dependency.Getter) (worker.Worker, error) {
	if err := config.Validate(); err != nil {
		return nil, errors.Capture(err)
	}

	var dbGetter coredatabase.DBGetter
	if err := getter.Get(config.DBAccessorName, &dbGetter); err != nil {
		return nil, errors.Capture(err)
	}

	// The reconciler works entirely on the controller database (import claims,
	// namespace registrations and staged model-database deletions). The database
	// drop itself is performed out of band by the undertaker's model-database
	// deleter. Build a controller-DB txn-runner factory from the accessor and
	// construct the controller-scoped modelmigration import service directly,
	// mirroring how the migration import path constructs its own controller
	// services; this avoids widening the controller domain services interface
	// for a single consumer.
	controllerDB := coredatabase.NewTxnRunnerFactoryForNamespace(dbGetter.GetDB, coredatabase.ControllerNS)
	service := migrationservice.NewImportService(
		migrationstate.New(controllerDB, config.Clock), config.Logger,
	)
	deps := migration.Deps{
		ControllerDB: controllerDB,
		Clock:        config.Clock,
		Logger:       config.Logger,
	}

	return config.NewWorker(Config{
		Service: service,
		Abort: func(ctx context.Context, modelUUID coremodel.UUID) error {
			return migration.AbortModelImport(ctx, deps, modelUUID)
		},
		Clock:  config.Clock,
		Logger: config.Logger,
	})
}
