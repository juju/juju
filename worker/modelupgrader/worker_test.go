// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package modelupgrader_test

import (
	"sync"

	"github.com/juju/errors"
	"github.com/juju/loggo"
	"github.com/juju/names/v4"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/worker/v2/workertest"
	gc "gopkg.in/check.v1"
	"gopkg.in/tomb.v2"

	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/core/status"
	"github.com/juju/juju/core/watcher"
	"github.com/juju/juju/environs"
	"github.com/juju/juju/environs/context"
	coretesting "github.com/juju/juju/testing"
	"github.com/juju/juju/worker/modelupgrader"
)

type WorkerSuite struct {
	testing.IsolationSuite
}

var _ = gc.Suite(&WorkerSuite{})

func (*WorkerSuite) TestNewWorkerValidatesConfig(c *gc.C) {
	_, err := modelupgrader.NewWorker(modelupgrader.Config{})
	c.Assert(err, gc.ErrorMatches, "nil Facade not valid")
}

func (*WorkerSuite) TestNewWorker(c *gc.C) {
	mockFacade := mockFacade{current: 123, target: 124}
	mockEnviron := mockEnviron{}
	mockGateUnlocker := mockGateUnlocker{}
	w, err := modelupgrader.NewWorker(modelupgrader.Config{
		Facade:        &mockFacade,
		Environ:       &mockEnviron,
		GateUnlocker:  &mockGateUnlocker,
		ControllerTag: coretesting.ControllerTag,
		ModelTag:      coretesting.ModelTag,
		CredentialAPI: &credentialAPIForTest{},
		Logger:        loggo.GetLogger("test"),
	})
	c.Assert(err, jc.ErrorIsNil)
	workertest.CheckKill(c, w)
	mockFacade.CheckCalls(c, []testing.StubCall{
		{"ModelTargetEnvironVersion", []interface{}{coretesting.ModelTag}},
		{"ModelEnvironVersion", []interface{}{coretesting.ModelTag}},
		{"SetModelStatus", []interface{}{coretesting.ModelTag, status.Busy, "upgrading environ from version 123 to 124", nilData}},
		{"SetModelStatus", []interface{}{coretesting.ModelTag, status.Available, "", nilData}},
	})
	mockEnviron.CheckCallNames(c, "UpgradeOperations")
	mockGateUnlocker.CheckCallNames(c, "Unlock")
}

func (*WorkerSuite) TestNewWorkerModelRemovedUninstalls(c *gc.C) {
	mockFacade := mockFacade{current: 123, target: 124}
	mockFacade.SetErrors(&params.Error{Code: params.CodeNotFound})
	mockEnviron := mockEnviron{}
	mockGateUnlocker := mockGateUnlocker{}
	w, err := modelupgrader.NewWorker(modelupgrader.Config{
		Facade:        &mockFacade,
		Environ:       &mockEnviron,
		GateUnlocker:  &mockGateUnlocker,
		ControllerTag: coretesting.ControllerTag,
		ModelTag:      coretesting.ModelTag,
		CredentialAPI: &credentialAPIForTest{},
		Logger:        loggo.GetLogger("test"),
	})
	c.Assert(errors.Cause(err), gc.ErrorMatches, modelupgrader.ErrModelRemoved.Error())
	workertest.CheckNilOrKill(c, w)
	mockFacade.CheckCalls(c, []testing.StubCall{
		{"ModelTargetEnvironVersion", []interface{}{coretesting.ModelTag}},
	})
	mockEnviron.CheckNoCalls(c)
	mockGateUnlocker.CheckNoCalls(c)
}

func (*WorkerSuite) TestNonUpgradeable(c *gc.C) {
	mockFacade := mockFacade{current: 123, target: 124}
	mockEnviron := struct{ environs.Environ }{} // not an Upgrader
	mockGateUnlocker := mockGateUnlocker{}
	w, err := modelupgrader.NewWorker(modelupgrader.Config{
		Facade:        &mockFacade,
		Environ:       &mockEnviron,
		GateUnlocker:  &mockGateUnlocker,
		ControllerTag: coretesting.ControllerTag,
		ModelTag:      coretesting.ModelTag,
		CredentialAPI: &credentialAPIForTest{},
		Logger:        loggo.GetLogger("test"),
	})
	c.Assert(err, jc.ErrorIsNil)
	workertest.CheckKill(c, w)
	mockFacade.CheckCalls(c, []testing.StubCall{
		{"ModelTargetEnvironVersion", []interface{}{coretesting.ModelTag}},
		{"ModelEnvironVersion", []interface{}{coretesting.ModelTag}},
		{"SetModelStatus", []interface{}{coretesting.ModelTag, status.Busy, "upgrading environ from version 123 to 124", nilData}},
		{"SetModelStatus", []interface{}{coretesting.ModelTag, status.Available, "", nilData}},
	})
	mockGateUnlocker.CheckCallNames(c, "Unlock")
}

func (*WorkerSuite) TestRunUpgradeOperations(c *gc.C) {
	var stepsStub testing.Stub
	mockFacade := mockFacade{current: 123, target: 125}
	mockEnviron := mockEnviron{
		ops: []environs.UpgradeOperation{{
			TargetVersion: 123,
			Steps: []environs.UpgradeStep{
				newStep(&stepsStub, "step122"),
			},
		}, {
			TargetVersion: 123,
			Steps: []environs.UpgradeStep{
				newStep(&stepsStub, "step123"),
			},
		}, {
			TargetVersion: 124,
			Steps: []environs.UpgradeStep{
				newStep(&stepsStub, "step124_0"),
				newStep(&stepsStub, "step124_1"),
			},
		}, {
			TargetVersion: 125,
			Steps: []environs.UpgradeStep{
				newStep(&stepsStub, "step125"),
			},
		}, {
			TargetVersion: 126,
			Steps: []environs.UpgradeStep{
				newStep(&stepsStub, "step126"),
			},
		}},
	}
	mockGateUnlocker := mockGateUnlocker{}
	w, err := modelupgrader.NewWorker(modelupgrader.Config{
		Facade:        &mockFacade,
		Environ:       &mockEnviron,
		GateUnlocker:  &mockGateUnlocker,
		ControllerTag: coretesting.ControllerTag,
		ModelTag:      coretesting.ModelTag,
		CredentialAPI: &credentialAPIForTest{},
		Logger:        loggo.GetLogger("test"),
	})
	c.Assert(err, jc.ErrorIsNil)
	workertest.CheckKill(c, w)
	mockFacade.CheckCalls(c, []testing.StubCall{
		{"ModelTargetEnvironVersion", []interface{}{coretesting.ModelTag}},
		{"ModelEnvironVersion", []interface{}{coretesting.ModelTag}},
		{"SetModelStatus", []interface{}{coretesting.ModelTag, status.Busy, "upgrading environ from version 123 to 125", nilData}},
		{"SetModelEnvironVersion", []interface{}{
			coretesting.ModelTag, 124,
		}},
		{"SetModelEnvironVersion", []interface{}{
			coretesting.ModelTag, 125,
		}},
		{"SetModelStatus", []interface{}{coretesting.ModelTag, status.Available, "", nilData}},
	})
	mockEnviron.CheckCalls(c, []testing.StubCall{
		{"UpgradeOperations", []interface{}{
			mockEnviron.callCtxUsed,
			environs.UpgradeOperationsParams{
				ControllerUUID: coretesting.ControllerTag.Id(),
			}},
		},
	})
	mockGateUnlocker.CheckCallNames(c, "Unlock")
	stepsStub.CheckCallNames(c, "step124_0", "step124_1", "step125")
}

func (*WorkerSuite) TestRunUpgradeOperationsStepError(c *gc.C) {
	var stepsStub testing.Stub
	stepsStub.SetErrors(errors.New("phooey"))
	mockFacade := mockFacade{current: 123, target: 124}
	mockEnviron := mockEnviron{
		ops: []environs.UpgradeOperation{{
			TargetVersion: 124,
			Steps: []environs.UpgradeStep{
				newStep(&stepsStub, "step124"),
			},
		}},
	}
	mockGateUnlocker := mockGateUnlocker{}
	w, err := modelupgrader.NewWorker(modelupgrader.Config{
		Facade:        &mockFacade,
		Environ:       &mockEnviron,
		GateUnlocker:  &mockGateUnlocker,
		ControllerTag: coretesting.ControllerTag,
		ModelTag:      coretesting.ModelTag,
		CredentialAPI: &credentialAPIForTest{},
		Logger:        loggo.GetLogger("test"),
	})
	c.Assert(err, jc.ErrorIsNil)

	err = workertest.CheckKilled(c, w)
	c.Assert(err, gc.ErrorMatches, "upgrading environ: phooey")

	mockFacade.CheckCalls(c, []testing.StubCall{
		{"ModelTargetEnvironVersion", []interface{}{coretesting.ModelTag}},
		{"ModelEnvironVersion", []interface{}{coretesting.ModelTag}},
		{"SetModelStatus", []interface{}{coretesting.ModelTag, status.Busy, "upgrading environ from version 123 to 124", nilData}},
		{"SetModelStatus", []interface{}{coretesting.ModelTag, status.Error, "failed to upgrade environ: phooey", nilData}},
	})
	mockGateUnlocker.CheckNoCalls(c)
}

func (*WorkerSuite) TestWaitForUpgrade(c *gc.C) {
	ch := make(chan struct{})
	mockFacade := mockFacade{
		current: 123,
		target:  125,
		watcher: newMockNotifyWatcher(ch),
	}
	mockGateUnlocker := mockGateUnlocker{}
	w, err := modelupgrader.NewWorker(modelupgrader.Config{
		Facade:        &mockFacade,
		Environ:       nil, // not responsible for running upgrades
		GateUnlocker:  &mockGateUnlocker,
		ControllerTag: coretesting.ControllerTag,
		ModelTag:      coretesting.ModelTag,
		CredentialAPI: &credentialAPIForTest{},
		Logger:        loggo.GetLogger("test"),
	})
	c.Assert(err, jc.ErrorIsNil)

	// Send the initial change event on the watcher, and
	// wait for the worker to call "ModelEnvironVersion".
	ch <- struct{}{}
	for a := coretesting.LongAttempt.Start(); a.Next(); {
		if len(mockFacade.Calls()) < 3 && a.HasNext() {
			continue
		}
		mockFacade.CheckCalls(c, []testing.StubCall{
			{"ModelTargetEnvironVersion", []interface{}{coretesting.ModelTag}},
			{"WatchModelEnvironVersion", []interface{}{coretesting.ModelTag}},
			{"ModelEnvironVersion", []interface{}{coretesting.ModelTag}},
		})
		mockGateUnlocker.CheckNoCalls(c)
		break
	}

	// Set the current version >= target. In practice we should
	// only ever see that the current version <= target, as all
	// controller agents should be running the same version at
	// this point. We require that the environ version be strictly
	// increasing, so we can be defensive.
	mockFacade.setCurrent(126)
	ch <- struct{}{}

	workertest.CheckKill(c, w)
	mockFacade.CheckCalls(c, []testing.StubCall{
		{"ModelTargetEnvironVersion", []interface{}{coretesting.ModelTag}},
		{"WatchModelEnvironVersion", []interface{}{coretesting.ModelTag}},
		{"ModelEnvironVersion", []interface{}{coretesting.ModelTag}},
		{"ModelEnvironVersion", []interface{}{coretesting.ModelTag}},
	})
	mockGateUnlocker.CheckCallNames(c, "Unlock")
}

func (*WorkerSuite) TestModelNotFoundWhenRunning(c *gc.C) {
	ch := make(chan struct{})
	mockFacade := mockFacade{
		current: 123,
		target:  125,
		watcher: newMockNotifyWatcher(ch),
	}
	w, err := modelupgrader.NewWorker(modelupgrader.Config{
		Facade:        &mockFacade,
		Environ:       nil, // not responsible for running upgrades
		GateUnlocker:  &mockGateUnlocker{},
		ControllerTag: coretesting.ControllerTag,
		ModelTag:      coretesting.ModelTag,
		CredentialAPI: &credentialAPIForTest{},
		Logger:        loggo.GetLogger("test"),
	})
	c.Assert(err, jc.ErrorIsNil)

	mockFacade.SetErrors(&params.Error{Code: params.CodeNotFound})
	ch <- struct{}{}

	err = workertest.CheckKill(c, w)
	// We expect NotFound to be changed to modelupgrader.ErrModelRemoved.
	c.Check(err, gc.ErrorMatches, "model has been removed")
}

func newStep(stub *testing.Stub, name string) environs.UpgradeStep {
	run := func() error {
		stub.AddCall(name)
		return stub.NextErr()
	}
	return mockUpgradeStep{name, run}
}

type mockUpgradeStep struct {
	description string
	run         func() error
}

func (s mockUpgradeStep) Description() string {
	return s.description
}

func (s mockUpgradeStep) Run(ctx context.ProviderCallContext) error {
	return s.run()
}

type mockFacade struct {
	testing.Stub
	target  int
	watcher *mockNotifyWatcher

	mu      sync.Mutex
	current int
}

func (f *mockFacade) setCurrent(v int) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.current = v
}

func (f *mockFacade) ModelEnvironVersion(tag names.ModelTag) (int, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.MethodCall(f, "ModelEnvironVersion", tag)
	return f.current, f.NextErr()
}

func (f *mockFacade) ModelTargetEnvironVersion(tag names.ModelTag) (int, error) {
	f.MethodCall(f, "ModelTargetEnvironVersion", tag)
	return f.target, f.NextErr()
}

func (f *mockFacade) SetModelEnvironVersion(tag names.ModelTag, v int) error {
	f.MethodCall(f, "SetModelEnvironVersion", tag, v)
	return f.NextErr()
}

func (f *mockFacade) WatchModelEnvironVersion(tag names.ModelTag) (watcher.NotifyWatcher, error) {
	f.MethodCall(f, "WatchModelEnvironVersion", tag)
	if err := f.NextErr(); err != nil {
		return nil, err
	}
	if f.watcher != nil {
		return f.watcher, nil
	}
	return nil, errors.New("unexpected call to WatchModelEnvironVersion")
}

var nilData map[string]interface{}

func (f *mockFacade) SetModelStatus(tag names.ModelTag, status status.Status, info string, data map[string]interface{}) error {
	f.MethodCall(f, "SetModelStatus", tag, status, info, data)
	return f.NextErr()
}

type mockEnviron struct {
	environs.Environ
	testing.Stub
	ops []environs.UpgradeOperation

	callCtxUsed context.ProviderCallContext
}

func (e *mockEnviron) UpgradeOperations(ctx context.ProviderCallContext, args environs.UpgradeOperationsParams) []environs.UpgradeOperation {
	e.MethodCall(e, "UpgradeOperations", ctx, args)
	e.callCtxUsed = ctx
	e.PopNoErr()
	return e.ops
}

type mockGateUnlocker struct {
	testing.Stub
}

func (g *mockGateUnlocker) Unlock() {
	g.MethodCall(g, "Unlock")
	g.PopNoErr()
}

type mockNotifyWatcher struct {
	tomb tomb.Tomb
	ch   chan struct{}
}

func newMockNotifyWatcher(ch chan struct{}) *mockNotifyWatcher {
	w := &mockNotifyWatcher{ch: ch}
	w.tomb.Go(func() error {
		defer close(ch)
		<-w.tomb.Dying()
		return tomb.ErrDying
	})
	return w
}

func (w *mockNotifyWatcher) Changes() watcher.NotifyChannel {
	return w.ch
}

func (w *mockNotifyWatcher) Kill() {
	w.tomb.Kill(nil)
}

func (w *mockNotifyWatcher) Wait() error {
	return w.tomb.Wait()
}

type credentialAPIForTest struct{}

func (*credentialAPIForTest) InvalidateModelCredential(reason string) error {
	return nil
}
