// Copyright 2021 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package container_test

import (
	"github.com/juju/juju/internal/worker/uniter/hook"
	"github.com/juju/juju/internal/worker/uniter/operation"
)

type mockOperations struct {
	operation.Factory
}

func (m *mockOperations) NewRunHook(hookInfo hook.Info) (operation.Operation, error) {
	return &mockRunHookOp{hookInfo: hookInfo}, nil
}

func (m *mockOperations) NewSkipHook(hookInfo hook.Info) (operation.Operation, error) {
	return &mockSkipHookOp{hookInfo: hookInfo}, nil
}

type mockRunHookOp struct {
	operation.Operation
	hookInfo hook.Info
}

func (op *mockRunHookOp) String() string {
	return "hook op"
}

type mockSkipHookOp struct {
	operation.Operation
	hookInfo hook.Info
}

func (op *mockSkipHookOp) String() string {
	return "skip hook op"
}
