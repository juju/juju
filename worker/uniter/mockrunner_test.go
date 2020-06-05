// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package uniter_test

import (
	"fmt"

	"github.com/juju/collections/set"
	"github.com/juju/errors"
	utilexec "github.com/juju/utils/exec"

	"github.com/juju/juju/worker/uniter/runner"
)

// mockRunner implements Runner.
type mockRunner struct {
	ctx runner.Context

	hooksWithErrors set.Strings
	ranActions      []actionData
}

func (r *mockRunner) Context() runner.Context {
	return r.ctx
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
	r.ranActions = append(r.ranActions, ranAction)
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
