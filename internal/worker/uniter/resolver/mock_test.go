// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package resolver_test

import (
	"context"

	"github.com/juju/juju/internal/testhelpers"
	"github.com/juju/juju/internal/worker/fortress"
	"github.com/juju/juju/internal/worker/uniter/hook"
	"github.com/juju/juju/internal/worker/uniter/operation"
	"github.com/juju/juju/internal/worker/uniter/remotestate"
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
	testhelpers.Stub
	op mockOp
}

func (f *mockOpFactory) NewInstall(charmURL string) (operation.Operation, error) {
	f.MethodCall(f, "NewInstall", charmURL)
	return f.op, f.NextErr()
}

func (f *mockOpFactory) NewUpgrade(charmURL string) (operation.Operation, error) {
	f.MethodCall(f, "NewUpgrade", charmURL)
	return f.op, f.NextErr()
}

func (f *mockOpFactory) NewRevertUpgrade(charmURL string) (operation.Operation, error) {
	f.MethodCall(f, "NewRevertUpgrade", charmURL)
	return f.op, f.NextErr()
}

func (f *mockOpFactory) NewResolvedUpgrade(charmURL string) (operation.Operation, error) {
	f.MethodCall(f, "NewResolvedUpgrade", charmURL)
	return f.op, f.NextErr()
}

func (f *mockOpFactory) NewRunHook(info hook.Info) (operation.Operation, error) {
	f.MethodCall(f, "NewRunHook", info)
	return f.op, f.NextErr()
}

func (f *mockOpFactory) NewSkipHook(info hook.Info) (operation.Operation, error) {
	f.MethodCall(f, "NewSkipHook", info)
	return f.op, f.NextErr()
}

func (f *mockOpFactory) NewAction(_ context.Context, id string) (operation.Operation, error) {
	f.MethodCall(f, "NewAction", id)
	return f.op, f.NextErr()
}

func (f *mockOpFactory) NewFailAction(id string) (operation.Operation, error) {
	f.MethodCall(f, "NewFailAction", id)
	return f.op, f.NextErr()
}

type mockOpExecutor struct {
	operation.Executor
	testhelpers.Stub
	st  operation.State
	run func(operation.Operation, <-chan remotestate.Snapshot) error
}

func (e *mockOpExecutor) State() operation.State {
	e.MethodCall(e, "State")
	return e.st
}

func (e *mockOpExecutor) Run(ctx context.Context, op operation.Operation, rs <-chan remotestate.Snapshot) error {
	e.MethodCall(e, "Run", op, rs)
	if e.run != nil {
		return e.run(op, rs)
	}
	return e.NextErr()
}

type mockOp struct {
	operation.Operation
	commit  func(operation.State) (*operation.State, error)
	prepare func(operation.State) (*operation.State, error)
}

func (op mockOp) Prepare(ctx context.Context, st operation.State) (*operation.State, error) {
	if op.prepare != nil {
		return op.prepare(st)
	}
	return &st, nil
}

func (op mockOp) Commit(ctx context.Context, st operation.State) (*operation.State, error) {
	if op.commit != nil {
		return op.commit(st)
	}
	return &st, nil
}

type mockCharmDirGuard struct {
	fortress.Guard
	testhelpers.Stub
}

func (l *mockCharmDirGuard) Unlock() error {
	l.MethodCall(l, "Unlock")
	return l.NextErr()
}

func (l *mockCharmDirGuard) Lockdown(abort fortress.Abort) error {
	l.MethodCall(l, "Lockdown", abort)
	return l.NextErr()
}
