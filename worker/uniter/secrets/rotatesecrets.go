// Copyright 2021 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package secrets

import (
	"github.com/juju/charm/v9/hooks"
	"github.com/juju/errors"

	"github.com/juju/juju/core/life"
	"github.com/juju/juju/worker/uniter/hook"
	"github.com/juju/juju/worker/uniter/operation"
	"github.com/juju/juju/worker/uniter/remotestate"
	"github.com/juju/juju/worker/uniter/resolver"
)

// secretsResolver is a Resolver that returns operations to rotate secrets.
// When a rotation is completed, the "rotatedSecrets" callback
// is invoked to update the rotate time in the remote state.
type secretsResolver struct {
	rotatedSecrets func(url string)
}

// NewSecretsResolver returns a new Resolver that returns operations
// to rotate secrets.
func NewSecretsResolver(rotatedSecrets func(string)) resolver.Resolver {
	return &secretsResolver{rotatedSecrets}
}

// NextOp is part of the resolver.Resolver interface.
func (s *secretsResolver) NextOp(
	localState resolver.LocalState,
	remoteState remotestate.Snapshot,
	opFactory operation.Factory,
) (operation.Operation, error) {
	if len(remoteState.SecretRotations) == 0 {
		return nil, resolver.ErrNoOperation
	}

	// Nothing to do if not yet installed, the unit is dying, or we're no longer the leader.
	if !localState.Installed || remoteState.Life == life.Dying || !remoteState.Leader {
		return nil, resolver.ErrNoOperation
	}

	// We should only evaluate the resolver logic if there is no other pending operation
	if localState.Kind != operation.Continue {
		return nil, resolver.ErrNoOperation
	}

	uri := remoteState.SecretRotations[0]
	op, err := opFactory.NewRunHook(hook.Info{
		Kind:      hooks.SecretRotate,
		SecretURI: uri,
	})
	if err != nil {
		return nil, errors.Trace(err)
	}
	opCompleted := func() {
		s.rotatedSecrets(uri)
	}
	return &secretCompleter{op, opCompleted}, nil
}

type secretCompleter struct {
	operation.Operation
	secretCompleted func()
}

func (c *secretCompleter) Commit(st operation.State) (*operation.State, error) {
	result, err := c.Operation.Commit(st)
	if err == nil {
		c.secretCompleted()
	}
	return result, err
}

// WrappedOperation is part of the WrappedOperation interface.
func (c *secretCompleter) WrappedOperation() operation.Operation {
	return c.Operation
}
