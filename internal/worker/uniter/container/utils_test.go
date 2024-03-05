// Copyright 2021 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package container_test

import (
	"github.com/juju/juju/internal/worker/uniter/hook"
	"github.com/juju/juju/internal/worker/uniter/operation"
	"github.com/juju/juju/internal/worker/uniter/remotestate"
)

type mockOperations struct {
	operation.Factory
}

func (m *mockOperations) NewRemoteInit(runningStatus remotestate.ContainerRunningStatus) (operation.Operation, error) {
	return &mockRemoteInit{runningStatus: runningStatus}, nil
}

func (m *mockOperations) NewSkipRemoteInit(retry bool) (operation.Operation, error) {
	return &mockSkipOp{retry: retry}, nil
}

func (m *mockOperations) NewRunHook(hookInfo hook.Info) (operation.Operation, error) {
	return &mockRunHookOp{hookInfo: hookInfo}, nil
}

func (m *mockOperations) NewSkipHook(hookInfo hook.Info) (operation.Operation, error) {
	return &mockSkipHookOp{hookInfo: hookInfo}, nil
}

type mockRemoteInit struct {
	operation.Operation
	runningStatus remotestate.ContainerRunningStatus
}

func (op *mockRemoteInit) String() string {
	return "remote init"
}

type mockSkipOp struct {
	operation.Operation
	retry bool
}

func (op *mockSkipOp) String() string {
	return "skip remote init"
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
