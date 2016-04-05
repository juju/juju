// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package migrationflag

import (
	"github.com/juju/errors"

	"github.com/juju/juju/api/base"
	"github.com/juju/juju/worker"
	"github.com/juju/juju/worker/dependency"
)

type ManifoldConfig struct {
	APICallerName string
	Check         Predicate

	NewFacade func(base.APICaller) (Facade, error)
	NewWorker func(Config) (worker.Worker, error)
}

func (config ManifoldConfig) start(context dependency.Context) (worker.Worker, error) {
	var apiCaller base.APICaller
	if err := context.Get(config.APICallerName, &apiCaller); err != nil {
		return nil, errors.Trace(err)
	}
	facade, err := config.NewFacade(apiCaller)
	if err != nil {
		return nil, errors.Trace(err)
	}
	modelTag, err := apiCaller.ModelTag()
	if err != nil {
		return nil, errors.Trace(err)
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

func Manifold(config ManifoldConfig) dependency.Manifold {
	return dependency.Manifold{
		Inputs: []string{config.APICallerName},
		Start:  config.start,
		Output: dependency.FlagOutput,
		Filter: func(err error) error {
			if errors.Cause(err) == ErrChanged {
				return dependency.ErrBounce
			}
			return err
		},
	}
}
