//go:build !dqlite

// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package secretsdrainworker

import (
	"context"

	"github.com/juju/errors"
	"github.com/juju/worker/v5"
	"github.com/juju/worker/v5/dependency"
	"gopkg.in/tomb.v2"

	"github.com/juju/juju/core/logger"
)

// ModelManifoldConfig holds the dependencies and configuration for a
// model-only Worker manifold backed by local domain services instead of
// an API caller.
type ModelManifoldConfig struct {
	DomainServicesName string
	ModelUUID          string
	Logger             logger.Logger

	NewWorker func(Config) (worker.Worker, error)
}

// Validate is called by start to check for bad configuration.
func (cfg ModelManifoldConfig) validate() error {
	if cfg.DomainServicesName == "" {
		return errors.NotValidf("empty DomainServicesName")
	}
	if cfg.ModelUUID == "" {
		return errors.NotValidf("empty ModelUUID")
	}
	if cfg.Logger == nil {
		return errors.NotValidf("nil Logger")
	}
	if cfg.NewWorker == nil {
		return errors.NotValidf("nil NewWorker")
	}
	return nil
}

// ModelManifold packages a Worker for use in a dependency.Engine. It reads
// secret drain data through local domain services rather than an API caller.
func ModelManifold(config ModelManifoldConfig) dependency.Manifold {
	return dependency.Manifold{
		Inputs: []string{config.DomainServicesName},
		Start: func(ctx context.Context, getter dependency.Getter) (worker.Worker, error) {
			if err := config.validate(); err != nil {
				return nil, errors.Trace(err)
			}
			return newNoopWorker(), nil
		},
	}
}

// NoopWorker ensures that we get a functioning tracer even if we're not using
// it.
type noopWorker struct {
	tomb tomb.Tomb
}

// NewNoopWorker worker creates a worker that doesn't perform any new work on
// the context. Though it will manage the lifecycle of the worker.
func newNoopWorker() *noopWorker {
	// Set this up, so we only ever hand out a singular tracer and span per
	// worker.
	w := &noopWorker{}

	w.tomb.Go(func() error {
		<-w.tomb.Dying()
		return tomb.ErrDying
	})

	return w
}

// Kill is part of the worker.Worker interface.
func (w *noopWorker) Kill() {
	w.tomb.Kill(nil)
}

// Wait is part of the worker.Worker interface.
func (w *noopWorker) Wait() error {
	return w.tomb.Wait()
}
