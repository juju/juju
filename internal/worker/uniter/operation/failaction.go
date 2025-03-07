// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package operation

import (
	"fmt"

	"github.com/juju/juju/internal/worker/uniter/remotestate"
)

type failAction struct {
	actionId  string
	callbacks Callbacks
	RequiresMachineLock
}

// String is part of the Operation interface.
func (fa *failAction) String() string {
	return fmt.Sprintf("fail action %s", fa.actionId)
}

// Prepare is part of the Operation interface.
func (fa *failAction) Prepare(state State) (*State, error) {
	return stateChange{
		Kind:     RunAction,
		Step:     Pending,
		ActionId: &fa.actionId,
		Hook:     state.Hook,
	}.apply(state), nil
}

// Execute is part of the Operation interface.
func (fa *failAction) Execute(state State) (*State, error) {
	if err := fa.callbacks.FailAction(fa.actionId, "action terminated"); err != nil {
		return nil, err
	}

	return stateChange{
		Kind:     RunAction,
		Step:     Done,
		ActionId: &fa.actionId,
		Hook:     state.Hook,
	}.apply(state), nil
}

// Commit preserves the recorded hook, and returns a neutral state.
// Commit is part of the Operation interface.
func (fa *failAction) Commit(state State) (*State, error) {
	return stateChange{
		Kind: continuationKind(state),
		Step: Pending,
		Hook: state.Hook,
	}.apply(state), nil
}

// RemoteStateChanged is called when the remote state changed during execution
// of the operation.
func (fa *failAction) RemoteStateChanged(snapshot remotestate.Snapshot) {
}
