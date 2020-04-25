// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package upgradeseries

import (
	"github.com/juju/charm/v7/hooks"

	"github.com/juju/juju/core/model"
	"github.com/juju/juju/worker/uniter/hook"
	"github.com/juju/juju/worker/uniter/operation"
	"github.com/juju/juju/worker/uniter/remotestate"
	"github.com/juju/juju/worker/uniter/resolver"
)

type upgradeSeriesResolver struct{}

// NewResolver returns a new upgrade-series resolver.
func NewResolver() resolver.Resolver {
	return &upgradeSeriesResolver{}
}

// NextOp is defined on the Resolver interface.
func (l *upgradeSeriesResolver) NextOp(
	localState resolver.LocalState, remoteState remotestate.Snapshot, opFactory operation.Factory,
) (operation.Operation, error) {
	// If the unit has completed a pre-series-upgrade hook
	// (as noted by its state) then the uniter should idle in the face of all
	// remote state changes.
	if remoteState.UpgradeSeriesStatus == model.UpgradeSeriesPrepareCompleted {
		return nil, resolver.ErrDoNotProceed
	}

	if localState.Kind == operation.Continue {
		if localState.UpgradeSeriesStatus == model.UpgradeSeriesNotStarted &&
			remoteState.UpgradeSeriesStatus == model.UpgradeSeriesPrepareStarted {
			return opFactory.NewRunHook(hook.Info{Kind: hooks.PreSeriesUpgrade})
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
