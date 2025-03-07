// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package secretspruner

import (
	"github.com/juju/errors"
	"github.com/juju/worker/v3"
	"github.com/juju/worker/v3/dependency"

	"github.com/juju/juju/api/base"
	"github.com/juju/juju/api/controller/usersecrets"
)

// ManifoldConfig describes the resources used by the secretspruner worker.
type ManifoldConfig struct {
	APICallerName string
	Logger        Logger

	NewUserSecretsFacade func(base.APICaller) SecretsFacade
	NewWorker            func(Config) (worker.Worker, error)
}

// NewUserSecretsFacade returns a new SecretsFacade.
func NewUserSecretsFacade(caller base.APICaller) SecretsFacade {
	return usersecrets.NewClient(caller)
}

// Manifold returns a Manifold that encapsulates the secretspruner worker.
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
	if cfg.NewUserSecretsFacade == nil {
		return errors.NotValidf("nil NewUserSecretsFacade")
	}
	if cfg.NewWorker == nil {
		return errors.NotValidf("nil NewWorker")
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

	worker, err := cfg.NewWorker(Config{
		SecretsFacade: cfg.NewUserSecretsFacade(apiCaller),
		Logger:        cfg.Logger,
	})
	if err != nil {
		return nil, errors.Trace(err)
	}
	return worker, nil
}
