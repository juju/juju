// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package operation_test

import (
	"github.com/juju/juju/worker/caasoperator/runner/context"
	"github.com/juju/testing"
	utilexec "github.com/juju/utils/exec"
	"gopkg.in/juju/charm.v6/hooks"

	"github.com/juju/juju/core/relation"
	"github.com/juju/juju/worker/caasoperator/commands"
	"github.com/juju/juju/worker/caasoperator/hook"
	"github.com/juju/juju/worker/caasoperator/operation"
	"github.com/juju/juju/worker/caasoperator/runner"
)

func unitHooks() []hooks.Kind {
	return []hooks.Kind{
		hooks.ConfigChanged,
		hooks.UpdateStatus,
	}
}

type MockPrepareHook struct {
	gotHook *hook.Info
	name    string
	err     error
}

func (mock *MockPrepareHook) Call(hookInfo hook.Info) (string, error) {
	mock.gotHook = &hookInfo
	return mock.name, mock.err
}

type ExecuteHookCallbacks struct {
	operation.Callbacks
	*MockPrepareHook
	executingMessage string
}

func (cb *ExecuteHookCallbacks) PrepareHook(hookInfo hook.Info) (string, error) {
	return cb.MockPrepareHook.Call(hookInfo)
}

func (cb *ExecuteHookCallbacks) SetExecutingStatus(message string) error {
	cb.executingMessage = message
	return nil
}

type MockCommitHook struct {
	gotHook *hook.Info
	err     error
}

func (mock *MockCommitHook) Call(hookInfo hook.Info) error {
	mock.gotHook = &hookInfo
	return mock.err
}

type CommitHookCallbacks struct {
	operation.Callbacks
	*MockCommitHook
}

func (cb *CommitHookCallbacks) CommitHook(hookInfo hook.Info) error {
	return cb.MockCommitHook.Call(hookInfo)
}

type MockNewHookRunner struct {
	gotHook *hook.Info
	runner  *MockRunner
	err     error
}

func (mock *MockNewHookRunner) Call(hookInfo hook.Info) (runner.Runner, error) {
	mock.gotHook = &hookInfo
	return mock.runner, mock.err
}

type MockNewCommandRunner struct {
	gotInfo *context.CommandInfo
	runner  *MockRunner
	err     error
}

func (mock *MockNewCommandRunner) Call(info context.CommandInfo) (runner.Runner, error) {
	mock.gotInfo = &info
	return mock.runner, mock.err
}

type MockRunnerFactory struct {
	*MockNewHookRunner
	*MockNewCommandRunner
}

func (f *MockRunnerFactory) NewHookRunner(hookInfo hook.Info) (runner.Runner, error) {
	return f.MockNewHookRunner.Call(hookInfo)
}

func (f *MockRunnerFactory) NewCommandRunner(commandInfo context.CommandInfo) (runner.Runner, error) {
	return f.MockNewCommandRunner.Call(commandInfo)
}

type MockContext struct {
	runner.Context
	testing.Stub
	setStatusCalled bool
	status          commands.StatusInfo
	isLeader        bool
	relation        *MockRelation
}

func (mock *MockContext) SetUnitStatus(status commands.StatusInfo) error {
	mock.setStatusCalled = true
	mock.status = status
	return nil
}

func (mock *MockContext) UnitName() string {
	return "unit/0"
}

func (mock *MockContext) UnitStatus() (*commands.StatusInfo, error) {
	return &mock.status, nil
}

func (mock *MockContext) Prepare() error {
	mock.MethodCall(mock, "Prepare")
	return mock.NextErr()
}

func (mock *MockContext) Relation(id int) (commands.ContextRelation, error) {
	return mock.relation, nil
}

type MockRelation struct {
	commands.ContextRelation
	suspended bool
	status    relation.Status
}

func (mock *MockRelation) Suspended() bool {
	return mock.suspended
}

func (mock *MockRelation) SetStatus(status relation.Status) error {
	mock.status = status
	return nil
}

type MockRunCommands struct {
	gotCommands *string
	response    *utilexec.ExecResponse
	err         error
}

func (mock *MockRunCommands) Call(commands string) (*utilexec.ExecResponse, error) {
	mock.gotCommands = &commands
	return mock.response, mock.err
}

type MockRunHook struct {
	gotName *string
	err     error
}

func (mock *MockRunHook) Call(hookName string) error {
	mock.gotName = &hookName
	return mock.err
}

type MockRunner struct {
	*MockRunCommands
	*MockRunHook
	context runner.Context
}

func (r *MockRunner) Context() runner.Context {
	return r.context
}

func (r *MockRunner) RunCommands(commands string) (*utilexec.ExecResponse, error) {
	return r.MockRunCommands.Call(commands)
}

func (r *MockRunner) RunHook(hookName string) error {
	return r.MockRunHook.Call(hookName)
}

func newExecuteHookCallbacks() *ExecuteHookCallbacks {
	return &ExecuteHookCallbacks{
		MockPrepareHook: &MockPrepareHook{nil, "some-hook-name", nil},
	}
}

func NewRunHookRunnerFactory(runErr error) *MockRunnerFactory {
	return &MockRunnerFactory{
		MockNewHookRunner: &MockNewHookRunner{
			runner: &MockRunner{
				MockRunHook: &MockRunHook{err: runErr},
				context:     &MockContext{},
			},
		},
	}
}

var overwriteState = operation.State{
	Kind: operation.Continue,
	Step: operation.Pending,
	Hook: &hook.Info{Kind: hooks.ConfigChanged},
}
