// Copyright 2014-2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package operation_test

import (
	"context"
	"sync"

	"github.com/juju/errors"
	utilexec "github.com/juju/utils/v4/exec"

	"github.com/juju/juju/api/agent/uniter"
	"github.com/juju/juju/core/relation"
	"github.com/juju/juju/core/secrets"
	"github.com/juju/juju/internal/charm/hooks"
	"github.com/juju/juju/internal/testhelpers"
	"github.com/juju/juju/internal/worker/uniter/charm"
	"github.com/juju/juju/internal/worker/uniter/hook"
	"github.com/juju/juju/internal/worker/uniter/operation"
	"github.com/juju/juju/internal/worker/uniter/runner"
	runnercontext "github.com/juju/juju/internal/worker/uniter/runner/context"
	"github.com/juju/juju/internal/worker/uniter/runner/jujuc"
)

type MockGetArchiveInfo struct {
	gotCharmURL string
	info        charm.BundleInfo
	err         error
}

func (mock *MockGetArchiveInfo) Call(charmURL string) (charm.BundleInfo, error) {
	mock.gotCharmURL = charmURL
	return mock.info, mock.err
}

type MockSetCurrentCharm struct {
	gotCharmURL string
	err         error
}

func (mock *MockSetCurrentCharm) Call(charmURL string) error {
	mock.gotCharmURL = charmURL
	return mock.err
}

type DeployCallbacks struct {
	operation.Callbacks
	*MockGetArchiveInfo
	*MockSetCurrentCharm
	MockInitializeMetricsTimers *MockNoArgs
}

func (cb *DeployCallbacks) GetArchiveInfo(charmURL string) (charm.BundleInfo, error) {
	return cb.MockGetArchiveInfo.Call(charmURL)
}

func (cb *DeployCallbacks) SetCurrentCharm(ctx context.Context, charmURL string) error {
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

func (d *MockDeployer) Stage(ctx context.Context, info charm.BundleInfo, abort <-chan struct{}) error {
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

func (cb *RunActionCallbacks) FailAction(_ context.Context, actionId, message string) error {
	return cb.MockFailAction.Call(actionId, message)
}

func (cb *RunActionCallbacks) SetExecutingStatus(_ context.Context, message string) error {
	cb.mut.Lock()
	defer cb.mut.Unlock()
	cb.executingMessage = message
	return nil
}

func (cb *RunActionCallbacks) ActionStatus(_ context.Context, actionId string) (string, error) {
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

func (cb *RunCommandsCallbacks) SetExecutingStatus(_ context.Context, message string) error {
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

func (cb *PrepareHookCallbacks) PrepareHook(_ context.Context, hookInfo hook.Info) (string, error) {
	return cb.MockPrepareHook.Call(hookInfo)
}

func (cb *PrepareHookCallbacks) SetExecutingStatus(_ context.Context, message string) error {
	cb.executingMessage = message
	return nil
}

type MockNotify struct {
	gotName    *string
	gotContext *runnercontext.Context
}

func (mock *MockNotify) Call(hookName string, ctx runnercontext.Context) {
	mock.gotName = &hookName
	mock.gotContext = &ctx
}

type ExecuteHookCallbacks struct {
	*PrepareHookCallbacks
	MockNotifyHookCompleted *MockNotify
	MockNotifyHookFailed    *MockNotify
}

func (cb *ExecuteHookCallbacks) NotifyHookCompleted(hookName string, ctx runnercontext.Context) {
	cb.MockNotifyHookCompleted.Call(hookName, ctx)
}

func (cb *ExecuteHookCallbacks) NotifyHookFailed(hookName string, ctx runnercontext.Context) {
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

	rotatedSecretURI   string
	rotatedOldRevision int
}

func (cb *CommitHookCallbacks) PrepareHook(_ context.Context, hookInfo hook.Info) (string, error) {
	return "", nil
}

func (cb *CommitHookCallbacks) CommitHook(_ context.Context, hookInfo hook.Info) error {
	return cb.MockCommitHook.Call(hookInfo)
}

func (cb *CommitHookCallbacks) SetSecretRotated(_ context.Context, url string, oldRevision int) error {
	cb.rotatedSecretURI = url
	cb.rotatedOldRevision = oldRevision
	return nil
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
	gotInfo *runnercontext.CommandInfo
	runner  *MockRunner
	err     error
}

func (mock *MockNewCommandRunner) Call(info runnercontext.CommandInfo) (runner.Runner, error) {
	mock.gotInfo = &info
	return mock.runner, mock.err
}

type MockRunnerFactory struct {
	*MockNewActionRunner
	*MockNewHookRunner
	*MockNewCommandRunner
}

func (f *MockRunnerFactory) NewActionRunner(_ context.Context, action *uniter.Action, cancel <-chan struct{}) (runner.Runner, error) {
	return f.MockNewActionRunner.Call(action.ID(), cancel)
}

func (f *MockRunnerFactory) NewHookRunner(_ context.Context, hookInfo hook.Info) (runner.Runner, error) {
	return f.MockNewHookRunner.Call(hookInfo)
}

func (f *MockRunnerFactory) NewCommandRunner(_ context.Context, commandInfo runnercontext.CommandInfo) (runner.Runner, error) {
	return f.MockNewCommandRunner.Call(commandInfo)
}

type MockRunnerActionWaitFactory struct {
	runner.Factory
	*MockNewActionWaitRunner
}

func (f *MockRunnerActionWaitFactory) NewActionRunner(_ context.Context, action *uniter.Action, cancel <-chan struct{}) (runner.Runner, error) {
	return f.MockNewActionWaitRunner.Call(action.ID(), cancel)
}

type MockContext struct {
	runnercontext.Context
	testhelpers.Stub
	actionData      *runnercontext.ActionData
	setStatusCalled bool
	status          jujuc.StatusInfo
	isLeader        bool
	relation        *MockRelation
}

func (mock *MockContext) SecretMetadata() (map[string]jujuc.SecretMetadata, error) {
	return map[string]jujuc.SecretMetadata{
		"9m4e2mr0ui3e8a215n4g": {
			Description:    "description",
			Label:          "label",
			Owner:          secrets.Owner{Kind: secrets.ApplicationOwner, ID: "mariadb"},
			RotatePolicy:   secrets.RotateHourly,
			LatestRevision: 666,
			LatestChecksum: "deadbeef",
		},
	}, nil
}

func (mock *MockContext) ActionData() (*runnercontext.ActionData, error) {
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

func (mock *MockContext) SetUnitStatus(_ context.Context, status jujuc.StatusInfo) error {
	mock.setStatusCalled = true
	mock.status = status
	return nil
}

func (mock *MockContext) UnitName() string {
	return "unit/0"
}

func (mock *MockContext) UnitStatus(_ context.Context) (*jujuc.StatusInfo, error) {
	return &mock.status, nil
}

func (mock *MockContext) Prepare(context.Context) error {
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

func (mock *MockRelation) SetStatus(_ context.Context, status relation.Status) error {
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
	context runnercontext.Context
}

func (r *MockRunner) Context() runnercontext.Context {
	return r.context
}

func (r *MockRunner) RunAction(ctx context.Context, actionName string) (runner.HookHandlerType, error) {
	return runner.ExplicitHookHandler, r.MockRunAction.Call(actionName)
}

func (r *MockRunner) RunCommands(ctx context.Context, commands string) (*utilexec.ExecResponse, error) {
	return r.MockRunCommands.Call(commands)
}

func (r *MockRunner) RunHook(ctx context.Context, hookName string) (runner.HookHandlerType, error) {
	r.Context().(*MockContext).setStatusCalled = r.MockRunHook.setStatusCalled
	return runner.ExplicitHookHandler, r.MockRunHook.Call(hookName)
}

type MockActionWaitRunner struct {
	runner.Runner

	context    runnercontext.Context
	actionChan <-chan error

	actionName string
}

func (r *MockActionWaitRunner) Context() runnercontext.Context {
	return r.context
}

func (r *MockActionWaitRunner) RunAction(ctx context.Context, actionName string) (runner.HookHandlerType, error) {
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

func NewPrepareHookCallbacks(kind hooks.Kind) *PrepareHookCallbacks {
	return &PrepareHookCallbacks{
		MockPrepareHook: &MockPrepareHook{nil, string(kind), nil},
	}
}

func NewRunActionRunnerFactory(runErr error) *MockRunnerFactory {
	return &MockRunnerFactory{
		MockNewActionRunner: &MockNewActionRunner{
			runner: &MockRunner{
				MockRunAction: &MockRunAction{err: runErr},
				context: &MockContext{
					actionData: &runnercontext.ActionData{Name: "some-action-name"},
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
					actionData: &runnercontext.ActionData{
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

var someActionId = "f47ac10b-58cc-4372-a567-0e02b2c3d479"
var randomActionId = "9f484882-2f18-4fd2-967d-db9663db7bea"
var overwriteState = operation.State{
	Kind:     operation.Continue,
	Step:     operation.Pending,
	Started:  true,
	CharmURL: "ch:quantal/wordpress-2",
	ActionId: &randomActionId,
	Hook:     &hook.Info{Kind: hooks.Install},
}
var someCommandArgs = operation.CommandArgs{
	Commands:        "do something",
	RelationId:      123,
	RemoteUnitName:  "foo/456",
	ForceRemoteUnit: true,
}
