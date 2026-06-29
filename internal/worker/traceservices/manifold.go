// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package traceservices

import (
	"context"

	"github.com/juju/errors"
	"github.com/juju/worker/v5"
	"github.com/juju/worker/v5/dependency"

	"github.com/juju/juju/core/changestream"
	coredatabase "github.com/juju/juju/core/database"
	"github.com/juju/juju/core/logger"
	domainservicefactory "github.com/juju/juju/domain/services"
	"github.com/juju/juju/internal/services"
	"github.com/juju/juju/internal/worker/common"
)

// ManifoldConfig holds the information necessary to run a trace services
// worker in a dependency.Engine.
type ManifoldConfig struct {
	ChangeStreamName string
	Logger           logger.Logger
	NewWorker        func(Config) (worker.Worker, error)
	NewTraceServices TraceServicesFn
}

// TraceServicesFn is a function that returns trace services.
type TraceServicesFn func(
	changestream.WatchableDBGetter,
	logger.Logger,
) services.TraceServices

// Validate validates the manifold configuration.
func (config ManifoldConfig) Validate() error {
	if config.ChangeStreamName == "" {
		return errors.NotValidf("empty ChangeStreamName")
	}
	if config.Logger == nil {
		return errors.NotValidf("nil Logger")
	}
	if config.NewWorker == nil {
		return errors.NotValidf("nil NewWorker")
	}
	if config.NewTraceServices == nil {
		return errors.NotValidf("nil NewTraceServices")
	}
	return nil
}

// Manifold returns a dependency.Manifold that will run a trace services worker.
func Manifold(config ManifoldConfig) dependency.Manifold {
	return dependency.Manifold{
		Inputs: []string{
			config.ChangeStreamName,
		},
		Start:  config.start,
		Output: config.output,
	}
}

func (config ManifoldConfig) start(context context.Context, getter dependency.Getter) (worker.Worker, error) {
	if err := config.Validate(); err != nil {
		return nil, errors.Trace(err)
	}

	var dbGetter changestream.WatchableDBGetter
	if err := getter.Get(config.ChangeStreamName, &dbGetter); err != nil {
		return nil, errors.Trace(err)
	}

	return config.NewWorker(Config{
		DBGetter:         dbGetter,
		Logger:           config.Logger,
		NewTraceServices: config.NewTraceServices,
	})
}

func (config ManifoldConfig) output(in worker.Worker, out any) error {
	if w, ok := in.(*common.CleanupWorker); ok {
		in = w.Worker
	}
	w, ok := in.(*servicesWorker)
	if !ok {
		return errors.Errorf("expected input of type dbWorker, got %T", in)
	}

	switch out := out.(type) {
	case *services.TraceServices:
		*out = w.Services()

	default:
		return errors.Errorf("unsupported output type %T", out)
	}
	return nil
}

// NewTraceServices returns a new trace services.
func NewTraceServices(
	dbGetter changestream.WatchableDBGetter,
	logger logger.Logger,
) services.TraceServices {
	return domainservicefactory.NewTraceServices(
		changestream.NewWatchableDBFactoryForNamespace(dbGetter.GetWatchableDB, coredatabase.ControllerNS),
		logger,
	)
}
