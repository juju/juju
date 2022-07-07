// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package upgradeseries

import (
	"github.com/juju/charm/v9/hooks"

	"github.com/juju/juju/core/model"
	"github.com/juju/juju/worker/uniter/hook"
	"github.com/juju/juju/worker/uniter/operation"
	"github.com/juju/juju/worker/uniter/remotestate"
	"github.com/juju/juju/worker/uniter/resolver"
)

// Logger represents the logging methods used by this package.
type Logger interface {
	Debugf(string, ...interface{})
	Tracef(string, ...interface{})
}

type upgradeSeriesResolver struct{ logger Logger }

// NewResolver returns a new upgrade-series resolver.
func NewResolver(logger Logger) resolver.Resolver {
	return &upgradeSeriesResolver{logger}
}

// NextOp is defined on the Resolver interface.
func (r *upgradeSeriesResolver) NextOp(
	localState resolver.LocalState, remoteState remotestate.Snapshot, opFactory operation.Factory,
) (operation.Operation, error) {
	// If the unit is in the validate state, just sit and idle until validation
	// has been completed.
	if remoteState.UpgradeSeriesStatus == model.UpgradeSeriesValidate {
		r.logger.Debugf("unit validating, waiting for prepare started")
		return nil, resolver.ErrDoNotProceed
	}

	// If the unit has completed a pre-series-upgrade hook
	// (as noted by its state) then the uniter should idle in the face of all
	// remote state changes.
	if remoteState.UpgradeSeriesStatus == model.UpgradeSeriesPrepareCompleted {
		r.logger.Debugf("unit prepared, waiting for complete request")
		return nil, resolver.ErrDoNotProceed
	}

	r.logger.Tracef("localState.Kind=%q, localState.UpgradeSeriesStatus=%q, remoteState.UpgradeSeriesStatus=%q",
		localState.Kind, localState.UpgradeSeriesStatus, remoteState.UpgradeSeriesStatus)

	if localState.Kind == operation.Continue {
		if localState.UpgradeSeriesStatus == model.UpgradeSeriesNotStarted &&
			remoteState.UpgradeSeriesStatus == model.UpgradeSeriesPrepareStarted {
			return opFactory.NewRunHook(hook.Info{
				Kind:                hooks.PreSeriesUpgrade,
				SeriesUpgradeTarget: remoteState.UpgradeSeriesTarget,
			})
		}

		// The uniter's local state will be in the "not started" state if the
		// uniter was stopped for any reason, while performing a series upgrade.
		// If the uniter was not stopped then it will be in the "prepare completed"
		// state and likewise run the post upgrade hook.
		if (localState.UpgradeSeriesStatus == model.UpgradeSeriesNotStarted ||
			localState.UpgradeSeriesStatus == model.UpgradeSeriesPrepareCompleted) &&
			remoteState.UpgradeSeriesStatus == model.UpgradeSeriesCompleteStarted {
			return opFactory.NewRunHook(hook.Info{Kind: hooks.PostSeriesUpgrade})
		}

		// If the local state is completed but the remote state is not started,
		// then this means that the lock has been removed and the local uniter
		// state should be reset.
		if localState.UpgradeSeriesStatus == model.UpgradeSeriesCompleted &&
			remoteState.UpgradeSeriesStatus == model.UpgradeSeriesNotStarted {
			return opFactory.NewNoOpFinishUpgradeSeries()
		}
	}

	return nil, resolver.ErrNoOperation
}
