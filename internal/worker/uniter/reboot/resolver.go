// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package reboot

import (
	"context"

	"github.com/juju/errors"
	"github.com/juju/juju/charm/hooks"

	"github.com/juju/juju/core/model"
	"github.com/juju/juju/internal/worker/uniter/hook"
	"github.com/juju/juju/internal/worker/uniter/operation"
	"github.com/juju/juju/internal/worker/uniter/remotestate"
	"github.com/juju/juju/internal/worker/uniter/resolver"
)

// Logger represents the logging methods used in this package.
type Logger interface {
	Infof(string, ...interface{})
}

// NewResolver returns a resolver that runs the start hook to notify install
// charms that the machine has been rebooted.
func NewResolver(logger Logger, rebootDetected bool) resolver.Resolver {
	if !rebootDetected {
		return nopResolver{}
	}

	return &rebootResolver{
		rebootDetected: rebootDetected,
		logger:         logger,
	}
}

type nopResolver struct {
}

func (nopResolver) NextOp(_ context.Context, _ resolver.LocalState, _ remotestate.Snapshot, _ operation.Factory) (operation.Operation, error) {
	return nil, resolver.ErrNoOperation
}

type rebootResolver struct {
	rebootDetected bool
	logger         Logger
}

func (r *rebootResolver) NextOp(ctx context.Context, localState resolver.LocalState, remoteState remotestate.Snapshot, opfactory operation.Factory) (operation.Operation, error) {
	// Have we already notified that a reboot occurred?
	if !r.rebootDetected {
		return nil, resolver.ErrNoOperation
	}

	// If we performing a series upgrade, suppress start hooks until the
	// upgrade is complete.
	if remoteState.UpgradeMachineStatus != model.UpgradeSeriesNotStarted {
		return nil, resolver.ErrNoOperation
	}

	// If we did reboot but the charm has not been installed yet then we
	// can safely skip the start hook.
	if !localState.Started {
		r.rebootDetected = false
		return nil, resolver.ErrNoOperation
	}

	// If there is another hook currently, wait until they are done.
	if localState.Kind == operation.RunHook {
		return nil, resolver.ErrNoOperation
	}

	op, err := opfactory.NewRunHook(hook.Info{Kind: hooks.Start})
	if err != nil {
		return nil, errors.Trace(err)
	}

	r.logger.Infof("reboot detected; triggering implicit start hook to notify charm")

	r.rebootDetected = false
	return op, nil
}
