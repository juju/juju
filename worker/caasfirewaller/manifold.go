// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package caasfirewaller

import (
	"github.com/juju/errors"
	"gopkg.in/juju/worker.v1"
	"gopkg.in/juju/worker.v1/dependency"

	"github.com/juju/juju/api/base"
	"github.com/juju/juju/caas"
)

// ManifoldConfig describes the resources used by the firewaller worker.
type ManifoldConfig struct {
	APICallerName string
	BrokerName    string

	ControllerUUID string
	ModelUUID      string

	NewClient func(base.APICaller) Client
	NewWorker func(Config) (worker.Worker, error)
}

// Manifold returns a Manifold that encapsulates the firewaller worker.
func Manifold(cfg ManifoldConfig) dependency.Manifold {
	return dependency.Manifold{
		Inputs: []string{
			cfg.APICallerName,
			cfg.BrokerName,
		},
		Start: cfg.start,
	}
}

// Validate is called by start to check for bad configuration.
func (config ManifoldConfig) Validate() error {
	if config.ControllerUUID == "" {
		return errors.NotValidf("empty ControllerUUID")
	}
	if config.ModelUUID == "" {
		return errors.NotValidf("empty ModelUUID")
	}
	if config.APICallerName == "" {
		return errors.NotValidf("empty APICallerName")
	}
	if config.BrokerName == "" {
		return errors.NotValidf("empty BrokerName")
	}
	if config.NewClient == nil {
		return errors.NotValidf("nil NewClient")
	}
	if config.NewWorker == nil {
		return errors.NotValidf("nil NewWorker")
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

	var broker caas.Broker
	if err := context.Get(config.BrokerName, &broker); err != nil {
		return nil, errors.Trace(err)
	}

	client := config.NewClient(apiCaller)
	w, err := config.NewWorker(Config{
		ControllerUUID:    config.ControllerUUID,
		ModelUUID:         config.ModelUUID,
		ApplicationGetter: client,
		LifeGetter:        client,
		ServiceExposer:    broker,
	})
	if err != nil {
		return nil, errors.Trace(err)
	}
	return w, nil
}
