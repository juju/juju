// Copyright 2014-2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package operation_test

import (
	"sync"

	"github.com/juju/errors"
	"github.com/juju/testing"
	utilexec "github.com/juju/utils/exec"
	corecharm "gopkg.in/juju/charm.v6"
	"gopkg.in/juju/charm.v6/hooks"

	"github.com/juju/juju/core/model"
	"github.com/juju/juju/core/relation"
	"github.com/juju/juju/worker/uniter/charm"
	"github.com/juju/juju/worker/uniter/hook"
	"github.com/juju/juju/worker/uniter/operation"
	"github.com/juju/juju/worker/uniter/remotestate"
	"github.com/juju/juju/worker/uniter/runner"
	"github.com/juju/juju/worker/uniter/runner/context"
	"github.com/juju/juju/worker/uniter/runner/jujuc"
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
	MockInitializeMetricsTimers *MockNoArgs
}

func (cb *DeployCallbacks) GetArchiveInfo(charmURL *corecharm.URL) (charm.BundleInfo, error) {
	return cb.MockGetArchiveInfo.Call(charmURL)
}

func (cb *DeployCallbacks) SetCurrentCharm(charmURL *corecharm.URL) error {
	return cb.MockSetCurrentCharm.Call(charmURL)
}

func (cb *DeployCallbacks) InitializeMetricsTimers() error {
	return cb.MockInitializeMetricsTimers.Call()
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

type MockNoArgs struct {
	called bool
	err    error
}

func (mock *MockNoArgs) Call() error {
	mock.called = true
	return mock.err
}

type MockDeployer struct {
	charm.Deployer
	*MockStage
	MockDeploy         *MockNoArgs
	MockNotifyRevert   *MockNoArgs
	MockNotifyResolved *MockNoArgs
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

type RunActionCallbacks struct {
	operation.Callbacks
	*MockFailAction
	executingMessage string
	actionStatus     string
	actionStatusErr  error
	mut              sync.Mutex
}

func (cb *RunActionCallbacks) FailAction(actionId, message string) error {
	return cb.MockFailAction.Call(actionId, message)
}

func (cb *RunActionCallbacks) SetExecutingStatus(message string) error {
	cb.mut.Lock()
	defer cb.mut.Unlock()
	cb.executingMessage = message
	return nil
}

func (cb *RunActionCallbacks) ActionStatus(actionId string) (string, error) {
	cb.mut.Lock()
	defer cb.mut.Unlock()
	return cb.actionStatus, cb.actionStatusErr
}

func (cb *RunActionCallbacks) setActionStatus(status string, err error) {
	cb.mut.Lock()
	defer cb.mut.Unlock()
	cb.actionStatus = status
	cb.actionStatusErr = err
}

type RunCommandsCallbacks struct {
	operation.Callbacks
	executingMessage string
}

func (cb *RunCommandsCallbacks) SetExecutingStatus(message string) error {
	cb.executingMessage = message
	return nil
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
	executingMessage string
}

func (cb *PrepareHookCallbacks) PrepareHook(hookInfo hook.Info) (string, error) {
	return cb.MockPrepareHook.Call(hookInfo)
}

func (cb *PrepareHookCallbacks) SetExecutingStatus(message string) error {
	cb.executingMessage = message
	return nil
}

func (cb *PrepareHookCallbacks) SetUpgradeSeriesStatus(model.UpgradeSeriesStatus, string) error {
	return nil
}

type MockNotify struct {
	gotName    *string
	gotContext *runner.Context
}

func (mock *MockNotify) Call(hookName string, ctx runner.Context) {
	mock.gotName = &hookName
	mock.gotContext = &ctx
}

type ExecuteHookCallbacks struct {
	*PrepareHookCallbacks
	MockNotifyHookCompleted *MockNotify
	MockNotifyHookFailed    *MockNotify
}

func (cb *ExecuteHookCallbacks) NotifyHookCompleted(hookName string, ctx runner.Context) {
	cb.MockNotifyHookCompleted.Call(hookName, ctx)
}

func (cb *ExecuteHookCallbacks) NotifyHookFailed(hookName string, ctx runner.Context) {
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
	gotCancel   <-chan struct{}
	runner      *MockRunner
	err         error
}

func (mock *MockNewActionRunner) Call(actionId string, cancel <-chan struct{}) (runner.Runner, error) {
	mock.gotActionId = &actionId
	mock.gotCancel = cancel
	return mock.runner, mock.err
}

type MockNewActionWaitRunner struct {
	gotActionId *string
	gotCancel   <-chan struct{}
	runner      *MockActionWaitRunner
	err         error
}

func (mock *MockNewActionWaitRunner) Call(actionId string, cancel <-chan struct{}) (runner.Runner, error) {
	mock.gotActionId = &actionId
	mock.gotCancel = cancel
	mock.runner.context.(*MockContext).actionData.Cancel = cancel
	return mock.runner, mock.err
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
	*MockNewActionRunner
	*MockNewHookRunner
	*MockNewCommandRunner
}

func (f *MockRunnerFactory) NewActionRunner(actionId string, cancel <-chan struct{}) (runner.Runner, error) {
	return f.MockNewActionRunner.Call(actionId, cancel)
}

func (f *MockRunnerFactory) NewHookRunner(hookInfo hook.Info) (runner.Runner, error) {
	return f.MockNewHookRunner.Call(hookInfo)
}

func (f *MockRunnerFactory) NewCommandRunner(commandInfo context.CommandInfo) (runner.Runner, error) {
	return f.MockNewCommandRunner.Call(commandInfo)
}

type MockRunnerActionWaitFactory struct {
	runner.Factory
	*MockNewActionWaitRunner
}

func (f *MockRunnerActionWaitFactory) NewActionRunner(actionId string, cancel <-chan struct{}) (runner.Runner, error) {
	return f.MockNewActionWaitRunner.Call(actionId, cancel)
}

type MockContext struct {
	runner.Context
	testing.Stub
	actionData      *context.ActionData
	setStatusCalled bool
	status          jujuc.StatusInfo
	isLeader        bool
	relation        *MockRelation
}

func (mock *MockContext) ActionData() (*context.ActionData, error) {
	if mock.actionData == nil {
		return nil, errors.New("not an action context")
	}
	return mock.actionData, nil
}

func (mock *MockContext) HasExecutionSetUnitStatus() bool {
	return mock.setStatusCalled
}

func (mock *MockContext) ResetExecutionSetUnitStatus() {
	mock.setStatusCalled = false
}

func (mock *MockContext) SetUnitStatus(status jujuc.StatusInfo) error {
	mock.setStatusCalled = true
	mock.status = status
	return nil
}

func (mock *MockContext) UnitName() string {
	return "unit/0"
}

func (mock *MockContext) UnitStatus() (*jujuc.StatusInfo, error) {
	return &mock.status, nil
}

func (mock *MockContext) Prepare() error {
	mock.MethodCall(mock, "Prepare")
	return mock.NextErr()
}

func (mock *MockContext) IsLeader() (bool, error) {
	return mock.isLeader, nil
}

func (mock *MockContext) Relation(id int) (jujuc.ContextRelation, error) {
	return mock.relation, nil
}

type MockRelation struct {
	jujuc.ContextRelation
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
	gotName         *string
	err             error
	setStatusCalled bool
}

func (mock *MockRunHook) Call(hookName string) error {
	mock.gotName = &hookName
	return mock.err
}

type MockRunner struct {
	*MockRunAction
	*MockRunCommands
	*MockRunHook
	context runner.Context
}

func (r *MockRunner) Context() runner.Context {
	return r.context
}

func (r *MockRunner) RunAction(actionName string) (runner.HookHandlerType, error) {
	return runner.ExplicitHookHandler, r.MockRunAction.Call(actionName)
}

func (r *MockRunner) RunCommands(commands string) (*utilexec.ExecResponse, error) {
	return r.MockRunCommands.Call(commands)
}

func (r *MockRunner) RunHook(hookName string) (runner.HookHandlerType, error) {
	r.Context().(*MockContext).setStatusCalled = r.MockRunHook.setStatusCalled
	return runner.ExplicitHookHandler, r.MockRunHook.Call(hookName)
}

type MockActionWaitRunner struct {
	runner.Runner

	context    runner.Context
	actionChan <-chan error

	actionName string
}

func (r *MockActionWaitRunner) Context() runner.Context {
	return r.context
}

func (r *MockActionWaitRunner) RunAction(actionName string) (runner.HookHandlerType, error) {
	r.actionName = actionName
	return runner.ExplicitHookHandler, <-r.actionChan
}

func NewDeployCallbacks() *DeployCallbacks {
	return &DeployCallbacks{
		MockGetArchiveInfo:  &MockGetArchiveInfo{info: &MockBundleInfo{}},
		MockSetCurrentCharm: &MockSetCurrentCharm{},
	}
}

func NewDeployCommitCallbacks(err error) *DeployCallbacks {
	return &DeployCallbacks{
		MockInitializeMetricsTimers: &MockNoArgs{err: err},
	}
}
func NewMockDeployer() *MockDeployer {
	return &MockDeployer{
		MockStage:          &MockStage{},
		MockDeploy:         &MockNoArgs{},
		MockNotifyRevert:   &MockNoArgs{},
		MockNotifyResolved: &MockNoArgs{},
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
					actionData: &context.ActionData{Name: "some-action-name"},
				},
			},
		},
	}
}

func NewRunActionWaitRunnerFactory(actionChan <-chan error) *MockRunnerActionWaitFactory {
	return &MockRunnerActionWaitFactory{
		MockNewActionWaitRunner: &MockNewActionWaitRunner{
			runner: &MockActionWaitRunner{
				actionChan: actionChan,
				context: &MockContext{
					actionData: &context.ActionData{
						Name: "some-action-name",
					},
				},
			},
		},
	}
}

func NewRunCommandsRunnerFactory(runResponse *utilexec.ExecResponse, runErr error) *MockRunnerFactory {
	return &MockRunnerFactory{
		MockNewCommandRunner: &MockNewCommandRunner{
			runner: &MockRunner{
				MockRunCommands: &MockRunCommands{response: runResponse, err: runErr},
				context:         &MockContext{},
			},
		},
	}
}

func NewRunHookRunnerFactory(runErr error, contextOps ...func(*MockContext)) *MockRunnerFactory {
	ctx := &MockContext{isLeader: true}
	for _, op := range contextOps {
		op(ctx)
	}

	return &MockRunnerFactory{
		MockNewHookRunner: &MockNewHookRunner{
			runner: &MockRunner{
				MockRunHook: &MockRunHook{err: runErr},
				context:     ctx,
			},
		},
	}
}

type MockSendResponse struct {
	gotResponse **utilexec.ExecResponse
	gotErr      *error
	eatError    bool
}

func (mock *MockSendResponse) Call(response *utilexec.ExecResponse, err error) bool {
	mock.gotResponse = &response
	mock.gotErr = &err
	return mock.eatError
}

var curl = corecharm.MustParseURL
var someActionId = "f47ac10b-58cc-4372-a567-0e02b2c3d479"
var randomActionId = "9f484882-2f18-4fd2-967d-db9663db7bea"
var overwriteState = operation.State{
	Kind:     operation.Continue,
	Step:     operation.Pending,
	Started:  true,
	CharmURL: curl("cs:quantal/wordpress-2"),
	ActionId: &randomActionId,
	Hook:     &hook.Info{Kind: hooks.Install},
}
var someCommandArgs = operation.CommandArgs{
	Commands:        "do something",
	RelationId:      123,
	RemoteUnitName:  "foo/456",
	ForceRemoteUnit: true,
}

type RemoteInitCallbacks struct {
	operation.Callbacks
	MockRemoteInit *MockRemoteInit
}

func (cb *RemoteInitCallbacks) RemoteInit(runningStatus remotestate.ContainerRunningStatus, abort <-chan struct{}) error {
	return cb.MockRemoteInit.Call(runningStatus, abort)
}

type MockRemoteInit struct {
	gotRunningStatus *remotestate.ContainerRunningStatus
	gotAbort         <-chan struct{}
	err              error
}

func (mock *MockRemoteInit) Call(runningStatus remotestate.ContainerRunningStatus, abort <-chan struct{}) error {
	mock.gotRunningStatus = &runningStatus
	mock.gotAbort = abort
	return mock.err
}
