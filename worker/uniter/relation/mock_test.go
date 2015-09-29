// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package relation_test

import (
	"fmt"

	"github.com/juju/juju/worker/uniter/hook"
	"github.com/juju/juju/worker/uniter/operation"
)

type mockOperations struct {
	operation.Factory
}

func (m *mockOperations) NewRunHook(hookInfo hook.Info) (operation.Operation, error) {
	return &mockOperation{hookInfo}, nil
}

type mockOperation struct {
	hookInfo hook.Info
}

func (m *mockOperation) String() string {
	return fmt.Sprintf("run hook %v on unit with relation %d",
		m.hookInfo.Kind, m.hookInfo.RelationId)
}

func (m *mockOperation) NeedsGlobalMachineLock() bool {
	return false
}

func (m *mockOperation) Prepare(state operation.State) (*operation.State, error) {
	return &state, nil
}

func (m *mockOperation) Execute(state operation.State) (*operation.State, error) {
	return &state, nil
}

func (m *mockOperation) Commit(state operation.State) (*operation.State, error) {
	return &state, nil
}
