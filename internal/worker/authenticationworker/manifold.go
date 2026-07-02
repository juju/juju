// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package authenticationworker

import (
	"context"

	"github.com/juju/errors"
	"github.com/juju/worker/v5"
	"github.com/juju/worker/v5/dependency"

	"github.com/juju/juju/agent"
	"github.com/juju/juju/agent/engine"
	"github.com/juju/juju/api/agent/keyupdater"
	"github.com/juju/juju/api/base"
)

// ManifoldConfig defines the names of the manifolds on which a Manifold will depend.
type ManifoldConfig engine.AgentAPIManifoldConfig

// Manifold returns a dependency manifold that runs a authenticationworker worker,
// using the resource names defined in the supplied config.
func Manifold(config ManifoldConfig) dependency.Manifold {
	typedConfig := engine.AgentAPIManifoldConfig(config)

	manifold := engine.AgentAPIManifold(typedConfig, newWorker)
	// Expose the worker's EphemeralKeysUpdater so that the sshsession worker can
	// inject and remove ephemeral keys for the lifetime of a reverse tunnel.
	manifold.Output = output
	return manifold
}

func newWorker(_ context.Context, a agent.Agent, apiCaller base.APICaller) (worker.Worker, error) {
	w, err := NewWorker(keyupdater.NewClient(apiCaller), a.CurrentConfig())
	if err != nil {
		return nil, errors.Annotate(err, "cannot start ssh auth-keys updater worker")
	}
	return w, nil
}

// output extracts an EphemeralKeysUpdater from the running AuthWorker.
func output(in worker.Worker, out any) error {
	w, ok := in.(*AuthWorker)
	if !ok {
		return errors.Errorf("expected *AuthWorker, got %T", in)
	}
	switch outPtr := out.(type) {
	case *EphemeralKeysUpdater:
		*outPtr = w
	default:
		return errors.Errorf("expected *EphemeralKeysUpdater, got %T", out)
	}
	return nil
}
