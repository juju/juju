// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package authenticationworker

import (
	"github.com/juju/errors"
	"github.com/juju/worker/v3"
	"github.com/juju/worker/v3/dependency"
	gossh "golang.org/x/crypto/ssh"

	"github.com/juju/juju/agent"
	"github.com/juju/juju/api/agent/keyupdater"
	"github.com/juju/juju/api/base"
	"github.com/juju/juju/cmd/jujud/agent/engine"
)

type EphemeralKeysUpdater interface {
	AddEphemeralKey(ephemeralKey gossh.PublicKey) error
	RemoveEphemeralKey(ephemeralKey gossh.PublicKey) error
}

// ManifoldConfig defines the names of the manifolds on which a Manifold will depend.
type ManifoldConfig engine.AgentAPIManifoldConfig

// Manifold returns a dependency manifold that runs a authenticationworker worker,
// using the resource names defined in the supplied config.
func Manifold(config ManifoldConfig) dependency.Manifold {
	return dependency.Manifold{
		Inputs: []string{
			config.AgentName,
			config.APICallerName,
		},
		Start: func(context dependency.Context) (worker.Worker, error) {
			var agent agent.Agent
			if err := context.Get(config.AgentName, &agent); err != nil {
				return nil, err
			}
			var apiCaller base.APICaller
			if err := context.Get(config.APICallerName, &apiCaller); err != nil {
				return nil, err
			}
			return newWorker(agent, apiCaller)
		},
		Output: outputFunc,
	}
}

func newWorker(a agent.Agent, apiCaller base.APICaller) (worker.Worker, error) {
	w, err := NewWorker(keyupdater.NewState(apiCaller), a.CurrentConfig())
	if err != nil {
		return nil, errors.Annotate(err, "cannot start ssh auth-keys updater worker")
	}
	return w, nil
}

func outputFunc(in worker.Worker, out interface{}) error {
	inWorker, _ := in.(*AuthWorker)
	if inWorker == nil {
		return errors.Errorf("in should be a %T; got %T", inWorker, in)
	}
	switch outPointer := out.(type) {
	case *EphemeralKeysUpdater:
		*outPointer = inWorker
	default:
		return errors.Errorf("out should be keyUpdater; got %T", out)
	}
	return nil
}
