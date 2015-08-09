// Copyright 2014-2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package operation_test

import (
	"github.com/juju/errors"
	"github.com/juju/testing"
	utilexec "github.com/juju/utils/exec"
	corecharm "gopkg.in/juju/charm.v5"
	"gopkg.in/juju/charm.v5/hooks"

	"github.com/juju/juju/worker/uniter/charm"
	"github.com/juju/juju/worker/uniter/hook"
	"github.com/juju/juju/worker/uniter/operation"
	"github.com/juju/juju/worker/uniter/runner"
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
	MockClearResolvedFlag       *MockNoArgs
	MockInitializeMetricsTimers *MockNoArgs
}

func (cb *DeployCallbacks) GetArchiveInfo(charmURL *corecharm.URL) (charm.BundleInfo, error) {
	return cb.MockGetArchiveInfo.Call(charmURL)
}

func (cb *DeployCallbacks) SetCurrentCharm(charmURL *corecharm.URL) error {
	return cb.MockSetCurrentCharm.Call(charmURL)
}

func (cb *DeployCallbacks) ClearResolvedFlag() error {
	return cb.MockClearResolvedFlag.Call()
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

func (d *MockDeployer) NotifyRevert() error {
	return d.MockNotifyRevert.Call()
}

func (d *MockDeployer) NotifyResolved() error {
	return d.MockNotifyResolved.Call()
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
}

func (cb *RunActionCallbacks) FailAction(actionId, message string) error {
	return cb.MockFailAction.Call(actionId, message)
}

func (cb *RunActionCallbacks) SetExecutingStatus(message string) error {
	cb.executingMessage = message
	return nil
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
	MockClearResolvedFlag *MockNoArgs
	executingMessage      string
}

func (cb *PrepareHookCallbacks) PrepareHook(hookInfo hook.Info) (string, error) {
	return cb.MockPrepareHook.Call(hookInfo)
}

func (cb *PrepareHookCallbacks) ClearResolvedFlag() error {
	return cb.MockClearResolvedFlag.Call()
}

func (cb *PrepareHookCallbacks) SetExecutingStatus(message string) error {
	cb.executingMessage = message
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

type MockUpdateRelations struct {
	gotIds *[]int
	err    error
}

func (mock *MockUpdateRelations) Call(ids []int) error {
	mock.gotIds = &ids
	return mock.err
}

type UpdateRelationsCallbacks struct {
	operation.Callbacks
	*MockUpdateRelations
}

func (cb *UpdateRelationsCallbacks) UpdateRelations(ids []int) error {
	return cb.MockUpdateRelations.Call(ids)
}

type MockNewActionRunner struct {
	gotActionId *string
	runner      *MockRunner
	err         error
}

func (mock *MockNewActionRunner) Call(actionId string) (runner.Runner, error) {
	mock.gotActionId = &actionId
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
	gotInfo *runner.CommandInfo
	runner  *MockRunner
	err     error
}

func (mock *MockNewCommandRunner) Call(info runner.CommandInfo) (runner.Runner, error) {
	mock.gotInfo = &info
	return mock.runner, mock.err
}

type MockRunnerFactory struct {
	*MockNewActionRunner
	*MockNewHookRunner
	*MockNewCommandRunner
}

func (f *MockRunnerFactory) NewActionRunner(actionId string) (runner.Runner, error) {
	return f.MockNewActionRunner.Call(actionId)
}

func (f *MockRunnerFactory) NewHookRunner(hookInfo hook.Info) (runner.Runner, error) {
	return f.MockNewHookRunner.Call(hookInfo)
}

func (f *MockRunnerFactory) NewCommandRunner(commandInfo runner.CommandInfo) (runner.Runner, error) {
	return f.MockNewCommandRunner.Call(commandInfo)
}

type MockContext struct {
	runner.Context
	testing.Stub
	actionData      *runner.ActionData
	setStatusCalled bool
	status          jujuc.StatusInfo
}

func (mock *MockContext) ActionData() (*runner.ActionData, error) {
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

func (mock *MockContext) UnitStatus() (*jujuc.StatusInfo, error) {
	return &mock.status, nil
}

func (mock *MockContext) Prepare() error {
	mock.MethodCall(mock, "Prepare")
	return mock.NextErr()
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

func (r *MockRunner) RunAction(actionName string) error {
	return r.MockRunAction.Call(actionName)
}

func (r *MockRunner) RunCommands(commands string) (*utilexec.ExecResponse, error) {
	return r.MockRunCommands.Call(commands)
}

func (r *MockRunner) RunHook(hookName string) error {
	r.Context().(*MockContext).setStatusCalled = r.MockRunHook.setStatusCalled
	return r.MockRunHook.Call(hookName)
}

func NewDeployCallbacks() *DeployCallbacks {
	return &DeployCallbacks{
		MockGetArchiveInfo:    &MockGetArchiveInfo{info: &MockBundleInfo{}},
		MockSetCurrentCharm:   &MockSetCurrentCharm{},
		MockClearResolvedFlag: &MockNoArgs{},
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
		MockPrepareHook:       &MockPrepareHook{nil, "some-hook-name", nil},
		MockClearResolvedFlag: &MockNoArgs{},
	}
}

func NewRunActionRunnerFactory(runErr error) *MockRunnerFactory {
	return &MockRunnerFactory{
		MockNewActionRunner: &MockNewActionRunner{
			runner: &MockRunner{
				MockRunAction: &MockRunAction{err: runErr},
				context: &MockContext{
					actionData: &runner.ActionData{Name: "some-action-name"},
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
	UpdateStatusTime:   1234567,
	CharmURL:           curl("cs:quantal/wordpress-2"),
	ActionId:           &randomActionId,
	Hook:               &hook.Info{Kind: hooks.Install},
}
var someCommandArgs = operation.CommandArgs{
	Commands:        "do something",
	RelationId:      123,
	RemoteUnitName:  "foo/456",
	ForceRemoteUnit: true,
}
