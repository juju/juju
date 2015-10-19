// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package uniter_test

import (
	"github.com/juju/names"
	"github.com/juju/utils/exec"

	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/worker/uniter/hook"
	"github.com/juju/juju/worker/uniter/operation"
	"github.com/juju/juju/worker/uniter/relation"
	"github.com/juju/juju/worker/uniter/remotestate"
	"github.com/juju/juju/worker/uniter/resolver"
	"github.com/juju/juju/worker/uniter/runner"
	runnercontext "github.com/juju/juju/worker/uniter/runner/context"
	"github.com/juju/juju/worker/uniter/storage"
)

type dummyRelations struct {
	relation.Relations
}

func (*dummyRelations) NextHook(_ resolver.LocalState, _ remotestate.Snapshot) (hook.Info, error) {
	return hook.Info{}, resolver.ErrNoOperation
}

type dummyStorageAccessor struct {
	storage.StorageAccessor
}

func (*dummyStorageAccessor) UnitStorageAttachments(_ names.UnitTag) ([]params.StorageAttachmentId, error) {
	return nil, nil
}

type dummyRunnerFactory struct {
	runner.Factory
	newCommandRunner func(runnercontext.CommandInfo) (runner.Runner, error)
}

func (f *dummyRunnerFactory) NewCommandRunner(info runnercontext.CommandInfo) (runner.Runner, error) {
	return f.newCommandRunner(info)
}

type dummyRunner struct {
	runner.Runner
	runCommands func(string) (*exec.ExecResponse, error)
}

func (r *dummyRunner) Context() runner.Context {
	return &dummyRunnerContext{}
}

func (r *dummyRunner) RunCommands(commands string) (*exec.ExecResponse, error) {
	return r.runCommands(commands)
}

type dummyRunnerContext struct {
	runner.Context
}

func (*dummyRunnerContext) Prepare() error {
	return nil
}

type dummyCallbacks struct {
	operation.Callbacks
}

func (c *dummyCallbacks) SetExecutingStatus(string) error {
	return nil
}
