// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package modelupgrader

import (
	"github.com/juju/errors"
	"gopkg.in/juju/names.v2"
	"gopkg.in/juju/worker.v1"

	"github.com/juju/juju/api/base"
	"github.com/juju/juju/environs"
	"github.com/juju/juju/worker/dependency"
	"github.com/juju/juju/worker/gate"
)

// ManifoldConfig describes how to configure and construct a Worker,
// and what registered resources it may depend upon.
type ManifoldConfig struct {
	APICallerName string
	EnvironName   string
	GateName      string
	ControllerTag names.ControllerTag
	ModelTag      names.ModelTag

	NewFacade func(base.APICaller) (Facade, error)
	NewWorker func(Config) (worker.Worker, error)
}

func (config ManifoldConfig) start(context dependency.Context) (worker.Worker, error) {

	var environ environs.Environ
	if err := context.Get(config.EnvironName, &environ); err != nil {
		if errors.Cause(err) != dependency.ErrMissing {
			return nil, errors.Trace(err)
		}
		// Only the controller's leader is given an Environ; the
		// other controller units will watch the model and wait
		// for its environ version to be updated.
		environ = nil
	}

	var apiCaller base.APICaller
	if err := context.Get(config.APICallerName, &apiCaller); err != nil {
		return nil, errors.Trace(err)
	}

	var gate gate.Unlocker
	if err := context.Get(config.GateName, &gate); err != nil {
		return nil, errors.Trace(err)
	}

	facade, err := config.NewFacade(apiCaller)
	if err != nil {
		return nil, errors.Trace(err)
	}

	worker, err := config.NewWorker(Config{
		Facade:        facade,
		Environ:       environ,
		GateUnlocker:  gate,
		ControllerTag: config.ControllerTag,
		ModelTag:      config.ModelTag,
	})
	if err != nil {
		return nil, errors.Trace(err)
	}
	return worker, nil
}

// Manifold returns a dependency.Manifold that will run a Worker as
// configured.
func Manifold(config ManifoldConfig) dependency.Manifold {
	return dependency.Manifold{
		Inputs: []string{
			config.APICallerName,
			config.EnvironName,
			config.GateName,
		},
		Start: config.start,
	}
}
