// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package runcommands_test

import (
	"context"

	"github.com/juju/testing"
	"github.com/juju/utils/v4/exec"

	"github.com/juju/juju/internal/worker/uniter/operation"
	"github.com/juju/juju/internal/worker/uniter/runner"
	runnercontext "github.com/juju/juju/internal/worker/uniter/runner/context"
)

type mockRunnerFactory struct {
	runner.Factory
	newCommandRunner func(runnercontext.CommandInfo) (runner.Runner, error)
}

func (f *mockRunnerFactory) NewCommandRunner(_ context.Context, info runnercontext.CommandInfo) (runner.Runner, error) {
	return f.newCommandRunner(info)
}

type mockRunner struct {
	runner.Runner
	runCommands func(string, runner.RunLocation) (*exec.ExecResponse, error)
}

func (r *mockRunner) Context() runnercontext.Context {
	return &mockRunnerContext{}
}

func (r *mockRunner) RunCommands(_ context.Context, commands string, runLocation runner.RunLocation) (*exec.ExecResponse, error) {
	return r.runCommands(commands, runLocation)
}

type mockRunnerContext struct {
	runnercontext.Context
}

func (*mockRunnerContext) Prepare(context.Context) error {
	return nil
}

type mockCallbacks struct {
	testing.Stub
	operation.Callbacks
}

func (c *mockCallbacks) SetExecutingStatus(status string) error {
	c.MethodCall(c, "SetExecutingStatus", status)
	return c.NextErr()
}
