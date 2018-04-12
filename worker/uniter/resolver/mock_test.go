// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package resolver_test

import (
	"github.com/juju/testing"
	"gopkg.in/juju/charm.v6"

	"github.com/juju/juju/worker/fortress"
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
	op mockOp
}

func (f *mockOpFactory) NewUpgrade(charmURL *charm.URL) (operation.Operation, error) {
	f.MethodCall(f, "NewUpgrade", charmURL)
	return f.op, f.NextErr()
}

func (f *mockOpFactory) NewNoOpUpgrade(charmURL *charm.URL) (operation.Operation, error) {
	f.MethodCall(f, "NewNoOpUpgrade", charmURL)
	return f.op, f.NextErr()
}

func (f *mockOpFactory) NewRevertUpgrade(charmURL *charm.URL) (operation.Operation, error) {
	f.MethodCall(f, "NewRevertUpgrade", charmURL)
	return f.op, f.NextErr()
}

func (f *mockOpFactory) NewResolvedUpgrade(charmURL *charm.URL) (operation.Operation, error) {
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

func (f *mockOpFactory) NewAction(id string) (operation.Operation, error) {
	f.MethodCall(f, "NewAction", id)
	return f.op, f.NextErr()
}

type mockOpExecutor struct {
	operation.Executor
	testing.Stub
	st operation.State
}

func (e *mockOpExecutor) State() operation.State {
	e.MethodCall(e, "State")
	return e.st
}

func (e *mockOpExecutor) Run(op operation.Operation) error {
	e.MethodCall(e, "Run", op)
	return e.NextErr()
}

type mockOp struct {
	operation.Operation
	commit func(operation.State) (*operation.State, error)
}

func (op mockOp) Commit(st operation.State) (*operation.State, error) {
	if op.commit != nil {
		return op.commit(st)
	}
	return &st, nil
}

type mockCharmDirGuard struct {
	fortress.Guard
	testing.Stub
	commit func(operation.State) (*operation.State, error)
}

func (l *mockCharmDirGuard) Unlock() error {
	l.MethodCall(l, "Unlock")
	return l.NextErr()
}

func (l *mockCharmDirGuard) Lockdown(abort fortress.Abort) error {
	l.MethodCall(l, "Lockdown", abort)
	return l.NextErr()
}
