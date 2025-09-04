// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package operationpruner

import (
	"context"
	"time"

	"github.com/juju/clock"
	"github.com/juju/errors"
	"github.com/juju/worker/v4"
	"github.com/juju/worker/v4/dependency"

	"github.com/juju/juju/core/logger"
	"github.com/juju/juju/internal/services"
	internalworker "github.com/juju/juju/internal/worker"
)

// ManifoldConfig describes the resources used by the operation pruner worker.
type ManifoldConfig struct {
	DomainServicesName string
	Clock              clock.Clock
	Logger             logger.Logger
	// PruneInterval specifies how often the pruner should run.
	PruneInterval time.Duration
}

// Validate validates the manifold configuration.
func (config ManifoldConfig) Validate() error {
	if config.DomainServicesName == "" {
		return errors.NotValidf("empty DomainServicesName")
	}
	if config.Clock == nil {
		return errors.NotValidf("nil Clock")
	}
	if config.Logger == nil {
		return errors.NotValidf("nil Logger")
	}
	if config.PruneInterval <= 0 {
		return errors.NotValidf("non-positive PruneInterval")
	}
	return nil
}

// start starts the operation pruner worker.
func (config ManifoldConfig) start(ctx context.Context, getter dependency.Getter) (worker.Worker, error) {
	if err := config.Validate(); err != nil {
		return nil, errors.Trace(err)
	}

	var domainServices services.ModelDomainServices
	if err := getter.Get(config.DomainServicesName, &domainServices); err != nil {
		return nil, errors.Trace(err)
	}

	w, err := NewWorker(Config{
		Clock:            config.Clock,
		ModelConfig:      domainServices.Config(),
		OperationService: domainServices.Operation(),
		Logger:           config.Logger,
		PruneInterval:    config.PruneInterval,
	})
	if err != nil {
		return nil, errors.Trace(err)
	}
	return w, nil
}

// Manifold returns a Manifold that encapsulates the operation pruner worker.
func Manifold(config ManifoldConfig) dependency.Manifold {
	return dependency.Manifold{
		Inputs: []string{
			config.DomainServicesName,
		},
		Start:  config.start,
		Filter: internalworker.ShouldWorkerUninstall,
	}
}
