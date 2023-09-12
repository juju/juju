// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package controllercharm

import (
	"github.com/juju/errors"
	"github.com/juju/worker/v3"
	"github.com/juju/worker/v3/dependency"

	"github.com/juju/juju/api/agent/controllercharm"
	"github.com/juju/juju/api/base"
)

// Hub is a pubsub hub used for internal messaging.
type Hub interface {
	Publish(topic string, data interface{}) func()
	Subscribe(topic string, handler func(string, interface{})) func()
}

// ManifoldConfig defines the names of the manifolds on which the
// controllercharm worker depends.
type ManifoldConfig struct {
	APICallerName string
	Hub           Hub
	Logger        Logger

	NewFacade func(base.APICaller) (Facade, error)
	NewWorker func(Config) (worker.Worker, error)
}

// validate is called by start to check for bad configuration.
func (config ManifoldConfig) validate() error {
	if config.APICallerName == "" {
		return errors.NotValidf("empty APICallerName")
	}
	if config.Hub == nil {
		return errors.NotValidf("nil Hub")
	}
	if config.Logger == nil {
		return errors.NotValidf("nil Logger")
	}
	if config.NewFacade == nil {
		return errors.NotValidf("nil NewFacade")
	}
	if config.NewWorker == nil {
		return errors.NotValidf("nil NewWorker")
	}
	return nil
}

// start is a StartFunc for a Worker manifold.
func (config ManifoldConfig) start(context dependency.Context) (worker.Worker, error) {
	config.Logger.Infof("attempting to start controllercharm worker manifold")
	if err := config.validate(); err != nil {
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

	worker, err := config.NewWorker(Config{
		Facade: facade,
		Hub:    config.Hub,
		Logger: config.Logger,
	})
	if err != nil {
		return nil, errors.Trace(err)
	}
	return worker, nil
}

// Manifold returns a dependency manifold that runs the migration
// worker.
func Manifold(config ManifoldConfig) dependency.Manifold {
	return dependency.Manifold{
		Inputs: []string{
			config.APICallerName,
		},
		Start: config.start,
	}
}

func NewFacade(apiCaller base.APICaller) (Facade, error) {
	facade := controllercharm.NewClient(apiCaller)
	return facade, nil
}
