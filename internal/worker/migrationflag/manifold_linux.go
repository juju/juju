//go:build dqlite

// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package migrationflag

import (
	"context"

	"github.com/juju/errors"
	"github.com/juju/worker/v5"
	"github.com/juju/worker/v5/dependency"

	"github.com/juju/juju/agent/engine"
	"github.com/juju/juju/core/migration"
	"github.com/juju/juju/core/watcher"
	modelmigrationservice "github.com/juju/juju/domain/modelmigration/service"
	"github.com/juju/juju/internal/services"
)

// ModelManifoldConfig holds the dependencies and configuration for a
// model-only Worker manifold backed by local domain services instead of
// an API caller.
type ModelManifoldConfig struct {
	DomainServicesName string
	ModelUUID          string
	Check              Predicate

	NewWorker func(context.Context, Config) (worker.Worker, error)
}

// validate is called by start to check for bad configuration.
func (config ModelManifoldConfig) validate() error {
	if config.DomainServicesName == "" {
		return errors.NotValidf("empty DomainServicesName")
	}
	if config.ModelUUID == "" {
		return errors.NotValidf("empty ModelUUID")
	}
	if config.Check == nil {
		return errors.NotValidf("nil Check")
	}
	if config.NewWorker == nil {
		return errors.NotValidf("nil NewWorker")
	}
	return nil
}

// start is a StartFunc for a model-only Worker manifold.
func (config ModelManifoldConfig) start(ctx context.Context, getter dependency.Getter) (worker.Worker, error) {
	if err := config.validate(); err != nil {
		return nil, errors.Trace(err)
	}
	var domainServices services.DomainServices
	if err := getter.Get(config.DomainServicesName, &domainServices); err != nil {
		return nil, errors.Trace(err)
	}
	facade := &domainServicesFacade{
		migrationSvc: domainServices.ModelMigration(),
	}
	w, err := config.NewWorker(ctx, Config{
		Facade: facade,
		Model:  config.ModelUUID,
		Check:  config.Check,
	})
	if err != nil {
		return nil, errors.Trace(err)
	}
	return w, nil
}

// ModelManifold packages a Worker for use in a dependency.Engine. It
// reads migration phase through local domain services rather than an API caller.
func ModelManifold(config ModelManifoldConfig) dependency.Manifold {
	return dependency.Manifold{
		Inputs: []string{config.DomainServicesName},
		Start:  config.start,
		Output: engine.FlagOutput,
		Filter: bounceErrChanged,
	}
}

// domainServicesFacade is a local facade adapter that satisfies the Facade
// interface by reading from domain services directly.
type domainServicesFacade struct {
	migrationSvc *modelmigrationservice.Service
}

// Phase is part of the Facade interface.
func (f *domainServicesFacade) Phase(ctx context.Context, _ string) (migration.Phase, error) {
	mig, err := f.migrationSvc.Migration(ctx)
	if err != nil {
		return 0, errors.Trace(err)
	}
	return mig.Phase, nil
}

// Watch is part of the Facade interface.
func (f *domainServicesFacade) Watch(ctx context.Context, _ string) (watcher.NotifyWatcher, error) {
	return f.migrationSvc.WatchMigrationPhase(ctx)
}
