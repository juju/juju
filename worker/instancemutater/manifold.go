// Copyright 2019 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package instancemutater

import (
	"github.com/juju/errors"
	worker "gopkg.in/juju/worker.v1"
	"gopkg.in/juju/worker.v1/dependency"

	"github.com/juju/juju/api/base"
	"github.com/juju/juju/environs"
)

//go:generate mockgen -package mocks -destination mocks/worker_mock.go gopkg.in/juju/worker.v1 Worker
//go:generate mockgen -package mocks -destination mocks/dependency_mock.go gopkg.in/juju/worker.v1/dependency Context
//go:generate mockgen -package mocks -destination mocks/environs_mock.go github.com/juju/juju/environs Environ
//go:generate mockgen -package mocks -destination mocks/base_mock.go github.com/juju/juju/api/base APICaller

// ManifoldConfig describes the resources used by the instancemuter worker.
type ManifoldConfig struct {
	APICallerName string
	EnvironName   string

	Logger    Logger
	NewWorker func(Config) (worker.Worker, error)
}

// Validate validates the manifold configuration.
func (config ManifoldConfig) Validate() error {
	if config.Logger == nil {
		return errors.NotValidf("nil Logger")
	}
	if config.NewWorker == nil {
		return errors.NotValidf("nil NewWorker")
	}
	return nil
}

func (config ManifoldConfig) newWorker(environ environs.Environ, apiCaller base.APICaller) (worker.Worker, error) {
	if err := config.Validate(); err != nil {
		return nil, errors.Trace(err)
	}

	cfg := Config{
		Logger: config.Logger,
	}

	w, err := config.NewWorker(cfg)
	if err != nil {
		return nil, errors.Annotate(err, "cannot start machine upgrade series worker")
	}
	return w, nil
}

// Manifold returns a Manifold that encapsulates the instancepoller worker.
func Manifold(config ManifoldConfig) dependency.Manifold {
	typedConfig := EnvironAPIConfig{
		EnvironName:   config.EnvironName,
		APICallerName: config.APICallerName,
	}
	return EnvironAPIManifold(typedConfig, config.newWorker)
}

// EnvironAPIConfig represents a typed manifold starter func, that handles
// getting resources from the configuration.
type EnvironAPIConfig struct {
	EnvironName   string
	APICallerName string
}

// EnvironAPIStartFunc encapsulates creation of a worker based on the environ
// and APICaller.
type EnvironAPIStartFunc func(environs.Environ, base.APICaller) (worker.Worker, error)

// EnvironAPIManifold returns a dependency.Manifold that calls the supplied
// start func with the API and envrion resources defined in the config
// (once those resources are present).
func EnvironAPIManifold(config EnvironAPIConfig, start EnvironAPIStartFunc) dependency.Manifold {
	return dependency.Manifold{
		Inputs: []string{
			config.EnvironName,
			config.APICallerName,
		},
		Start: func(context dependency.Context) (worker.Worker, error) {
			var environ environs.Environ
			if err := context.Get(config.EnvironName, &environ); err != nil {
				return nil, errors.Trace(err)
			}
			var apiCaller base.APICaller
			if err := context.Get(config.APICallerName, &apiCaller); err != nil {
				return nil, errors.Trace(err)
			}
			return start(environ, apiCaller)
		},
	}
}
