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
			if err := getResource(config.AgentName, &agent); err != nil {
				return nil, err
			}
			unitTag, ok := agent.Tag().(names.UnitTag)
			if !ok {
				return nil, fmt.Errorf("expected a unit tag; got %q", agent.Tag())
			}
			var apiConnection *api.State
			if err := getResource(config.ApiConnectionName, &apiConnection); err != nil {
				return nil, err
			}
			uniterFacade, err := apiConnection.Uniter()
			if err != nil {
				return nil, errors.Trace(err)
			}
			return NewFilter(uniterFacade, unitTag)
		},
		Output: func(in worker.Worker, out interface{}) error {
			inWorker, _ := in.(Filter)
			outPointer, _ := out.(*Filter)
			if inWorker == nil || outPointer == nil {
				return errors.Errorf("expected %T->%T; got %T->%T", inWorker, outPointer, in, out)
			}
			*outPointer = inWorker
			return nil
		},
	}
}
