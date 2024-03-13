// Copyright 2012-2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package providertracker_test

import (
	"context"
	"sync"

	"github.com/juju/names/v5"
	"github.com/juju/testing"
	"github.com/juju/worker/v4"
	"github.com/juju/worker/v4/workertest"
	gc "gopkg.in/check.v1"

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

func (fix *fixture) Run(c *gc.C, test func(*testObserver)) {
	watcher := newNotifyWatcher(fix.watcherErr)
	defer workertest.CleanKill(c, watcher)
	cloudWatcher := newNotifyWatcher(fix.watcherErr)
	defer workertest.CleanKill(c, cloudWatcher)
	env := &testObserver{
		cloud:        fix.initialSpec,
		config:       newModelConfig(c, fix.initialConfig),
		watcher:      watcher,
		cloudWatcher: cloudWatcher,
	}
	env.stub.SetErrors(fix.observerErrs...)
	test(env)
}

type testObserver struct {
	mu           sync.Mutex
	stub         testing.Stub
	cloud        environscloudspec.CloudSpec
	config       map[string]interface{}
	watcher      *notifyWatcher
	cloudWatcher *notifyWatcher
	credWatcher  *notifyWatcher
}

// SetConfig updates the configuration returned by ModelConfig.
func (o *testObserver) SetConfig(c *gc.C, extraAttrs coretesting.Attrs) {
	o.mu.Lock()
	defer o.mu.Unlock()
	o.config = newModelConfig(c, extraAttrs)
}

// SetCloudSpec updates the spec returned by CloudSpec.
func (o *testObserver) SetCloudSpec(c *gc.C, spec environscloudspec.CloudSpec) {
	o.mu.Lock()
	defer o.mu.Unlock()
	o.cloud = spec
}

// CloudSpec is part of the environ.ConfigObserver interface.
func (o *testObserver) CloudSpec(context.Context) (environscloudspec.CloudSpec, error) {
	o.mu.Lock()
	defer o.mu.Unlock()
	o.stub.AddCall("CloudSpec")
	if err := o.stub.NextErr(); err != nil {
		return environscloudspec.CloudSpec{}, err
	}
	return o.cloud, nil
}

// ModelConfig is part of the environ.ConfigObserver interface.
func (o *testObserver) ModelConfig(context.Context) (*config.Config, error) {
	o.mu.Lock()
	defer o.mu.Unlock()
	o.stub.AddCall("ModelConfig")
	if err := o.stub.NextErr(); err != nil {
		return nil, err
	}
	return config.New(config.NoDefaults, o.config)
}

// KillModelConfigNotify kills the watcher returned from WatchForModelConfigChanges with
// the error configured in the enclosing fixture.
func (o *testObserver) KillModelConfigNotify() {
	o.watcher.Kill()
}

// SendModelConfigNotify sends a value on the channel used by WatchForModelConfigChanges
// results.
func (o *testObserver) SendModelConfigNotify() {
	o.watcher.changes <- struct{}{}
}

// CloseModelConfigNotify closes the channel used by WatchForModelConfigChanges results.
func (o *testObserver) CloseModelConfigNotify() {
	close(o.watcher.changes)
}

// KillCloudSpecNotify kills the watcher returned from WatchCloudSpecChanges with
// the error configured in the enclosing fixture.
func (o *testObserver) KillCloudSpecNotify() {
	o.cloudWatcher.Kill()
}

// SendCloudSpecNotify sends a value on the channel used by WatchCloudSpecChanges
// results.
func (o *testObserver) SendCloudSpecNotify() {
	o.cloudWatcher.changes <- struct{}{}
}

// CloseCloudSpecNotify closes the channel used by WatchCloudSpecChanges results.
func (o *testObserver) CloseCloudSpecNotify() {
	close(o.cloudWatcher.changes)
}

// WatchForModelConfigChanges is part of the environ.ConfigObserver interface.
func (o *testObserver) WatchForModelConfigChanges() (watcher.NotifyWatcher, error) {
	o.mu.Lock()
	defer o.mu.Unlock()
	o.stub.AddCall("WatchForModelConfigChanges")
	if err := o.stub.NextErr(); err != nil {
		return nil, err
	}
	return o.watcher, nil
}

func (o *testObserver) WatchCloudSpecChanges() (watcher.NotifyWatcher, error) {
	o.mu.Lock()
	defer o.mu.Unlock()
	o.stub.AddCall("WatchCloudSpecChanges")
	if err := o.stub.NextErr(); err != nil {
		return nil, err
	}
	return o.cloudWatcher, nil
}

// KillCredentialNotify kills the watcher returned from WatchCredentialChanges with
// the error configured in the enclosing fixture.
func (o *testObserver) KillCredentialNotify() {
	o.credWatcher.Kill()
}

// SendCredentialNotify sends a value on the channel used by WatchCredentialChanges
// results.
func (o *testObserver) SendCredentialNotify() {
	o.credWatcher.changes <- struct{}{}
}

// CloseCredentialNotify closes the channel used by WatchCredentialChanges results.
func (o *testObserver) CloseCredentialNotify() {
	close(o.credWatcher.changes)
}

// WatchCredential is part of the environ.ConfigObserver interface.
func (o *testObserver) WatchCredential(cred names.CloudCredentialTag) (watcher.NotifyWatcher, error) {
	o.mu.Lock()
	defer o.mu.Unlock()
	o.stub.AddCall("WatchCredential")
	if err := o.stub.NextErr(); err != nil {
		return nil, err
	}
	return o.watcher, nil
}

func (o *testObserver) CheckCallNames(c *gc.C, names ...string) {
	o.mu.Lock()
	defer o.mu.Unlock()
	o.stub.CheckCallNames(c, names...)
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
