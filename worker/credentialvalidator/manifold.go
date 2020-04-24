// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package credentialvalidator

import (
	"github.com/juju/errors"
	"github.com/juju/worker/v2"
	"github.com/juju/worker/v2/dependency"

	"github.com/juju/juju/api/base"
	"github.com/juju/juju/cmd/jujud/agent/engine"
)

// Logger represents the methods used by the worker to log details.
type Logger interface {
	Debugf(string, ...interface{})
	Infof(string, ...interface{})
}

// ManifoldConfig holds the dependencies and configuration for a
// Worker manifold.
type ManifoldConfig struct {
	APICallerName string

	NewFacade func(base.APICaller) (Facade, error)
	NewWorker func(Config) (worker.Worker, error)
	Logger    Logger
}

// Validate is called by start to check for bad configuration.
func (config ManifoldConfig) Validate() error {
	if config.APICallerName == "" {
		return errors.NotValidf("empty APICallerName")
	}
	if config.NewFacade == nil {
		return errors.NotValidf("nil NewFacade")
	}
	if config.NewWorker == nil {
		return errors.NotValidf("nil NewWorker")
	}
	if config.Logger == nil {
		return errors.NotValidf("nil Logger")
	}
	return nil
}

// start is a StartFunc for a Worker manifold.
func (config ManifoldConfig) start(context dependency.Context) (worker.Worker, error) {
	if err := config.Validate(); err != nil {
		return nil, errors.Trace(err)
	}
	var apiCaller base.APICaller
	if err := context.Get(config.APICallerName, &apiCaller); err != nil {
		return nil, errors.Trace(err)
	}
	facade, err := config.NewFacade(apiCaller)
	if err != nil {
		return nil, errors.Trace(err)
	}
	w, err := config.NewWorker(Config{
		Facade: facade,
		Logger: config.Logger,
	})
	if err != nil {
		return nil, errors.Trace(err)
	}
	return w, nil
}

// Manifold packages a Worker for use in a dependency.Engine.
func Manifold(config ManifoldConfig) dependency.Manifold {
	return dependency.Manifold{
		Inputs: []string{config.APICallerName},
		Start:  config.start,
		Output: engine.FlagOutput,
		Filter: filterErrors,
	}
}

func filterErrors(err error) error {
	cause := errors.Cause(err)
	if cause == ErrValidityChanged ||
		cause == ErrModelCredentialChanged {
		return dependency.ErrBounce
	}
	return err
}
