// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package migrationflag

import (
	"github.com/juju/errors"
	"github.com/juju/worker/v2"
	"github.com/juju/worker/v2/dependency"

	"github.com/juju/juju/api/base"
	"github.com/juju/juju/cmd/jujud/agent/engine"
)

// logger is here to stop the desire of creating a package level logger.
// Don't do this, instead pass one passed as manifold config.
var logger interface{}

// ManifoldConfig holds the dependencies and configuration for a
// Worker manifold.
type ManifoldConfig struct {
	APICallerName string
	Check         Predicate

	NewFacade func(base.APICaller) (Facade, error)
	NewWorker func(Config) (worker.Worker, error)
}

// validate is called by start to check for bad configuration.
func (config ManifoldConfig) validate() error {
	if config.APICallerName == "" {
		return errors.NotValidf("empty APICallerName")
	}
	if config.Check == nil {
		return errors.NotValidf("nil Check")
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
	modelTag, ok := apiCaller.ModelTag()
	if !ok {
		return nil, errors.New("API connection is controller-only (should never happen)")
	}
	worker, err := config.NewWorker(Config{
		Facade: facade,
		Model:  modelTag.Id(),
		Check:  config.Check,
	})
	if err != nil {
		return nil, errors.Trace(err)
	}
	return worker, nil
}

// Manifold packages a Worker for use in a dependency.Engine.
func Manifold(config ManifoldConfig) dependency.Manifold {
	return dependency.Manifold{
		Inputs: []string{config.APICallerName},
		Start:  config.start,
		Output: engine.FlagOutput,
		Filter: bounceErrChanged,
	}
}

// bounceErrChanged converts ErrChanged to dependency.ErrBounce.
func bounceErrChanged(err error) error {
	if errors.Cause(err) == ErrChanged {
		return dependency.ErrBounce
	}
	return err
}
