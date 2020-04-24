// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package fanconfigurer

import (
	"github.com/juju/errors"
	"github.com/juju/worker/v2"
	"github.com/juju/worker/v2/dependency"

	"github.com/juju/clock"
	"github.com/juju/juju/api/base"
	apifanconfigurer "github.com/juju/juju/api/fanconfigurer"
)

// ManifoldConfig defines the names of the manifolds on which a
// Manifold will depend.
type ManifoldConfig struct {
	// These are the dependency resource names.
	APICallerName string
	Clock         clock.Clock
}

// Manifold returns a dependency manifold that runs a fan configurer
// worker, using the resource names defined in the supplied config.
func Manifold(config ManifoldConfig) dependency.Manifold {
	return dependency.Manifold{
		Inputs: []string{
			config.APICallerName,
		},
		Output: func(in worker.Worker, out interface{}) error {
			inWorker, _ := in.(*FanConfigurer)
			if inWorker == nil {
				return errors.Errorf("in should be a %T; got %T", inWorker, in)
			}
			switch outPointer := out.(type) {
			case *bool:
				*outPointer = true
			default:
				return errors.Errorf("out should be *bool; got %T", out)
			}
			return nil
		},
		Start: func(context dependency.Context) (worker.Worker, error) {
			var apiCaller base.APICaller
			if err := context.Get(config.APICallerName, &apiCaller); err != nil {
				return nil, errors.Trace(err)
			}

			facade := apifanconfigurer.NewFacade(apiCaller)

			fanconfigurer, err := NewFanConfigurer(FanConfigurerConfig{
				Facade: facade,
			}, config.Clock)
			return fanconfigurer, errors.Annotate(err, "creating fanconfigurer orchestrator")
		},
	}
}
