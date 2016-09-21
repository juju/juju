// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package operation

import (
	"fmt"
)

type failAction struct {
	actionId  string
	callbacks Callbacks
	name      string
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
