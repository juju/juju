// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package resolver_test

import (
	"github.com/juju/testing"
	"gopkg.in/juju/charm.v5"

	"github.com/juju/juju/worker/uniter/hook"
	"github.com/juju/juju/worker/uniter/operation"
	"github.com/juju/juju/worker/uniter/remotestate"
)

type mockRemoteStateWatcher struct {
	remotestate.RemoteStateWatcher
	changes  chan struct{}
	snapshot remotestate.Snapshot
}

func (w *mockRemoteStateWatcher) RemoteStateChanged() <-chan struct{} {
	return w.changes
}

func (w *mockRemoteStateWatcher) Snapshot() remotestate.Snapshot {
	return w.snapshot
}

type mockOpFactory struct {
	operation.Factory
	testing.Stub
}

func (f *mockOpFactory) NewUpgrade(charmURL *charm.URL) (operation.Operation, error) {
	f.MethodCall(f, "NewUpgrade", charmURL)
	return mockOp{}, f.NextErr()
}

func (f *mockOpFactory) NewRevertUpgrade(charmURL *charm.URL) (operation.Operation, error) {
	f.MethodCall(f, "NewRevertUpgrade", charmURL)
	return mockOp{}, f.NextErr()
}

func (f *mockOpFactory) NewResolvedUpgrade(charmURL *charm.URL) (operation.Operation, error) {
	f.MethodCall(f, "NewResolvedUpgrade", charmURL)
	return mockOp{}, f.NextErr()
}

func (f *mockOpFactory) NewRunHook(info hook.Info) (operation.Operation, error) {
	f.MethodCall(f, "NewRunHook", info)
	return mockOp{}, f.NextErr()
}

func (f *mockOpFactory) NewSkipHook(info hook.Info) (operation.Operation, error) {
	f.MethodCall(f, "NewSkipHook", info)
	return mockOp{}, f.NextErr()
}

type mockOpExecutor struct {
	operation.Executor
	testing.Stub
	st operation.State
}

func (e *mockOpExecutor) State() operation.State {
	e.MethodCall(e, "State", "State")
	return e.st
}

func (e *mockOpExecutor) Run(op operation.Operation) error {
	e.MethodCall(e, "Run", op)
	return e.NextErr()
}

type mockOp struct {
	operation.Operation
}
