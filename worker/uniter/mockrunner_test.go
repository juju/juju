// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package uniter_test

import (
	stdcontext "context"
	"fmt"
	"sync"

	"github.com/juju/charm/v11/hooks"
	"github.com/juju/collections/set"
	"github.com/juju/errors"
	utilexec "github.com/juju/utils/v3/exec"

	"github.com/juju/juju/core/status"
	"github.com/juju/juju/worker/uniter/runner"
	"github.com/juju/juju/worker/uniter/runner/context"
)

// mockRunner implements Runner.
type mockRunner struct {
	stdContext context.Context

	mu              sync.Mutex
	ctx             *testContext
	hooksWithErrors set.Strings
	ranActions_     []actionData
}

func (r *mockRunner) Context() context.Context {
	return r.stdContext
}

func (r *mockRunner) ranActions() []actionData {
	r.mu.Lock()
	defer r.mu.Unlock()
	result := make([]actionData, len(r.ranActions_))
	for i, a := range r.ranActions_ {
		result[i] = a
	}
	return result
}

// RunCommands exists to satisfy the Runner interface.
func (r *mockRunner) RunCommands(ctx stdcontext.Context, commands string, runLocation runner.RunLocation) (*utilexec.ExecResponse, error) {
	result := &utilexec.ExecResponse{
		Code:   0,
		Stdout: []byte(fmt.Sprintf("%s on %s", commands, runLocation)),
	}
	return result, nil
}

// RunAction exists to satisfy the Runner interface.
func (r *mockRunner) RunAction(ctx stdcontext.Context, actionName string) (runner.HookHandlerType, error) {
	data, err := r.stdContext.ActionData()
	if err != nil {
		return runner.ExplicitHookHandler, errors.Trace(err)
	}
	params := data.Params
	command := actionName
	ranAction := actionData{actionName: command}
	for k, v := range params {
		ranAction.args = append(ranAction.args, fmt.Sprintf("%s=%s", k, v))
	}
	r.mu.Lock()
	r.ranActions_ = append(r.ranActions_, ranAction)
	r.mu.Unlock()
	return runner.ExplicitHookHandler, nil
}

// RunHook exists to satisfy the Runner interface.
func (r *mockRunner) RunHook(ctx stdcontext.Context, hookName string) (runner.HookHandlerType, error) {
	r.ctx.unit.mu.Lock()
	if hookName == string(hooks.Install) {
		r.ctx.unit.unitStatus = status.StatusInfo{
			Status:  status.Maintenance,
			Message: status.MessageInstallingCharm,
		}
	}
	r.ctx.unit.mu.Unlock()
	var err error = nil
	if r.hooksWithErrors != nil && r.hooksWithErrors.Contains(hookName) {
		err = errors.Errorf("%q failed", hookName)
	}
	return runner.ExplicitHookHandler, err
}
