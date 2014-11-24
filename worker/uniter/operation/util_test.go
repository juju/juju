// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package operation_test

import (
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

func NewPrepareDeploySuccessCallbacks() *DeployCallbacks {
	return &DeployCallbacks{
		MockGetArchiveInfo:  &MockGetArchiveInfo{info: &MockBundleInfo{}},
		MockSetCurrentCharm: &MockSetCurrentCharm{},
	}
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

type MockGetRunner struct {
	gotContext *context.Context
	runner     *MockRunner
}

func (mock *MockGetRunner) Call(ctx context.Context) context.Runner {
	mock.gotContext = &ctx
	return mock.runner
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
	*MockGetRunner
	MockNotifyHookCompleted *MockNotify
	MockNotifyHookFailed    *MockNotify
}

func (cb *ExecuteHookCallbacks) AcquireExecutionLock(message string) (func(), error) {
	return cb.MockAcquireExecutionLock.Call(message)
}

func (cb *ExecuteHookCallbacks) GetRunner(ctx context.Context) context.Runner {
	return cb.MockGetRunner.Call(ctx)
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

type MockNewHookContext struct {
	gotHook *hook.Info
	context context.Context
	err     error
}

func (mock *MockNewHookContext) Call(hookInfo hook.Info) (context.Context, error) {
	mock.gotHook = &hookInfo
	return mock.context, mock.err
}

type MockHookContextFactory struct {
	context.Factory
	*MockNewHookContext
}

func (f *MockHookContextFactory) NewHookContext(hookInfo hook.Info) (context.Context, error) {
	return f.MockNewHookContext.Call(hookInfo)
}

type MockContext struct {
	context.Context
}

type MockRunHook struct {
	context.Runner
	gotName *string
	err     error
}

func (mock *MockRunHook) Call(hookName string) error {
	mock.gotName = &hookName
	return mock.err
}

type MockRunner struct {
	context.Runner
	*MockRunHook
}

func (r *MockRunner) RunHook(hookName string) error {
	return r.MockRunHook.Call(hookName)
}

func NewPrepareHookSuccessFixture() (*PrepareHookCallbacks, *MockHookContextFactory) {
	return &PrepareHookCallbacks{
			MockPrepareHook: &MockPrepareHook{nil, "some-hook-name", nil},
		}, &MockHookContextFactory{
			MockNewHookContext: &MockNewHookContext{nil, &MockContext{}, nil},
		}
}

var curl = corecharm.MustParseURL
var dumbActionId = "foo_a_1"
var overwriteState = operation.State{
	Kind:               operation.Continue,
	Step:               operation.Pending,
	Started:            true,
	CollectMetricsTime: 1234567,
	CharmURL:           curl("cs:quantal/wordpress-2"),
	ActionId:           &dumbActionId,
	Hook:               &hook.Info{Kind: hooks.Install},
}
