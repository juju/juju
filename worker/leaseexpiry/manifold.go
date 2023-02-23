// Copyright 2022 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package leaseexpiry

import (
	"github.com/juju/clock"
	"github.com/juju/errors"
	"github.com/juju/juju/worker/dbaccessor"
	"github.com/juju/worker/v3"
	"github.com/juju/worker/v3/dependency"
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

	Logger Logger

	NewWorker func(Config) (worker.Worker, error)
}

// Validate checks that the config has all the required values.
func (c ManifoldConfig) Validate() error {
	if c.ClockName == "" {
		return errors.NotValidf("empty ClockName")
	}
	if c.DBAccessorName == "" {
		return errors.NotValidf("empty DBAccessorName")
	}
	if c.Logger == nil {
		return errors.NotValidf("nil Logger")
	}
	if c.NewWorker == nil {
		return errors.NotValidf("nil NewWorker")
	}

	return nil
}

func (c ManifoldConfig) start(ctx dependency.Context) (worker.Worker, error) {
	if err := c.Validate(); err != nil {
		return nil, errors.Trace(err)
	}

	var clk clock.Clock
	if err := ctx.Get(c.ClockName, &clk); err != nil {
		return nil, errors.Trace(err)
	}

	var dbGetter dbaccessor.DBGetter
	if err := ctx.Get(c.DBAccessorName, &dbGetter); err != nil {
		return nil, errors.Trace(err)
	}

	db, err := dbGetter.GetDB("controller")
	if err != nil {
		return nil, errors.Trace(err)
	}

	w, err := NewWorker(Config{
		Clock:  clk,
		Logger: c.Logger,
		DB:     db,
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
		},
		Start: cfg.start,
	}
}
