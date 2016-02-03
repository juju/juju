// Copyright 2012-2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package environ_test

import (
	"sync"

	"github.com/juju/testing"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/provider/dummy"
	coretesting "github.com/juju/juju/testing"
	"github.com/juju/juju/watcher"
	"github.com/juju/juju/worker"
	"github.com/juju/juju/worker/workertest"
)

type fixture struct {
	watcherErr    error
	observerErrs  []error
	initialConfig map[string]interface{}
}

func (fix *fixture) Run(c *gc.C, test func(*runContext)) {
	watcher := newNotifyWatcher(fix.watcherErr)
	defer workertest.DirtyKill(c, watcher)
	context := &runContext{
		config:  newEnvironConfig(c, fix.initialConfig),
		watcher: watcher,
	}
	context.stub.SetErrors(fix.observerErrs...)
	test(context)
}

type runContext struct {
	mu      sync.Mutex
	stub    testing.Stub
	config  map[string]interface{}
	watcher *notifyWatcher
}

// SetConfig updates the configuration returned by EnvironConfig.
func (context *runContext) SetConfig(c *gc.C, extraAttrs coretesting.Attrs) {
	context.mu.Lock()
	defer context.mu.Unlock()
	context.config = newEnvironConfig(c, extraAttrs)
}

// EnvironConfig is part of the environ.ConfigObserver interface.
func (context *runContext) EnvironConfig() (*config.Config, error) {
	context.mu.Lock()
	defer context.mu.Unlock()
	context.stub.AddCall("EnvironConfig")
	if err := context.stub.NextErr(); err != nil {
		return nil, err
	}
	return config.New(config.NoDefaults, context.config)
}

// KillNotify kills the watcher returned from WatchForEnvironConfigChanges with
// the error configured in the enclosing fixture.
func (context *runContext) KillNotify() {
	context.watcher.Kill()
}

// SendNotify sends a value on the channel used by WatchForEnvironConfigChanges
// results.
func (context *runContext) SendNotify() {
	context.watcher.changes <- struct{}{}
}

// CloseNotify closes the channel used by WatchForEnvironConfigChanges results.
func (context *runContext) CloseNotify() {
	close(context.watcher.changes)
}

// WatchForEnvironConfigChanges is part of the environ.ConfigObserver interface.
func (context *runContext) WatchForEnvironConfigChanges() (watcher.NotifyWatcher, error) {
	context.mu.Lock()
	defer context.mu.Unlock()
	context.stub.AddCall("WatchForEnvironConfigChanges")
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

// newEnvironConfig returns an environment config map with the supplied attrs
// (on top of some default set), or fails the test.
func newEnvironConfig(c *gc.C, extraAttrs coretesting.Attrs) map[string]interface{} {
	attrs := dummy.SampleConfig()
	attrs["broken"] = ""
	attrs["state-id"] = "42"
	for k, v := range extraAttrs {
		attrs[k] = v
	}
	return coretesting.CustomEnvironConfig(c, attrs).AllAttrs()
}
