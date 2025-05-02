// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package undertaker_test

import (
	"context"
	"time"

	"github.com/juju/clock/testclock"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/worker/v4"
	"github.com/juju/worker/v4/workertest"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/core/status"
	"github.com/juju/juju/core/watcher"
	"github.com/juju/juju/environs"
	"github.com/juju/juju/environs/cloudspec"
	"github.com/juju/juju/environs/config"
	loggertesting "github.com/juju/juju/internal/logger/testing"
	"github.com/juju/juju/internal/worker/undertaker"
	"github.com/juju/juju/rpc/params"
)

type mockFacade struct {
	stub         *testing.Stub
	info         params.UndertakerModelInfoResult
	clock        testclock.AdvanceableClock
	advance      time.Duration
	modelChanges chan struct{}
}

func (mock *mockFacade) ModelInfo(context.Context) (params.UndertakerModelInfoResult, error) {
	mock.stub.AddCall("ModelInfo")
	if err := mock.stub.NextErr(); err != nil {
		return params.UndertakerModelInfoResult{}, err
	}
	return mock.info, nil
}

func (mock *mockFacade) WatchModelResources(context.Context) (watcher.NotifyWatcher, error) {
	mock.stub.AddCall("WatchModelResources")
	if mock.advance > 0 {
		mock.clock.Advance(mock.advance)
	}
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

func (mock *mockFacade) ProcessDyingModel(context.Context) error {
	mock.stub.AddCall("ProcessDyingModel")
	time.Sleep(100 * time.Millisecond)
	return mock.stub.NextErr()
}

func (mock *mockFacade) SetStatus(ctx context.Context, status status.Status, info string, data map[string]interface{}) error {
	mock.stub.MethodCall(mock, "SetStatus", status, info, data)
	return mock.stub.NextErr()
}

func (mock *mockFacade) RemoveModel(context.Context) error {
	mock.stub.AddCall("RemoveModel")
	time.Sleep(100 * time.Millisecond)
	return mock.stub.NextErr()
}

func (mock *mockFacade) ModelConfig(_ context.Context) (*config.Config, error) {
	mock.stub.AddCall("ModelConfig")
	cfg, _ := config.New(config.NoDefaults, map[string]interface{}{
		"uuid": "00000000-0000-0000-0000-000000000000",
		"name": "name",
	})
	return cfg, mock.stub.NextErr()
}

func (mock *mockFacade) CloudSpec(_ context.Context) (cloudspec.CloudSpec, error) {
	mock.stub.AddCall("CloudSpec")
	return cloudspec.CloudSpec{}, mock.stub.NextErr()
}

func (mock *mockFacade) WatchModel(context.Context) (watcher.NotifyWatcher, error) {
	mock.stub.AddCall("WatchModel")
	if err := mock.stub.NextErr(); err != nil {
		return nil, err
	}
	return &mockWatcher{
		Worker:  workertest.NewErrorWorker(nil),
		changes: mock.modelChanges,
	}, nil
}

type mockDestroyer struct {
	environs.Environ
	stub *testing.Stub
}

func (mock *mockDestroyer) Destroy(ctx context.Context) error {
	mock.stub.AddCall("Destroy", ctx)
	// A small delay to allow any timeout to expire.
	time.Sleep(100 * time.Millisecond)
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
	info    params.UndertakerModelInfoResult
	errors  []error
	dirty   bool
	clock   testclock.AdvanceableClock
	advance time.Duration
}

func (fix *fixture) cleanup(c *gc.C, w worker.Worker) {
	if fix.dirty {
		workertest.DirtyKill(c, w)
	} else {
		workertest.CleanKill(c, w)
	}
}

func (fix *fixture) run(c *gc.C, test func(worker.Worker)) *testing.Stub {
	stub := &testing.Stub{}
	facade := &mockFacade{
		stub:         stub,
		info:         fix.info,
		clock:        fix.clock,
		advance:      fix.advance,
		modelChanges: make(chan struct{}, 1),
	}
	facade.modelChanges <- struct{}{}
	stub.SetErrors(fix.errors...)
	w, err := undertaker.NewUndertaker(undertaker.Config{
		Facade: facade,
		Logger: loggertesting.WrapCheckLog(c),
		Clock:  fix.clock,
		NewCloudDestroyerFunc: func(ctx context.Context, op environs.OpenParams, _ environs.CredentialInvalidator) (environs.CloudDestroyer, error) {
			return &mockDestroyer{stub: stub}, nil
		},
	})
	c.Assert(err, jc.ErrorIsNil)
	defer fix.cleanup(c, w)
	test(w)
	return stub
}
