// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package operation

import (
	"github.com/juju/errors"

	"github.com/juju/juju/worker/uniter/remotestate"
)

type remoteInit struct {
	callbacks     Callbacks
	abort         <-chan struct{}
	runningStatus remotestate.ContainerRunningStatus
	logger        Logger
}

// String is part of the Operation interface.
func (op *remoteInit) String() string {
	return "remote init"
}

// NeedsGlobalMachineLock is part of the Operation interface.
func (op *remoteInit) NeedsGlobalMachineLock() bool {
	return false
}

// Prepare is part of the Operation interface.
func (op *remoteInit) Prepare(state State) (*State, error) {
	return stateChange{
		Kind: RemoteInit,
		Step: Pending,
		Hook: state.Hook,
	}.apply(state), nil
}

// Execute is part of the Operation interface.
func (op *remoteInit) Execute(state State) (*State, error) {
	step := Done
	err := op.callbacks.RemoteInit(op.runningStatus, op.abort)
	if IsRetryableError(errors.Cause(err)) {
		op.logger.Warningf("got error: %v, re-queue the remote init operation and retry later", err)
		step = Queued
	} else if err != nil {
		return nil, err
	}
	return stateChange{
		Kind: RemoteInit,
		Step: step,
		Hook: state.Hook,
	}.apply(state), nil
}

// Commit preserves the recorded hook, and returns a neutral state.
// Commit is part of the Operation interface.
func (op *remoteInit) Commit(state State) (*State, error) {
	if state.Step != Done {
		op.logger.Warningf("remote init operation step %q was not done, so current operation will not continue to next op for now", state.Step)
		return &state, nil
	}
	return stateChange{
		Kind: continuationKind(state),
		Step: Pending,
		Hook: state.Hook,
	}.apply(state), nil
}

// RemoteStateChanged is called when the remote state changed during execution
// of the operation.
func (ri *remoteInit) RemoteStateChanged(snapshot remotestate.Snapshot) {
}

type skipRemoteInit struct {
	retry bool
}

// String is part of the Operation interface.
func (op *skipRemoteInit) String() string {
	return "skip remote init"
}

// NeedsGlobalMachineLock is part of the Operation interface.
func (op *skipRemoteInit) NeedsGlobalMachineLock() bool {
	return false
}

// Prepare is part of the Operation interface.
func (op *skipRemoteInit) Prepare(state State) (*State, error) {
	return nil, ErrSkipExecute
}

// Execute is part of the Operation interface.
func (op *skipRemoteInit) Execute(state State) (*State, error) {
	return nil, ErrSkipExecute
}

// Commit preserves the recorded hook, and returns a neutral state.
// Commit is part of the Operation interface.
func (op *skipRemoteInit) Commit(state State) (*State, error) {
	kind := continuationKind(state)
	if op.retry {
		kind = RemoteInit
	}
	return stateChange{
		Kind: kind,
		Step: Pending,
		Hook: state.Hook,
	}.apply(state), nil
}

// RemoteStateChanged is called when the remote state changed during execution
// of the operation.
func (ri *skipRemoteInit) RemoteStateChanged(snapshot remotestate.Snapshot) {
}
