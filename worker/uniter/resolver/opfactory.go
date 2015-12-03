// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package resolver

import (
	"github.com/juju/errors"
	"github.com/juju/loggo"
	"gopkg.in/juju/charm.v6-unstable"
	"gopkg.in/juju/charm.v6-unstable/hooks"

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

func (s *resolverOpFactory) NewUpgrade(charmURL *charm.URL) (operation.Operation, error) {
	op, err := s.Factory.NewUpgrade(charmURL)
	if err != nil {
		return nil, errors.Trace(err)
	}
	return s.wrapUpgradeOp(op, charmURL), nil
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
	f := func() {
		if s.LocalState.CompletedActions == nil {
			s.LocalState.CompletedActions = make(map[string]struct{})
		}
		s.LocalState.CompletedActions[id] = struct{}{}
		s.LocalState.CompletedActions = trimCompletedActions(s.RemoteState.Actions, s.LocalState.CompletedActions)
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
	return onCommitWrapper{op, func() {
		s.LocalState.CharmURL = charmURL
		s.LocalState.Restart = true
		s.LocalState.Conflicted = false
	}}
}

func (s *resolverOpFactory) wrapHookOp(op operation.Operation, info hook.Info) operation.Operation {
	switch info.Kind {
	case hooks.ConfigChanged:
		v := s.RemoteState.ConfigVersion
		op = onCommitWrapper{op, func() {
			s.LocalState.ConfigVersion = v
		}}
	case hooks.LeaderSettingsChanged:
		v := s.RemoteState.LeaderSettingsVersion
		op = onCommitWrapper{op, func() {
			s.LocalState.LeaderSettingsVersion = v
		}}
	}

	updateStatusVersion := s.RemoteState.UpdateStatusVersion
	op = onCommitWrapper{op, func() {
		// Update UpdateStatusVersion so that the update-status
		// hook only fires after the next timer.
		s.LocalState.UpdateStatusVersion = updateStatusVersion
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
	onCommit func()
}

func (op onCommitWrapper) Commit(state operation.State) (*operation.State, error) {
	st, err := op.Operation.Commit(state)
	if err != nil {
		return nil, err
	}
	op.onCommit()
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
