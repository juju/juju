// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package migrationreconciler

import (
	"context"

	"github.com/juju/clock"
	jujuerrors "github.com/juju/errors"
	"github.com/juju/worker/v5"
	"github.com/juju/worker/v5/dependency"

	coredatabase "github.com/juju/juju/core/database"
	"github.com/juju/juju/core/logger"
	coremodel "github.com/juju/juju/core/model"
	"github.com/juju/juju/internal/errors"
	"github.com/juju/juju/internal/migration"
	"github.com/juju/juju/internal/services"
)

// ManifoldConfig defines the names of the manifolds on which a Manifold will
// depend.
type ManifoldConfig struct {
	// DBAccessorName is the name of the DB accessor manifold, from which the
	// reconciler obtains the controller database used to undo the import writes
	// during abort compensation.
	DBAccessorName string

	// DomainServicesName is the domain services manifold, from which the
	// reconciler obtains the controller-scoped import claim service (with its
	// watcher) and a per-model domain services getter used to complete
	// interrupted activations (which write the model database).
	DomainServicesName string

	Clock     clock.Clock
	Logger    logger.Logger
	NewWorker func(Config) (worker.Worker, error)
}

// Validate checks the manifold configuration is complete.
func (cfg ManifoldConfig) Validate() error {
	if cfg.DBAccessorName == "" {
		return jujuerrors.NotValidf("empty DBAccessorName")
	}
	if cfg.DomainServicesName == "" {
		return jujuerrors.NotValidf("empty DomainServicesName")
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

// Manifold returns a dependency.Manifold that runs the migration reconciler.
func Manifold(config ManifoldConfig) dependency.Manifold {
	return dependency.Manifold{
		Inputs: []string{
			config.DBAccessorName,
			config.DomainServicesName,
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
	var controllerDomainServices services.ControllerDomainServices
	if err := getter.Get(config.DomainServicesName, &controllerDomainServices); err != nil {
		return nil, errors.Capture(err)
	}
	var domainServicesGetter services.DomainServicesGetter
	if err := getter.Get(config.DomainServicesName, &domainServicesGetter); err != nil {
		return nil, errors.Capture(err)
	}

	// The import claim service (with its watcher) comes from the controller
	// domain services. Abort compensation additionally undoes the raw
	// controller-database import writes, which the claim service cannot express,
	// so it needs a direct controller-DB txn-runner factory of its own.
	service := controllerDomainServices.ModelMigrationImport()
	controllerDB := coredatabase.NewTxnRunnerFactoryForNamespace(dbGetter.GetDB, coredatabase.ControllerNS)
	deps := migration.Deps{
		ControllerDB: controllerDB,
		Clock:        config.Clock,
		Logger:       config.Logger,
	}

	return config.NewWorker(Config{
		Service: service,
		Abort: func(ctx context.Context, modelUUID coremodel.UUID) error {
			return migration.AbortModelImport(ctx, deps, service.Service, modelUUID)
		},
		Activate: func(ctx context.Context, modelUUID coremodel.UUID) error {
			return migration.CompleteActivation(ctx, domainServicesGetter, modelUUID)
		},
		Clock:  config.Clock,
		Logger: config.Logger,
	})
}
