// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package operation_test

import (
	"github.com/juju/errors"
	utilexec "github.com/juju/utils/exec"
	corecharm "gopkg.in/juju/charm.v4"
	"gopkg.in/juju/charm.v4/hooks"

	"github.com/juju/juju/worker/uniter/charm"
	"github.com/juju/juju/worker/uniter/context"
	"github.com/juju/juju/worker/uniter/hook"
	"github.com/juju/juju/worker/uniter/operation"
)

type MockGetArchiveInfo struct {
	gotCharmURL *corecharm.URL
	info        charm.BundleInfo
	err         error
}

func (mock *MockGetArchiveInfo) Call(charmURL *corecharm.URL) (charm.BundleInfo, error) {
	mock.gotCharmURL = charmURL
	return mock.info, mock.err
}

type MockSetCurrentCharm struct {
	gotCharmURL *corecharm.URL
	err         error
}

func (mock *MockSetCurrentCharm) Call(charmURL *corecharm.URL) error {
	mock.gotCharmURL = charmURL
	return mock.err
}

type DeployCallbacks struct {
	operation.Callbacks
	*MockGetArchiveInfo
	*MockSetCurrentCharm
}

func (cb *DeployCallbacks) GetArchiveInfo(charmURL *corecharm.URL) (charm.BundleInfo, error) {
	return cb.MockGetArchiveInfo.Call(charmURL)
}

func (cb *DeployCallbacks) SetCurrentCharm(charmURL *corecharm.URL) error {
	return cb.MockSetCurrentCharm.Call(charmURL)
}

type MockBundleInfo struct {
	charm.BundleInfo
}

type MockStage struct {
	gotInfo  *charm.BundleInfo
	gotAbort *<-chan struct{}
	err      error
}

func (mock *MockStage) Call(info charm.BundleInfo, abort <-chan struct{}) error {
	mock.gotInfo = &info
	mock.gotAbort = &abort
	return mock.err
}

type MockDeploy struct {
	called bool
	err    error
}

func (mock *MockDeploy) Call() error {
	mock.called = true
	return mock.err
}

type MockDeployer struct {
	charm.Deployer
	*MockStage
	*MockDeploy
}

func (d *MockDeployer) Stage(info charm.BundleInfo, abort <-chan struct{}) error {
	return d.MockStage.Call(info, abort)
}

func (d *MockDeployer) Deploy() error {
	return d.MockDeploy.Call()
}

type MockFailAction struct {
	gotActionId *string
	gotMessage  *string
	err         error
}

func (mock *MockFailAction) Call(actionId, message string) error {
	mock.gotActionId = &actionId
	mock.gotMessage = &message
	return mock.err
}

type MockAcquireExecutionLock struct {
	gotMessage *string
	didUnlock  bool
	err        error
}

func (mock *MockAcquireExecutionLock) Call(message string) (func(), error) {
	mock.gotMessage = &message
	if mock.err != nil {
		return nil, mock.err
	}
	return func() { mock.didUnlock = true }, nil
}

type RunActionCallbacks struct {
	operation.Callbacks
	*MockFailAction
	*MockAcquireExecutionLock
}

func (cb *RunActionCallbacks) FailAction(actionId, message string) error {
	return cb.MockFailAction.Call(actionId, message)
}

func (cb *RunActionCallbacks) AcquireExecutionLock(message string) (func(), error) {
	return cb.MockAcquireExecutionLock.Call(message)
}

type RunCommandsCallbacks struct {
	operation.Callbacks
	*MockAcquireExecutionLock
}

func (cb *RunCommandsCallbacks) AcquireExecutionLock(message string) (func(), error) {
	return cb.MockAcquireExecutionLock.Call(message)
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

type PrepareHookCallbacks struct {
	operation.Callbacks
	*MockPrepareHook
}

func (cb *PrepareHookCallbacks) PrepareHook(hookInfo hook.Info) (string, error) {
	return cb.MockPrepareHook.Call(hookInfo)
}

type MockNotify struct {
	gotName    *string
	gotContext *context.Context
}

func (mock *MockNotify) Call(hookName string, ctx context.Context) {
	mock.gotName = &hookName
	mock.gotContext = &ctx
}

type ExecuteHookCallbacks struct {
	*PrepareHookCallbacks
	*MockAcquireExecutionLock
	MockNotifyHookCompleted *MockNotify
	MockNotifyHookFailed    *MockNotify
}

func (cb *ExecuteHookCallbacks) AcquireExecutionLock(message string) (func(), error) {
	return cb.MockAcquireExecutionLock.Call(message)
}

func (cb *ExecuteHookCallbacks) NotifyHookCompleted(hookName string, ctx context.Context) {
	cb.MockNotifyHookCompleted.Call(hookName, ctx)
}

func (cb *ExecuteHookCallbacks) NotifyHookFailed(hookName string, ctx context.Context) {
	cb.MockNotifyHookFailed.Call(hookName, ctx)
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

type MockNewActionRunner struct {
	gotActionId *string
	runner      *MockRunner
	err         error
}

func (mock *MockNewActionRunner) Call(actionId string) (context.Runner, error) {
	mock.gotActionId = &actionId
	return mock.runner, mock.err
}

type MockNewHookRunner struct {
	gotHook *hook.Info
	runner  *MockRunner
	err     error
}

func (mock *MockNewHookRunner) Call(hookInfo hook.Info) (context.Runner, error) {
	mock.gotHook = &hookInfo
	return mock.runner, mock.err
}

type MockNewRunner struct {
	gotRelationId     *int
	gotRemoteUnitName *string
	runner            *MockRunner
	err               error
}

func (mock *MockNewRunner) Call(relationId int, remoteUnitName string) (context.Runner, error) {
	mock.gotRelationId = &relationId
	mock.gotRemoteUnitName = &remoteUnitName
	return mock.runner, mock.err
}

type MockRunnerFactory struct {
	*MockNewActionRunner
	*MockNewHookRunner
	*MockNewRunner
}

func (f *MockRunnerFactory) NewActionRunner(actionId string) (context.Runner, error) {
	return f.MockNewActionRunner.Call(actionId)
}

func (f *MockRunnerFactory) NewHookRunner(hookInfo hook.Info) (context.Runner, error) {
	return f.MockNewHookRunner.Call(hookInfo)
}

func (f *MockRunnerFactory) NewRunner(relationId int, remoteUnitName string) (context.Runner, error) {
	return f.MockNewRunner.Call(relationId, remoteUnitName)
}

type MockContext struct {
	context.Context
	actionData *context.ActionData
}

func (mock *MockContext) ActionData() (*context.ActionData, error) {
	if mock.actionData == nil {
		return nil, errors.New("not an action context")
	}
	return mock.actionData, nil
}

type MockRunAction struct {
	gotName *string
	err     error
}

func (mock *MockRunAction) Call(actionName string) error {
	mock.gotName = &actionName
	return mock.err
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
	*MockRunAction
	*MockRunCommands
	*MockRunHook
	context context.Context
}

func (r *MockRunner) Context() context.Context {
	return r.context
}

func (r *MockRunner) RunAction(actionName string) error {
	return r.MockRunAction.Call(actionName)
}

func (r *MockRunner) RunCommands(commands string) (*utilexec.ExecResponse, error) {
	return r.MockRunCommands.Call(commands)
}

func (r *MockRunner) RunHook(hookName string) error {
	return r.MockRunHook.Call(hookName)
}

func NewDeployCallbacks() *DeployCallbacks {
	return &DeployCallbacks{
		MockGetArchiveInfo:  &MockGetArchiveInfo{info: &MockBundleInfo{}},
		MockSetCurrentCharm: &MockSetCurrentCharm{},
	}
}

func NewMockDeployer() *MockDeployer {
	return &MockDeployer{
		MockStage:  &MockStage{},
		MockDeploy: &MockDeploy{},
	}
}

func NewPrepareHookCallbacks() *PrepareHookCallbacks {
	return &PrepareHookCallbacks{
		MockPrepareHook: &MockPrepareHook{nil, "some-hook-name", nil},
	}
}

func NewRunActionRunnerFactory(runErr error) *MockRunnerFactory {
	return &MockRunnerFactory{
		MockNewActionRunner: &MockNewActionRunner{
			runner: &MockRunner{
				MockRunAction: &MockRunAction{err: runErr},
				context: &MockContext{
					actionData: &context.ActionData{ActionName: "some-action-name"},
				},
			},
		},
	}
}

func NewRunCommandsRunnerFactory(runResponse *utilexec.ExecResponse, runErr error) *MockRunnerFactory {
	return &MockRunnerFactory{
		MockNewRunner: &MockNewRunner{
			runner: &MockRunner{
				MockRunCommands: &MockRunCommands{response: runResponse, err: runErr},
			},
		},
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

type MockSendResponse struct {
	gotResponse **utilexec.ExecResponse
	gotErr      *error
}

func (mock *MockSendResponse) Call(response *utilexec.ExecResponse, err error) {
	mock.gotResponse = &response
	mock.gotErr = &err
}

var curl = corecharm.MustParseURL
var someActionId = "f47ac10b-58cc-4372-a567-0e02b2c3d479"
var randomActionId = "9f484882-2f18-4fd2-967d-db9663db7bea"
var overwriteState = operation.State{
	Kind:               operation.Continue,
	Step:               operation.Pending,
	Started:            true,
	CollectMetricsTime: 1234567,
	CharmURL:           curl("cs:quantal/wordpress-2"),
	ActionId:           &randomActionId,
	Hook:               &hook.Info{Kind: hooks.Install},
}
