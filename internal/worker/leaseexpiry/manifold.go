// Copyright 2022 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package leaseexpiry

import (
	"context"

	"github.com/juju/clock"
	"github.com/juju/errors"
	"github.com/juju/worker/v4"
	"github.com/juju/worker/v4/dependency"

	"github.com/juju/juju/core/database"
	"github.com/juju/juju/core/lease"
	coretrace "github.com/juju/juju/core/trace"
	"github.com/juju/juju/domain/lease/service"
	"github.com/juju/juju/domain/lease/state"
	"github.com/juju/juju/internal/worker/trace"
)

// Logger represents the methods used by the worker to log details.
type Logger interface {
	Infof(string, ...interface{})
	Debugf(string, ...interface{})
}

// ManifoldConfig holds the resources required
// to start the lease expiry worker.
type ManifoldConfig struct {
	ClockName      string
	DBAccessorName string
	TraceName      string

	Logger Logger

	NewWorker func(Config) (worker.Worker, error)
	NewStore  func(database.DBGetter, Logger) lease.ExpiryStore
}

// Validate checks that the config has all the required values.
func (c ManifoldConfig) Validate() error {
	if c.ClockName == "" {
		return errors.NotValidf("empty ClockName")
	}
	if c.DBAccessorName == "" {
		return errors.NotValidf("empty DBAccessorName")
	}
	if c.TraceName == "" {
		return errors.NotValidf("empty TraceName")
	}
	if c.Logger == nil {
		return errors.NotValidf("nil Logger")
	}
	if c.NewWorker == nil {
		return errors.NotValidf("nil NewWorker")
	}
	if c.NewStore == nil {
		return errors.NotValidf("nil NewStore")
	}
	return nil
}

func (c ManifoldConfig) start(ctx context.Context, getter dependency.Getter) (worker.Worker, error) {
	if err := c.Validate(); err != nil {
		return nil, errors.Trace(err)
	}

	var clk clock.Clock
	if err := getter.Get(c.ClockName, &clk); err != nil {
		return nil, errors.Trace(err)
	}

	var dbGetter database.DBGetter
	if err := getter.Get(c.DBAccessorName, &dbGetter); err != nil {
		return nil, errors.Trace(err)
	}

	var tracerGetter trace.TracerGetter
	if err := getter.Get(c.TraceName, &tracerGetter); err != nil {
		return nil, errors.Trace(err)
	}

	tracer, err := tracerGetter.GetTracer(context.TODO(), coretrace.Namespace("leaseexpiry", database.ControllerNS))
	if err != nil {
		tracer = coretrace.NoopTracer{}
	}

	store := c.NewStore(dbGetter, c.Logger)

	w, err := NewWorker(Config{
		Clock:  clk,
		Logger: c.Logger,
		Store:  store,
		Tracer: tracer,
	})
	if err != nil {
		return nil, errors.Trace(err)
	}
	return w, nil
}

// Manifold returns a dependency.Manifold that will
// run the lease expiry worker.
func Manifold(cfg ManifoldConfig) dependency.Manifold {
	return dependency.Manifold{
		Inputs: []string{
			cfg.ClockName,
			cfg.DBAccessorName,
			cfg.TraceName,
		},
		Start: cfg.start,
	}
}

// NewStore returns a new lease store based on the input config.
func NewStore(dbGetter database.DBGetter, logger Logger) lease.ExpiryStore {
	factory := database.NewTxnRunnerFactoryForNamespace(dbGetter.GetDB, database.ControllerNS)
	return service.NewService(state.NewState(factory, logger))
}
