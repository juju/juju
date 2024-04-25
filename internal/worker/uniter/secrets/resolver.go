// Copyright 2021 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package secrets

import (
	"context"
	"strconv"
	"strings"

	"github.com/juju/charm/v13/hooks"
	"github.com/juju/collections/set"
	"github.com/juju/errors"

	"github.com/juju/juju/core/life"
	"github.com/juju/juju/core/logger"
	"github.com/juju/juju/internal/worker/uniter/hook"
	"github.com/juju/juju/internal/worker/uniter/operation"
	"github.com/juju/juju/internal/worker/uniter/remotestate"
	"github.com/juju/juju/internal/worker/uniter/resolver"
)

// secretsResolver is a Resolver that returns operations to rotate secrets.
// When a rotation is completed, the "rotatedSecrets" callback
// is invoked to update the rotate time in the remote state.
type secretsResolver struct {
	logger           logger.Logger
	secretsTracker   SecretStateTracker
	rotatedSecrets   func(url string)
	expiredRevisions func(rev string)
	deletedSecrets   func(uris []string)
}

// NewSecretsResolver returns a new Resolver that returns operations
// to rotate, expire, or run other secret related hooks.
func NewSecretsResolver(logger logger.Logger, secretsTracker SecretStateTracker,
	rotatedSecrets func(string), expiredRevisions func(string), deletedSecrets func([]string),
) resolver.Resolver {
	return &secretsResolver{logger: logger, secretsTracker: secretsTracker,
		rotatedSecrets: rotatedSecrets, expiredRevisions: expiredRevisions, deletedSecrets: deletedSecrets}
}

// NextOp is part of the resolver.Resolver interface.
func (s *secretsResolver) NextOp(
	ctx context.Context,
	localState resolver.LocalState,
	remoteState remotestate.Snapshot,
	opFactory operation.Factory,
) (operation.Operation, error) {
	// Nothing to do if not yet installed, the unit is dying.
	if !localState.Installed || remoteState.Life == life.Dying {
		return nil, resolver.ErrNoOperation
	}

	// We should only evaluate the resolver logic if there is no other pending operation
	if localState.Kind != operation.Continue {
		return nil, resolver.ErrNoOperation
	}

	if op, err := s.expireOp(localState, remoteState, opFactory); err == nil || err != resolver.ErrNoOperation {
		return op, err
	}
	if op, err := s.rotateOp(localState, remoteState, opFactory); err == nil || err != resolver.ErrNoOperation {
		return op, err
	}
	for uri, info := range remoteState.ConsumedSecretInfo {
		existing := s.secretsTracker.ConsumedSecretRevision(uri)
		s.logger.Debugf("%s: current=%d, new=%d", uri, existing, info.LatestRevision)
		if existing != info.LatestRevision {
			op, err := opFactory.NewRunHook(hook.Info{
				Kind:           hooks.SecretChanged,
				SecretURI:      uri,
				SecretRevision: info.LatestRevision,
				SecretLabel:    info.Label,
			})
			return op, err
		}
	}
	if len(remoteState.DeletedSecrets) > 0 {
		op, err := opFactory.NewNoOpSecretsRemoved(remoteState.DeletedSecrets)
		if err != nil {
			return nil, errors.Trace(err)
		}
		opCompleted := func() {
			s.deletedSecrets(remoteState.DeletedSecrets)
		}
		return &secretCompleter{op, opCompleted}, nil
	}
	for uri, revs := range remoteState.ObsoleteSecretRevisions {
		s.logger.Debugf("%s: resolving obsolete %v", uri, revs)
		alreadyProcessed := set.NewInts(s.secretsTracker.SecretObsoleteRevisions(uri)...)
		for _, rev := range revs {
			if alreadyProcessed.Contains(rev) {
				continue
			}
			op, err := opFactory.NewRunHook(hook.Info{
				Kind:           hooks.SecretRemove,
				SecretURI:      uri,
				SecretRevision: rev,
			})
			return op, err
		}
	}
	return nil, resolver.ErrNoOperation
}

func (s *secretsResolver) rotateOp(
	_ resolver.LocalState,
	remoteState remotestate.Snapshot,
	opFactory operation.Factory,
) (operation.Operation, error) {
	if len(remoteState.SecretRotations) == 0 {
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

func splitSecretChange(c string) (string, int) {
	parts := strings.Split(c, "/")
	if len(parts) < 2 {
		return parts[0], 0
	}
	rev, _ := strconv.Atoi(parts[1])
	return parts[0], rev
}

func (s *secretsResolver) expireOp(
	_ resolver.LocalState,
	remoteState remotestate.Snapshot,
	opFactory operation.Factory,
) (operation.Operation, error) {
	if len(remoteState.ExpiredSecretRevisions) == 0 {
		return nil, resolver.ErrNoOperation
	}

	revSpec := remoteState.ExpiredSecretRevisions[0]
	uri, rev := splitSecretChange(revSpec)
	if rev == 0 {
		s.logger.Warningf("ignoring invalid secret revision %q", revSpec)
		return nil, resolver.ErrNoOperation
	}

	op, err := opFactory.NewRunHook(hook.Info{
		Kind:           hooks.SecretExpired,
		SecretURI:      uri,
		SecretRevision: rev,
	})
	if err != nil {
		return nil, errors.Trace(err)
	}
	opCompleted := func() {
		s.expiredRevisions(revSpec)
	}
	return &secretCompleter{Operation: op, secretCompleted: opCompleted}, nil
}

type secretCompleter struct {
	operation.Operation
	secretCompleted func()
}

func (c *secretCompleter) Commit(ctx context.Context, st operation.State) (*operation.State, error) {
	result, err := c.Operation.Commit(ctx, st)
	if err == nil {
		c.secretCompleted()
	}
	return result, err
}

// WrappedOperation is part of the WrappedOperation interface.
func (c *secretCompleter) WrappedOperation() operation.Operation {
	return c.Operation
}
