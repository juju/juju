// Copyright 2016 Canonical Ltd.
// Copyright 2016 Cloudbase Solutions
// Licensed under the AGPLv3, see LICENCE file for details.

package retrystrategy_test

import (
	"context"
	"time"

	"github.com/juju/names/v6"
	"github.com/juju/tc"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/worker/v4"
	"github.com/juju/worker/v4/workertest"

	"github.com/juju/juju/core/watcher"
	loggertesting "github.com/juju/juju/internal/logger/testing"
	coretesting "github.com/juju/juju/internal/testing"
	"github.com/juju/juju/internal/worker/retrystrategy"
	"github.com/juju/juju/rpc/params"
)

type fixture struct {
	testing.Stub
}

func newFixture(c *tc.C, errs ...error) *fixture {
	fix := &fixture{}
	c.Assert(nil, jc.ErrorIsNil)
	fix.SetErrors(errs...)
	return fix
}

func (fix *fixture) Run(c *tc.C, test func(worker.Worker)) {
	stubRetryStrategy := params.RetryStrategy{
		ShouldRetry: true,
	}
	stubTag := stubTag{}
	stubFacade := newStubFacade(c, &fix.Stub, stubRetryStrategy, stubTag)
	stubConfig := retrystrategy.WorkerConfig{
		Facade:        stubFacade,
		AgentTag:      stubTag,
		RetryStrategy: stubRetryStrategy,
		Logger:        loggertesting.WrapCheckLog(c),
	}

	w, err := retrystrategy.NewRetryStrategyWorker(stubConfig)
	c.Assert(err, jc.ErrorIsNil)
	done := make(chan struct{})
	go func() {
		defer close(done)
		defer worker.Stop(w)
		test(w)
	}()

	select {
	case <-done:
	case <-time.After(coretesting.LongWait):
		c.Fatalf("test timed out")
	}
}

type stubFacade struct {
	c               *tc.C
	stub            *testing.Stub
	watcher         *stubWatcher
	count           int
	initialStrategy params.RetryStrategy
	stubTag         names.Tag
}

func newStubFacade(c *tc.C, stub *testing.Stub, initialStrategy params.RetryStrategy, stubTag names.Tag) *stubFacade {
	return &stubFacade{
		c:               c,
		stub:            stub,
		watcher:         newStubWatcher(),
		count:           0,
		initialStrategy: initialStrategy,
		stubTag:         stubTag,
	}
}

// WatchRetryStrategy is part of the retrystrategy Facade
func (f *stubFacade) WatchRetryStrategy(ctx context.Context, agentTag names.Tag) (watcher.NotifyWatcher, error) {
	f.c.Assert(agentTag, tc.Equals, f.stubTag)
	f.stub.AddCall("WatchRetryStrategy", agentTag)
	err := f.stub.NextErr()
	if err != nil {
		return nil, err
	}
	return f.watcher, nil
}

// RetryStrategy is part of the retrystrategy Facade
func (f *stubFacade) RetryStrategy(ctx context.Context, agentTag names.Tag) (params.RetryStrategy, error) {
	f.c.Assert(agentTag, tc.Equals, f.stubTag)
	f.stub.AddCall("RetryStrategy", agentTag)
	f.count = f.count + 1
	// Change the strategy after 2 handles
	if f.count == 2 {
		f.initialStrategy.ShouldRetry = !f.initialStrategy.ShouldRetry
	}
	return f.initialStrategy, f.stub.NextErr()
}

type stubWatcher struct {
	worker.Worker
	notifyChan <-chan struct{}
}

func newStubWatcher() *stubWatcher {
	changes := make(chan struct{}, 3)
	changes <- struct{}{}
	changes <- struct{}{}
	changes <- struct{}{}
	return &stubWatcher{
		Worker:     workertest.NewErrorWorker(nil),
		notifyChan: changes,
	}
}

// Changes is part of the watcher.NotifyWatcher interface
func (w *stubWatcher) Changes() watcher.NotifyChannel {
	return w.notifyChan
}

type stubTag struct {
	names.Tag
}
