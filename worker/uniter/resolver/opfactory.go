// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package resolver

import (
	"github.com/juju/errors"
	"github.com/juju/loggo"
	"gopkg.in/juju/charm.v6"
	"gopkg.in/juju/charm.v6/hooks"

	"github.com/juju/juju/core/model"
	"github.com/juju/juju/worker/uniter/hook"
	"github.com/juju/juju/worker/uniter/operation"
	"github.com/juju/juju/worker/uniter/remotestate"
)

var logger = loggo.GetLogger("juju.worker.uniter.resolver")

// resolverOpFactory wraps an operation.Factory such that skips that affect
// local state will, when committed, update the embedded LocalState struct
// to reflect the change made by the operation.
//
// The wrapped operations embed information specific to the remote state
// snapshot that was used to create the operation. Thus, remote state changes
// observed between the time the operation was created and committed do not
// affect the operation; and the local state change will not prevent further
// operations from being enqueued to achieve the new remote state.
type resolverOpFactory struct {
	operation.Factory

	LocalState  *LocalState
	RemoteState remotestate.Snapshot
}

func (s *resolverOpFactory) NewRunHook(info hook.Info) (operation.Operation, error) {
	op, err := s.Factory.NewRunHook(info)
	if err != nil {
		return nil, errors.Trace(err)
	}
	return s.wrapHookOp(op, info), nil
}

func (s *resolverOpFactory) NewSkipHook(info hook.Info) (operation.Operation, error) {
	op, err := s.Factory.NewSkipHook(info)
	if err != nil {
		return nil, errors.Trace(err)
	}
	return s.wrapHookOp(op, info), nil
}

func (s *resolverOpFactory) NewNoOpFinishUpgradeSeries() (operation.Operation, error) {
	op, err := s.Factory.NewNoOpFinishUpgradeSeries()
	if err != nil {
		return nil, errors.Trace(err)
	}
	f := func(*operation.State) {
		s.LocalState.UpgradeSeriesStatus = model.UpgradeSeriesNotStarted
	}
	op = onCommitWrapper{op, f}
	return op, nil
}

func (s *resolverOpFactory) NewUpgrade(charmURL *charm.URL) (operation.Operation, error) {
	op, err := s.Factory.NewUpgrade(charmURL)
	if err != nil {
		return nil, errors.Trace(err)
	}
	return s.wrapUpgradeOp(op, charmURL), nil
}

func (s *resolverOpFactory) NewRemoteInit(runningStatus remotestate.ContainerRunningStatus) (operation.Operation, error) {
	op, err := s.Factory.NewRemoteInit(runningStatus)
	if err != nil {
		return nil, errors.Trace(err)
	}
	return onCommitWrapper{op, func(*operation.State) {
		s.LocalState.ContainerRunningStatus = &runningStatus
		s.LocalState.OutdatedRemoteCharm = false
	}}, nil
}

func (s *resolverOpFactory) NewSkipRemoteInit(retry bool) (operation.Operation, error) {
	op, err := s.Factory.NewSkipRemoteInit(retry)
	if err != nil {
		return nil, errors.Trace(err)
	}
	return op, nil
}

func (s *resolverOpFactory) NewRevertUpgrade(charmURL *charm.URL) (operation.Operation, error) {
	op, err := s.Factory.NewRevertUpgrade(charmURL)
	if err != nil {
		return nil, errors.Trace(err)
	}
	return s.wrapUpgradeOp(op, charmURL), nil
}

func (s *resolverOpFactory) NewResolvedUpgrade(charmURL *charm.URL) (operation.Operation, error) {
	op, err := s.Factory.NewResolvedUpgrade(charmURL)
	if err != nil {
		return nil, errors.Trace(err)
	}
	return s.wrapUpgradeOp(op, charmURL), nil
}

func (s *resolverOpFactory) NewAction(id string) (operation.Operation, error) {
	op, err := s.Factory.NewAction(id)
	if err != nil {
		return nil, errors.Trace(err)
	}
	f := func(*operation.State) {
		if s.LocalState.CompletedActions == nil {
			s.LocalState.CompletedActions = make(map[string]struct{})
		}
		s.LocalState.CompletedActions[id] = struct{}{}
		s.LocalState.CompletedActions = trimCompletedActions(s.RemoteState.ActionsPending, s.LocalState.CompletedActions)
	}
	op = onCommitWrapper{op, f}
	return op, nil
}

func trimCompletedActions(pendingActions []string, completedActions map[string]struct{}) map[string]struct{} {
	newCompletedActions := map[string]struct{}{}
	for _, pendingAction := range pendingActions {
		if _, ok := completedActions[pendingAction]; ok {
			newCompletedActions[pendingAction] = struct{}{}
		}
	}
	return newCompletedActions
}

func (s *resolverOpFactory) wrapUpgradeOp(op operation.Operation, charmURL *charm.URL) operation.Operation {
	charmModifiedVersion := s.RemoteState.CharmModifiedVersion
	return onCommitWrapper{op, func(*operation.State) {
		s.LocalState.CharmURL = charmURL
		s.LocalState.Restart = true
		s.LocalState.Conflicted = false
		s.LocalState.CharmModifiedVersion = charmModifiedVersion
	}}
}

func (s *resolverOpFactory) wrapHookOp(op operation.Operation, info hook.Info) operation.Operation {
	switch info.Kind {
	case hooks.PreSeriesUpgrade:
		op = onPrepareWrapper{op, func() {
			//on prepare the local status should be made to reflect
			//that the upgrade process for this united has started.
			s.LocalState.UpgradeSeriesStatus = s.RemoteState.UpgradeSeriesStatus
		}}
		op = onCommitWrapper{op, func(*operation.State) {
			// on commit, the local status should indicate the hook
			// has completed. The remote status should already
			// indicate completion. We sync the states here.
			s.LocalState.UpgradeSeriesStatus = model.UpgradeSeriesPrepareCompleted
		}}
	case hooks.PostSeriesUpgrade:
		op = onPrepareWrapper{op, func() {
			s.LocalState.UpgradeSeriesStatus = s.RemoteState.UpgradeSeriesStatus
		}}
		op = onCommitWrapper{op, func(*operation.State) {
			s.LocalState.UpgradeSeriesStatus = model.UpgradeSeriesCompleted
		}}
	case hooks.ConfigChanged:
		configHash := s.RemoteState.ConfigHash
		trustHash := s.RemoteState.TrustHash
		addressesHash := s.RemoteState.AddressesHash
		op = onCommitWrapper{op, func(state *operation.State) {
			if state != nil {
				// Assign these on the operation.State so it gets
				// written into the state file on disk.
				state.ConfigHash = configHash
				state.TrustHash = trustHash
				state.AddressesHash = addressesHash
			}
		}}
	case hooks.LeaderSettingsChanged:
		v := s.RemoteState.LeaderSettingsVersion
		op = onCommitWrapper{op, func(*operation.State) {
			s.LocalState.LeaderSettingsVersion = v
		}}
	}

	charmModifiedVersion := s.RemoteState.CharmModifiedVersion
	updateStatusVersion := s.RemoteState.UpdateStatusVersion
	op = onCommitWrapper{op, func(*operation.State) {
		// Update UpdateStatusVersion so that the update-status
		// hook only fires after the next timer.
		s.LocalState.UpdateStatusVersion = updateStatusVersion
		s.LocalState.CharmModifiedVersion = charmModifiedVersion
	}}

	retryHookVersion := s.RemoteState.RetryHookVersion
	op = onPrepareWrapper{op, func() {
		// Update RetryHookVersion so that we don't attempt to
		// retry a hook more than once between timers signals.
		//
		// We need to do this in Prepare, rather than Commit,
		// in case the retried hook fails.
		s.LocalState.RetryHookVersion = retryHookVersion
	}}
	return op
}

type onCommitWrapper struct {
	operation.Operation
	onCommit func(*operation.State)
}

func (op onCommitWrapper) Commit(state operation.State) (*operation.State, error) {
	st, err := op.Operation.Commit(state)
	if err != nil {
		return nil, err
	}
	op.onCommit(st)
	return st, nil
}

type onPrepareWrapper struct {
	operation.Operation
	onPrepare func()
}

func (op onPrepareWrapper) Prepare(state operation.State) (*operation.State, error) {
	st, err := op.Operation.Prepare(state)
	if err != nil {
		return nil, err
	}
	op.onPrepare()
	return st, nil
}
