// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package operation

import (
	"github.com/juju/juju/worker/uniter/remotestate"
)

type remoteInit struct {
	callbacks     Callbacks
	abort         <-chan struct{}
	runningStatus remotestate.ContainerRunningStatus
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
	if err := op.callbacks.RemoteInit(op.runningStatus, op.abort); err != nil {
		return nil, err
	}
	return stateChange{
		Kind: RemoteInit,
		Step: Done,
		Hook: state.Hook,
	}.apply(state), nil
}

// Commit preserves the recorded hook, and returns a neutral state.
// Commit is part of the Operation interface.
func (op *remoteInit) Commit(state State) (*State, error) {
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
