// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package resolver

import (
	"github.com/juju/errors"
	"gopkg.in/juju/charm.v6"

	"github.com/juju/juju/core/model"
	"github.com/juju/juju/worker/uniter/operation"
	"github.com/juju/juju/worker/uniter/remotestate"
)

// ErrNoOperation is used to indicate that there are no
// currently pending operations to run.
var ErrNoOperation = errors.New("no operations")

// ErrWaiting indicates that the resolver loop should
// not execute any more operations until a remote state
// event has occurred.
var ErrWaiting = errors.New("waiting for remote state change")

// ErrRestart indicates that the resolver loop should
// be restarted with a new remote state watcher.
var ErrRestart = errors.New("restarting resolver")

// ErrTerminate is used when the unit has been marked
// as dead and so there will never be any more
// operations to run for that unit.
var ErrTerminate = errors.New("terminate resolver")

// Resolver instances use local (as is) and remote (to be) state
// to provide operations to run in order to progress towards
// the desired state.
type Resolver interface {
	// NextOp returns the next operation to run to reconcile
	// the local state with the remote, desired state. The
	// operations returned must be created using the given
	// operation.Factory.
	//
	// This method must return ErrNoOperation if there are no
	// operations to perform.
	//
	// By returning ErrTerminate, the resolver indicates that
	// it will never have any more operations to perform,
	// and the caller can cease calling.
	NextOp(
		LocalState,
		remotestate.Snapshot,
		operation.Factory,
	) (operation.Operation, error)
}

// LocalState is a cache of the state of the local unit, as needed by the
// Uniter. It is generally compared to the remote state of the expected state of
// the unit as stored in the controller.
type LocalState struct {
	operation.State

	// CharmModifiedVersion increases any time the charm,
	// or any part of it, is changed in some way.
	CharmModifiedVersion int

	// CharmURL reports the currently installed charm URL. This is set
	// by the committing of deploy (install/upgrade) ops.
	CharmURL *charm.URL

	// Conflicted indicates that the uniter is in a conflicted state,
	// and needs either resolution or a forced upgrade to continue.
	Conflicted bool

	// Restart indicates that the resolver should exit with ErrRestart
	// at the earliest opportunity.
	Restart bool

	// UpdateStatusVersion is the version of update status from remotestate.Snapshot
	// for which an update-status hook has been committed.
	UpdateStatusVersion int

	// RetryHookVersion is the version of hook-retries from
	// remotestate.Snapshot for which a hook has been retried.
	RetryHookVersion int

	// ConfigVersion is the version of config from remotestate.Snapshot
	// for which a config-changed hook has been committed.
	ConfigVersion int

	// LeaderSettingsVersion is the version of leader settings from
	// remotestate.Snapshot for which a leader-settings-changed hook has
	// been committed.
	LeaderSettingsVersion int

	// CompletedActions is the set of actions that have been completed.
	// This is used to prevent us re running actions requested by the
	// controller.
	CompletedActions map[string]struct{}

	// Series is the current series running on the unit from remotestate.Snapshot
	// for which a config-changed hook has been committed.
	Series string

	// UpgradeSeriesStatus is the current state of any currently running
	// upgrade series.
	UpgradeSeriesStatus model.UpgradeSeriesStatus
}
