// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package authenticationworker

import (
	"context"

	"github.com/juju/errors"
	"github.com/juju/worker/v5"
	"github.com/juju/worker/v5/dependency"

	coressh "github.com/juju/juju/core/ssh"
)

// Manifold returns a dependency manifold that runs the ephemeral SSH key worker.
func Manifold(output dependency.OutputFunc) dependency.Manifold {
	manifold := dependency.Manifold{Start: newWorker}
	// Expose the worker's EphemeralKeysUpdater so that the sshsession worker can
	// inject and remove ephemeral keys for the lifetime of a reverse tunnel.
	manifold.Output = output
	return manifold
}

func newWorker(_ context.Context, _ dependency.Getter) (worker.Worker, error) {
	w, err := NewWorker()
	if err != nil {
		return nil, errors.Annotate(err, "cannot start ephemeral SSH key worker")
	}
	return w, nil
}

// output extracts an EphemeralKeysUpdater from the running AuthWorker.
func Output(in worker.Worker, out any) error {
	w, ok := in.(*AuthWorker)
	if !ok {
		return errors.Errorf("expected *AuthWorker, got %T", in)
	}
	switch outPtr := out.(type) {
	case *coressh.EphemeralKeysUpdater:
		*outPtr = w
	default:
		return errors.Errorf("expected *coressh.EphemeralKeysUpdater, got %T", out)
	}
	return nil
}
