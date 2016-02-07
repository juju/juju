// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.
package apiserver

import (
	"github.com/juju/errors"

	"github.com/juju/juju/agent"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/state"
	"github.com/juju/juju/worker"
	"github.com/juju/juju/worker/dependency"
)

// ManifoldConfig defines an ecertupdater's dependencies.
type ManifoldConfig struct {
	AgentName          string
	NewApiserverWorker func(st *state.State, certChanged chan params.StateServingInfo) (worker.Worker, error)
	OpenState          func() (_ *state.State, _ *state.Machine, err error)
	CertChangedChan    chan params.StateServingInfo
}

// Manifold creates a manifold that runs a certupdater worker.
func Manifold(config ManifoldConfig) dependency.Manifold {
	return dependency.Manifold{
		Inputs: []string{config.AgentName},
		Start: func(getResource dependency.GetResourceFunc) (worker.Worker, error) {
			var agent agent.Agent
			if err := getResource(config.AgentName, &agent); err != nil {
				return nil, errors.Trace(err)
			}

			// Each time aipserver worker is restarted, we need a fresh copy of state due
			// to the fact that state holds lease managers which are killed and need to be reset.
			stateOpener := func() (*state.State, error) {
				st, _, err := config.OpenState()
				return st, err
			}

			w, err := NewWorker(stateOpener, config.NewApiserverWorker, config.CertChangedChan)
			if err != nil {
				return nil, errors.Annotate(err, "cannot start apiserver worker")
			}
			return w, nil
		},
	}
}
