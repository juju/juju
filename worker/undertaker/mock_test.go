// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package undertaker_test

import (
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/environs"
	"github.com/juju/juju/status"
	"github.com/juju/juju/watcher"
	"github.com/juju/juju/worker"
	"github.com/juju/juju/worker/undertaker"
	"github.com/juju/juju/worker/workertest"
)

type mockFacade struct {
	stub *testing.Stub
	info params.UndertakerModelInfoResult
}

func (mock *mockFacade) ModelInfo() (params.UndertakerModelInfoResult, error) {
	mock.stub.AddCall("ModelInfo")
	if err := mock.stub.NextErr(); err != nil {
		return params.UndertakerModelInfoResult{}, err
	}
	return mock.info, nil
}

func (mock *mockFacade) WatchModelResources() (watcher.NotifyWatcher, error) {
	mock.stub.AddCall("WatchModelResources")
	if err := mock.stub.NextErr(); err != nil {
		return nil, err
	}
	const count = 5
	changes := make(chan struct{}, count)
	for i := 0; i < count; i++ {
		changes <- struct{}{}
	}
	return &mockWatcher{
		Worker:  workertest.NewErrorWorker(nil),
		changes: changes,
	}, nil
}

func (mock *mockFacade) ProcessDyingModel() error {
	mock.stub.AddCall("ProcessDyingModel")
	return mock.stub.NextErr()
}

func (mock *mockFacade) SetStatus(status status.Status, info string, data map[string]interface{}) error {
	mock.stub.MethodCall(mock, "SetStatus", status, info, data)
	return mock.stub.NextErr()
}

func (mock *mockFacade) RemoveModel() error {
	mock.stub.AddCall("RemoveModel")
	return mock.stub.NextErr()
}

type mockEnviron struct {
	environs.Environ
	stub *testing.Stub
}

func (mock *mockEnviron) Destroy() error {
	mock.stub.AddCall("Destroy")
	return mock.stub.NextErr()
}

type mockWatcher struct {
	worker.Worker
	changes chan struct{}
}

func (mock *mockWatcher) Changes() watcher.NotifyChannel {
	return mock.changes
}

type fixture struct {
	info   params.UndertakerModelInfoResult
	errors []error
	dirty  bool
}

func (fix fixture) cleanup(c *gc.C, w worker.Worker) {
	if fix.dirty {
		workertest.DirtyKill(c, w)
	} else {
		workertest.CleanKill(c, w)
	}
}

func (fix fixture) run(c *gc.C, test func(worker.Worker)) *testing.Stub {
	stub := &testing.Stub{}
	environ := &mockEnviron{
		stub: stub,
	}
	facade := &mockFacade{
		stub: stub,
		info: fix.info,
	}
	stub.SetErrors(fix.errors...)
	w, err := undertaker.NewUndertaker(undertaker.Config{
		Facade:  facade,
		Environ: environ,
	})
	c.Assert(err, jc.ErrorIsNil)
	defer fix.cleanup(c, w)
	test(w)
	return stub
}
