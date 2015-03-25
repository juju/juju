// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package filter

import (
	"fmt"

	"github.com/juju/errors"
	"github.com/juju/names"

	"github.com/juju/juju/api"
	"github.com/juju/juju/worker"
	"github.com/juju/juju/worker/agent"
	"github.com/juju/juju/worker/dependency"
)

type ManifoldConfig struct {
	AgentName         string
	ApiConnectionName string
}

func Manifold(config ManifoldConfig) dependency.Manifold {
	return dependency.Manifold{
		Inputs: []string{
			config.AgentName,
			config.ApiConnectionName,
		},
		Start: func(getResource dependency.GetResourceFunc) (worker.Worker, error) {
			var agent agent.Agent
			if !getResource(config.AgentName, &agent) {
				return nil, dependency.ErrUnmetDependencies
			}
			unitTag, ok := agent.Tag().(names.UnitTag)
			if !ok {
				return nil, fmt.Errorf("expected a unit tag; got %q", agent.Tag())
			}
			var apiConnection *api.State
			if !getResource(config.ApiConnectionName, &apiConnection) {
				return nil, dependency.ErrUnmetDependencies
			}
			uniterFacade, err := apiConnection.Uniter()
			if err != nil {
				return nil, errors.Trace(err)
			}
			return NewFilter(uniterFacade, unitTag)
		},
		Output: func(in worker.Worker, out interface{}) bool {
			inWorker, _ := in.(Filter)
			outPointer, _ := out.(*Filter)
			if inWorker == nil || outPointer == nil {
				return false
			}
			*outPointer = inWorker
			return true
		},
	}
}
