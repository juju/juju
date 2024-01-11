// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package lifeflag

import (
	"context"

	"github.com/juju/errors"
	"github.com/juju/names/v5"
	"github.com/juju/worker/v4"
	"github.com/juju/worker/v4/dependency"

	"github.com/juju/juju/agent"
	"github.com/juju/juju/agent/engine"
	"github.com/juju/juju/api/base"
	"github.com/juju/juju/core/life"
)

// logger is here to stop the desire of creating a package level logger.
// Don't do this, instead pass one passed as manifold config.
type logger interface{}

var _ logger = struct{}{}

// ManifoldConfig describes how to configure and construct a Worker,
// and what registered resources it may depend upon.
type ManifoldConfig struct {
	APICallerName string
	AgentName     string

	Entity         names.Tag
	Result         life.Predicate
	Filter         dependency.FilterFunc
	NotFoundIsDead bool

	NewFacade func(base.APICaller) (Facade, error)
	NewWorker func(Config) (worker.Worker, error)
}

func (config ManifoldConfig) start(context context.Context, getter dependency.Getter) (worker.Worker, error) {
	var apiCaller base.APICaller
	if err := getter.Get(config.APICallerName, &apiCaller); err != nil {
		return nil, errors.Trace(err)
	}
	facade, err := config.NewFacade(apiCaller)
	if err != nil {
		return nil, errors.Trace(err)
	}

	if config.AgentName != "" {
		if config.Entity != nil {
			return nil, errors.NotValidf("passing AgentName and Entity")
		}
		var agent agent.Agent
		if err := getter.Get(config.AgentName, &agent); err != nil {
			return nil, err
		}
		config.Entity = agent.CurrentConfig().Tag()
	}

	worker, err := config.NewWorker(Config{
		Facade:         facade,
		Entity:         config.Entity,
		Result:         config.Result,
		NotFoundIsDead: config.NotFoundIsDead,
	})
	if err != nil {
		return nil, errors.Trace(err)
	}
	return worker, nil
}

// Manifold returns a dependency.Manifold that will run a Worker as
// configured.
func Manifold(config ManifoldConfig) dependency.Manifold {
	inputs := []string{config.APICallerName}
	if config.AgentName != "" {
		inputs = append(inputs, config.AgentName)
	}
	return dependency.Manifold{
		Inputs: inputs,
		Start:  config.start,
		Output: engine.FlagOutput,
		Filter: config.Filter,
	}
}
