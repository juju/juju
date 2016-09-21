// Copyright 2012-2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package environ_test

import (
	"sync"

	"github.com/juju/testing"
	gc "gopkg.in/check.v1"
	names "gopkg.in/juju/names.v2"

	"github.com/juju/juju/environs"
	"github.com/juju/juju/environs/config"
	coretesting "github.com/juju/juju/testing"
	"github.com/juju/juju/watcher"
	"github.com/juju/juju/worker"
	"github.com/juju/juju/worker/workertest"
)

type fixture struct {
	watcherErr    error
	observerErrs  []error
	cloud         environs.CloudSpec
	initialConfig map[string]interface{}
}

func (fix *fixture) Run(c *gc.C, test func(*runContext)) {
	watcher := newNotifyWatcher(fix.watcherErr)
	defer workertest.DirtyKill(c, watcher)
	context := &runContext{
		cloud:   fix.cloud,
		config:  newModelConfig(c, fix.initialConfig),
		watcher: watcher,
	}
	context.stub.SetErrors(fix.observerErrs...)
	test(context)
}

type runContext struct {
	mu          sync.Mutex
	stub        testing.Stub
	cloud       environs.CloudSpec
	config      map[string]interface{}
	watcher     *notifyWatcher
	credWatcher *notifyWatcher
}

// SetConfig updates the configuration returned by ModelConfig.
func (context *runContext) SetConfig(c *gc.C, extraAttrs coretesting.Attrs) {
	context.mu.Lock()
	defer context.mu.Unlock()
	context.config = newModelConfig(c, extraAttrs)
}

// CloudSpec is part of the environ.ConfigObserver interface.
func (context *runContext) CloudSpec(tag names.ModelTag) (environs.CloudSpec, error) {
	context.mu.Lock()
	defer context.mu.Unlock()
	context.stub.AddCall("CloudSpec", tag)
	if err := context.stub.NextErr(); err != nil {
		return environs.CloudSpec{}, err
	}
	return context.cloud, nil
}

// ModelConfig is part of the environ.ConfigObserver interface.
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

// WatchForModelConfigChanges is part of the environ.ConfigObserver interface.
func (context *runContext) WatchForModelConfigChanges() (watcher.NotifyWatcher, error) {
	context.mu.Lock()
	defer context.mu.Unlock()
	context.stub.AddCall("WatchForModelConfigChanges")
	if err := context.stub.NextErr(); err != nil {
		return nil, err
	}
	return context.watcher, nil
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

// WatchCredential is part of the environ.ConfigObserver interface.
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
	cfg *config.Config
	mu  sync.Mutex
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

func newMockEnviron(args environs.OpenParams) (environs.Environ, error) {
	return &mockEnviron{cfg: args.Config}, nil
}
