// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package relation_test

import (
	"fmt"

	"gopkg.in/juju/charm.v6/hooks"

	"github.com/juju/juju/worker/uniter/hook"
	"github.com/juju/juju/worker/uniter/operation"
	"github.com/juju/juju/worker/uniter/remotestate"
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
	if m.hookInfo.Kind == hooks.RelationBroken {
		// There is no app or unit for RelationBroken
		return fmt.Sprintf("run hook %v with relation %d",
			m.hookInfo.Kind, m.hookInfo.RelationId)
	}
	onStr := fmt.Sprintf("app %v", m.hookInfo.RemoteApplication)
	if m.hookInfo.RemoteUnit != "" {
		onStr = fmt.Sprintf("unit %v", m.hookInfo.RemoteUnit)
	}
	return fmt.Sprintf("run hook %v on %v with relation %d",
		m.hookInfo.Kind, onStr, m.hookInfo.RelationId)
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

func (m *mockOperation) RemoteStateChanged(snapshot remotestate.Snapshot) {
}

func (m *mockOperation) HookInfo() hook.Info {
	return m.hookInfo
}
