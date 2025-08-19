// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package apiremoterelationcaller

import (
	"context"

	"github.com/juju/clock"
	"github.com/juju/errors"
	"github.com/juju/worker/v4"
	"github.com/juju/worker/v4/dependency"

	"github.com/juju/juju/api"
	"github.com/juju/juju/core/logger"
	"github.com/juju/juju/core/model"
	"github.com/juju/juju/internal/services"
)

// APIRemoteCallerGetter is an interface that provides a method to get the
// remote API caller for a given model.
type APIRemoteCallerGetter interface {
	// GetConnectionForModel returns the remote API connection for the
	// specified model. The connection must be valid for the lifetime of the
	// returned RemoteConnection.
	GetConnectionForModel(ctx context.Context, modelUUID model.UUID) (api.Connection, error)
}

// NewWorkerFunc defines a function that creates a new Worker.
type NewWorkerFunc func(Config) (worker.Worker, error)

// ManifoldConfig defines the names of the manifolds on which a
// Worker manifold will depend.
type ManifoldConfig struct {
	DomainServicesName string

	NewWorker              NewWorkerFunc
	GetAPIInfoForModelFunc GetAPIInfoForModelFunc
	NewConnectionFunc      NewConnectionFunc
	Logger                 logger.Logger
	Clock                  clock.Clock
}

func (config ManifoldConfig) Validate() error {
	if config.DomainServicesName == "" {
		return errors.NotValidf("empty DomainServicesName")
	}
	if config.GetAPIInfoForModelFunc == nil {
		return errors.NotValidf("nil GetAPIInfoForModelFunc")
	}
	if config.NewConnectionFunc == nil {
		return errors.NotValidf("nil NewConnectionFunc")
	}
	if config.Logger == nil {
		return errors.NotValidf("nil Logger")
	}
	if config.Clock == nil {
		return errors.NotValidf("nil Clock")
	}
	return nil
}

// Manifold packages a Worker for use in a dependency.Engine.
func Manifold(config ManifoldConfig) dependency.Manifold {
	return dependency.Manifold{
		Inputs: []string{},
		Start: func(ctx context.Context, getter dependency.Getter) (worker.Worker, error) {
			if err := config.Validate(); err != nil {
				return nil, errors.Trace(err)
			}

			var domainServicesGetter services.DomainServicesGetter
			if err := getter.Get(config.DomainServicesName, &domainServicesGetter); err != nil {
				return nil, errors.Trace(err)
			}

			w, err := config.NewWorker(Config{
				GetAPIInfoForModel: config.GetAPIInfoForModelFunc,
				NewConnection:      config.NewConnectionFunc,
				Clock:              config.Clock,
				Logger:             config.Logger,
			})
			if err != nil {
				return nil, errors.Trace(err)
			}

			return w, nil
		},
		Output: remoteOutput,
	}
}

func remoteOutput(in worker.Worker, out any) error {
	w, ok := in.(*remoteWorker)
	if !ok {
		return errors.NotValidf("expected remoteWorker, got %T", in)
	}

	switch out := out.(type) {
	case *APIRemoteCallerGetter:
		*out = w
	default:
		return errors.NotValidf("expected *api.Connection, got %T", out)
	}
	return nil
}
