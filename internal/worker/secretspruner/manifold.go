// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package secretspruner

import (
	"context"

	"github.com/juju/errors"
	"github.com/juju/worker/v5"
	"github.com/juju/worker/v5/dependency"

	"github.com/juju/juju/core/logger"
	"github.com/juju/juju/core/watcher"
	"github.com/juju/juju/internal/services"
)

// ManifoldConfig describes the resources used by the secretspruner worker.
type ManifoldConfig struct {
	DomainServicesName string
	Logger             logger.Logger

	NewWorker func(Config) (worker.Worker, error)
}

// Manifold returns a Manifold that encapsulates the secretspruner worker.
func Manifold(config ManifoldConfig) dependency.Manifold {
	return dependency.Manifold{
		Inputs: []string{
			config.DomainServicesName,
		},
		Start: config.start,
	}
}

// Validate is called by start to check for bad configuration.
func (cfg ManifoldConfig) Validate() error {
	if cfg.DomainServicesName == "" {
		return errors.NotValidf("empty DomainServicesName")
	}
	if cfg.Logger == nil {
		return errors.NotValidf("nil Logger")
	}
	if cfg.NewWorker == nil {
		return errors.NotValidf("nil NewWorker")
	}
	return nil
}

// start is a StartFunc for a Worker manifold.
func (cfg ManifoldConfig) start(context context.Context, getter dependency.Getter) (worker.Worker, error) {
	if err := cfg.Validate(); err != nil {
		return nil, errors.Trace(err)
	}

	var domainServices services.DomainServices
	if err := getter.Get(cfg.DomainServicesName, &domainServices); err != nil {
		return nil, errors.Trace(err)
	}

	facade := &secretServiceAdapter{
		secretSvc: domainServices.Secret(),
	}

	worker, err := cfg.NewWorker(Config{
		SecretsFacade: facade,
		Logger:        cfg.Logger,
	})
	if err != nil {
		return nil, errors.Trace(err)
	}
	return worker, nil
}

// secretServiceAdapter satisfies the SecretsFacade interface by reading
// from the domain secret service directly.
type secretServiceAdapter struct {
	secretSvc secretService
}

// secretService is the subset of the domain secret service needed by the
// secrets-pruner facade adapter.
type secretService interface {
	WatchObsoleteUserSecretsToPrune(context.Context) (watcher.NotifyWatcher, error)
	DeleteObsoleteUserSecretRevisions(context.Context) error
}

// WatchRevisionsToPrune is part of the SecretsFacade interface.
func (a *secretServiceAdapter) WatchRevisionsToPrune(ctx context.Context) (watcher.NotifyWatcher, error) {
	return a.secretSvc.WatchObsoleteUserSecretsToPrune(ctx)
}

// DeleteObsoleteUserSecretRevisions is part of the SecretsFacade interface.
func (a *secretServiceAdapter) DeleteObsoleteUserSecretRevisions(ctx context.Context) error {
	return a.secretSvc.DeleteObsoleteUserSecretRevisions(ctx)
}
