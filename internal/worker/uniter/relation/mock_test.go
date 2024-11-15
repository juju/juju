// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package relation_test

import (
	"context"
	"fmt"

	"github.com/juju/juju/internal/charm/hooks"
	"github.com/juju/juju/internal/worker/uniter/hook"
	"github.com/juju/juju/internal/worker/uniter/operation"
	"github.com/juju/juju/internal/worker/uniter/remotestate"
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

func (m *mockOperation) ExecutionGroup() string {
	return ""
}

func (m *mockOperation) Prepare(ctx context.Context, state operation.State) (*operation.State, error) {
	return &state, nil
}

func (m *mockOperation) Execute(ctx context.Context, state operation.State) (*operation.State, error) {
	return &state, nil
}

func (m *mockOperation) Commit(ctx context.Context, state operation.State) (*operation.State, error) {
	return &state, nil
}

func (m *mockOperation) RemoteStateChanged(ctx context.Context, snapshot remotestate.Snapshot) {
}

func (m *mockOperation) HookInfo() hook.Info {
	return m.hookInfo
}
