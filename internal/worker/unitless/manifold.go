// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package unitless

import (
	"context"

	"github.com/juju/clock"
	"github.com/juju/worker/v5"
	"github.com/juju/worker/v5/dependency"

	coreerrors "github.com/juju/juju/core/errors"
	"github.com/juju/juju/core/logger"
	"github.com/juju/juju/internal/errors"
	"github.com/juju/juju/internal/services"
)

// ManifoldConfig contains the configuration used to start the unitless
// worker from a dependency engine.
type ManifoldConfig struct {
	// DomainServicesName is the name of the model domain services dependency.
	DomainServicesName string

	// GetScriptletService extracts the scriptlet service from model domain
	// services.
	GetScriptletService func(dependency.Getter, string) (ScriptletService, error)

	// NewWorker creates the unitless worker.
	NewWorker func(Config) (worker.Worker, error)

	// Clock supplies timing services to the worker.
	Clock clock.Clock

	// Logger logs worker output.
	Logger logger.Logger
}

// Validate checks that the manifold can start a worker.
func (config ManifoldConfig) Validate() error {
	if config.DomainServicesName == "" {
		return errors.New("empty DomainServicesName not valid").Add(coreerrors.NotValid)
	}
	if config.GetScriptletService == nil {
		return errors.New("nil GetScriptletService not valid").Add(coreerrors.NotValid)
	}
	if config.NewWorker == nil {
		return errors.New("nil NewWorker not valid").Add(coreerrors.NotValid)
	}
	if config.Clock == nil {
		return errors.New("nil Clock not valid").Add(coreerrors.NotValid)
	}
	if config.Logger == nil {
		return errors.New("nil Logger not valid").Add(coreerrors.NotValid)
	}
	return nil
}

// Manifold returns a dependency manifold that runs the unitless worker.
func Manifold(config ManifoldConfig) dependency.Manifold {
	return dependency.Manifold{
		Inputs: []string{
			config.DomainServicesName,
		},
		Start: config.start,
	}
}

func (config ManifoldConfig) start(_ context.Context, getter dependency.Getter) (worker.Worker, error) {
	if err := config.Validate(); err != nil {
		return nil, errors.Capture(err)
	}

	scriptletService, err := config.GetScriptletService(getter, config.DomainServicesName)
	if err != nil {
		return nil, errors.Capture(err)
	}

	w, err := config.NewWorker(Config{
		ScriptletService: scriptletService,
		NewExecutor:      NewStarformExecutor,
		Clock:            config.Clock,
		MaxAllocs:        defaultMaxAllocs,
		MaxSteps:         defaultMaxSteps,
		Logger:           config.Logger,
	})
	if err != nil {
		return nil, errors.Errorf("creating unitless worker: %w", err)
	}
	return w, nil
}

// GetScriptletService extracts the unitless service from model domain
// services.
func GetScriptletService(getter dependency.Getter, name string) (ScriptletService, error) {
	var domainServices services.ModelDomainServices
	if err := getter.Get(name, &domainServices); err != nil {
		return nil, errors.Capture(err)
	}

	return domainServices.Unitless(), nil
}
