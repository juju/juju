// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package secretmigrationworker

import (
	"github.com/juju/clock"
	"github.com/juju/errors"
	"github.com/juju/worker/v3"
	"github.com/juju/worker/v3/dependency"

	"github.com/juju/juju/api/agent/secretsmanager"
	"github.com/juju/juju/api/base"
	jujusecrets "github.com/juju/juju/secrets"
)

// ManifoldConfig describes the resources used by the secretmigrationworker worker.
type ManifoldConfig struct {
	APICallerName string
	Logger        Logger
	Clock         clock.Clock

	NewFacade         func(base.APICaller) Facade
	NewWorker         func(Config) (worker.Worker, error)
	NewBackendsClient func(jujusecrets.JujuAPIClient) (jujusecrets.BackendsClient, error)
}

// NewClient returns a new secretmigrationworker facade.
func NewClient(caller base.APICaller) Facade {
	return secretsmanager.NewClient(caller)
}

func NewBackendsClient(facade jujusecrets.JujuAPIClient) (jujusecrets.BackendsClient, error) {
	return jujusecrets.NewClient(facade)
}

// Manifold returns a Manifold that encapsulates the secretmigrationworker worker.
func Manifold(config ManifoldConfig) dependency.Manifold {
	return dependency.Manifold{
		Inputs: []string{
			config.APICallerName,
		},
		Start: config.start,
	}
}

// Validate is called by start to check for bad configuration.
func (cfg ManifoldConfig) Validate() error {
	if cfg.APICallerName == "" {
		return errors.NotValidf("empty APICallerName")
	}
	if cfg.Logger == nil {
		return errors.NotValidf("nil Logger")
	}
	if cfg.Clock == nil {
		return errors.NotValidf("nil Clock")
	}
	return nil
}

// start is a StartFunc for a Worker manifold.
func (cfg ManifoldConfig) start(context dependency.Context) (worker.Worker, error) {
	if err := cfg.Validate(); err != nil {
		return nil, errors.Trace(err)
	}

	var apiCaller base.APICaller
	if err := context.Get(cfg.APICallerName, &apiCaller); err != nil {
		return nil, errors.Trace(err)
	}

	facade := cfg.NewFacade(apiCaller)
	worker, err := cfg.NewWorker(Config{
		Facade: facade,
		Logger: cfg.Logger,
		Clock:  cfg.Clock,
		SecretsBackendGetter: func() (jujusecrets.BackendsClient, error) {
			return cfg.NewBackendsClient(secretsmanager.NewClient(apiCaller))
		},
	})
	if err != nil {
		return nil, errors.Trace(err)
	}
	return worker, nil
}
