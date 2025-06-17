// Copyright 2012-2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package environ_test

import (
	"context"
	"sync"

	"github.com/juju/names/v5"
	"github.com/juju/testing"
	"github.com/juju/worker/v3"
	"github.com/juju/worker/v3/workertest"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/controller"
	"github.com/juju/juju/core/watcher"
	"github.com/juju/juju/environs"
	environscloudspec "github.com/juju/juju/environs/cloudspec"
	"github.com/juju/juju/environs/config"
	coretesting "github.com/juju/juju/testing"
)

type fixture struct {
	watcherErr    error
	observerErrs  []error
	initialConfig map[string]interface{}
	initialSpec   environscloudspec.CloudSpec
}

func (fix *fixture) Run(c *gc.C, test func(*runContext)) {
	watcher := newNotifyWatcher(fix.watcherErr)
	defer workertest.DirtyKill(c, watcher)
	cloudWatcher := newNotifyWatcher(fix.watcherErr)
	defer workertest.DirtyKill(c, cloudWatcher)
	context := &runContext{
		cloud:            fix.initialSpec,
		config:           newModelConfig(c, fix.initialConfig),
		watcher:          watcher,
		cloudWatcher:     cloudWatcher,
		controllerConfig: coretesting.FakeControllerConfig(),
	}
	context.stub.SetErrors(fix.observerErrs...)
	test(context)
}

type runContext struct {
	mu               sync.Mutex
	stub             testing.Stub
	cloud            environscloudspec.CloudSpec
	config           map[string]interface{}
	watcher          *notifyWatcher
	cloudWatcher     *notifyWatcher
	credWatcher      *notifyWatcher
	controllerConfig controller.Config
}

func (context *runContext) ControllerConfig() (controller.Config, error) {
	context.mu.Lock()
	defer context.mu.Unlock()
	context.stub.AddCall("ControllerConfig")
	if err := context.stub.NextErr(); err != nil {
		return controller.Config{}, err
	}
	return context.controllerConfig, nil
}

// SetConfig updates the configuration returned by ModelConfig.
func (context *runContext) SetConfig(c *gc.C, extraAttrs coretesting.Attrs) {
	context.mu.Lock()
	defer context.mu.Unlock()
	context.config = newModelConfig(c, extraAttrs)
}

// SetCloudSpec updates the spec returned by CloudSpec.
func (context *runContext) SetCloudSpec(c *gc.C, spec environscloudspec.CloudSpec) {
	context.mu.Lock()
	defer context.mu.Unlock()
	context.cloud = spec
}

// CloudSpec is part of the environ.ConfigAPI interface.
func (context *runContext) CloudSpec() (environscloudspec.CloudSpec, error) {
	context.mu.Lock()
	defer context.mu.Unlock()
	context.stub.AddCall("CloudSpec")
	if err := context.stub.NextErr(); err != nil {
		return environscloudspec.CloudSpec{}, err
	}
	return context.cloud, nil
}

// ModelConfig is part of the environ.ConfigAPI interface.
func (context *runContext) ModelConfig() (*config.Config, error) {
	context.mu.Lock()
	defer context.mu.Unlock()
	context.stub.AddCall("ModelConfig")
	if err := context.stub.NextErr(); err != nil {
		return nil, err
	}
	return config.New(config.NoDefaults, context.config)
}

// KillModelConfigNotify kills the watcher returned from WatchForModelConfigChanges with
// the error configured in the enclosing fixture.
func (context *runContext) KillModelConfigNotify() {
	context.watcher.Kill()
}

// SendModelConfigNotify sends a value on the channel used by WatchForModelConfigChanges
// results.
func (context *runContext) SendModelConfigNotify() {
	context.watcher.changes <- struct{}{}
}

// CloseModelConfigNotify closes the channel used by WatchForModelConfigChanges results.
func (context *runContext) CloseModelConfigNotify() {
	close(context.watcher.changes)
}

// KillCloudSpecNotify kills the watcher returned from WatchCloudSpecChanges with
// the error configured in the enclosing fixture.
func (context *runContext) KillCloudSpecNotify() {
	context.cloudWatcher.Kill()
}

// SendCloudSpecNotify sends a value on the channel used by WatchCloudSpecChanges
// results.
func (context *runContext) SendCloudSpecNotify() {
	context.cloudWatcher.changes <- struct{}{}
}

// CloseCloudSpecNotify closes the channel used by WatchCloudSpecChanges results.
func (context *runContext) CloseCloudSpecNotify() {
	close(context.cloudWatcher.changes)
}

// WatchForModelConfigChanges is part of the environ.ConfigAPI interface.
func (context *runContext) WatchForModelConfigChanges() (watcher.NotifyWatcher, error) {
	context.mu.Lock()
	defer context.mu.Unlock()
	context.stub.AddCall("WatchForModelConfigChanges")
	if err := context.stub.NextErr(); err != nil {
		return nil, err
	}
	return context.watcher, nil
}

func (context *runContext) WatchCloudSpecChanges() (watcher.NotifyWatcher, error) {
	context.mu.Lock()
	defer context.mu.Unlock()
	context.stub.AddCall("WatchCloudSpecChanges")
	if err := context.stub.NextErr(); err != nil {
		return nil, err
	}
	return context.cloudWatcher, nil
}

// KillCredentialNotify kills the watcher returned from WatchCredentialChanges with
// the error configured in the enclosing fixture.
func (context *runContext) KillCredentialNotify() {
	context.credWatcher.Kill()
}

// SendCredentialNotify sends a value on the channel used by WatchCredentialChanges
// results.
func (context *runContext) SendCredentialNotify() {
	context.credWatcher.changes <- struct{}{}
}

// CloseCredentialNotify closes the channel used by WatchCredentialChanges results.
func (context *runContext) CloseCredentialNotify() {
	close(context.credWatcher.changes)
}

// WatchCredential is part of the environ.ConfigAPI interface.
func (context *runContext) WatchCredential(cred names.CloudCredentialTag) (watcher.NotifyWatcher, error) {
	context.mu.Lock()
	defer context.mu.Unlock()
	context.stub.AddCall("WatchCredential")
	if err := context.stub.NextErr(); err != nil {
		return nil, err
	}
	return context.watcher, nil
}

func (context *runContext) CheckCallNames(c *gc.C, names ...string) {
	context.mu.Lock()
	defer context.mu.Unlock()
	context.stub.CheckCallNames(c, names...)
}

// newNotifyWatcher returns a watcher.NotifyWatcher that will fail with the
// supplied error when Kill()ed.
func newNotifyWatcher(err error) *notifyWatcher {
	return &notifyWatcher{
		Worker:  workertest.NewErrorWorker(err),
		changes: make(chan struct{}, 1000),
	}
}

type notifyWatcher struct {
	worker.Worker
	changes chan struct{}
}

// Changes is part of the watcher.NotifyWatcher interface.
func (w *notifyWatcher) Changes() watcher.NotifyChannel {
	return w.changes
}

// newModelConfig returns an environment config map with the supplied attrs
// (on top of some default set), or fails the test.
func newModelConfig(c *gc.C, extraAttrs coretesting.Attrs) map[string]interface{} {
	return coretesting.CustomModelConfig(c, extraAttrs).AllAttrs()
}

type mockEnviron struct {
	environs.Environ
	testing.Stub
	cfg  *config.Config
	spec environscloudspec.CloudSpec
	mu   sync.Mutex
}

func (e *mockEnviron) Config() *config.Config {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.MethodCall(e, "Config")
	e.PopNoErr()
	return e.cfg
}

func (e *mockEnviron) SetConfig(cfg *config.Config) error {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.MethodCall(e, "SetConfig", cfg)
	if err := e.NextErr(); err != nil {
		return err
	}
	e.cfg = cfg
	return nil
}

func (e *mockEnviron) CloudSpec() environscloudspec.CloudSpec {
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.spec
}

func (e *mockEnviron) SetCloudSpec(_ context.Context, spec environscloudspec.CloudSpec) error {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.MethodCall(e, "SetCloudSpec", spec)
	if err := e.NextErr(); err != nil {
		return err
	}
	e.spec = spec
	return nil
}

func newMockEnviron(_ context.Context, args environs.OpenParams) (environs.Environ, error) {
	return &mockEnviron{cfg: args.Config, spec: args.Cloud}, nil
}
