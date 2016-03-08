// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package lifeflag

import (
	"github.com/juju/errors"
	"github.com/juju/names"

	"github.com/juju/juju/api/base"
	"github.com/juju/juju/core/life"
	"github.com/juju/juju/worker"
	"github.com/juju/juju/worker/dependency"
)

type ManifoldConfig struct {
	APICallerName string
	Entity        names.Tag
	Result        life.Predicate
	Filter        dependency.FilterFunc

	NewFacade func(base.APICaller) (Facade, error)
	NewWorker func(Config) (worker.Worker, error)
}

func (config ManifoldConfig) start(getResource dependency.GetResourceFunc) (worker.Worker, error) {

	var apiCaller base.APICaller
	if err := getResource(config.APICallerName, &apiCaller); err != nil {
		return nil, errors.Trace(err)
	}
	facade, err := config.NewFacade(apiCaller)
	if err != nil {
		return nil, errors.Trace(err)
	}

	worker, err := config.NewWorker(Config{
		Facade: facade,
		Entity: config.Entity,
		Result: config.Result,
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
		Output: manifoldOutput,
		Filter: config.Filter,
	}
}

func manifoldOutput(in worker.Worker, out interface{}) error {
	inWorker, ok := in.(*Worker)
	if !ok {
		return errors.Errorf("expected in to be a *FlagWorker, got a %T", in)
	}
	outFlag, ok := out.(*dependency.Flag)
	if !ok {
		return errors.Errorf("expected out to be a *dependency.Flag, got a %T", out)
	}
	*outFlag = inWorker
	return nil
}
