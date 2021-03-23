// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package uniter_test

import (
	"fmt"
	"sync"

	"github.com/juju/collections/set"
	"github.com/juju/errors"
	utilexec "github.com/juju/utils/v2/exec"

	"github.com/juju/juju/worker/uniter/runner"
)

// mockRunner implements Runner.
type mockRunner struct {
	ctx runner.Context

	mu              sync.Mutex
	hooksWithErrors set.Strings
	ranActions_     []actionData
}

func (r *mockRunner) Context() runner.Context {
	return r.ctx
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
func (r *mockRunner) RunCommands(commands string, runLocation runner.RunLocation) (*utilexec.ExecResponse, error) {
	result := &utilexec.ExecResponse{
		Code:   0,
		Stdout: []byte(fmt.Sprintf("%s on %s", commands, runLocation)),
	}
	return result, nil
}

// RunAction exists to satisfy the Runner interface.
func (r *mockRunner) RunAction(actionName string) (runner.HookHandlerType, error) {
	data, err := r.ctx.ActionData()
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
func (r *mockRunner) RunHook(hookName string) (runner.HookHandlerType, error) {
	var err error = nil
	if r.hooksWithErrors != nil && r.hooksWithErrors.Contains(hookName) {
		err = errors.Errorf("%q failed", hookName)
	}
	return runner.ExplicitHookHandler, err
}
