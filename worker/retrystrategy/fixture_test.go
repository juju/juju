// Copyright 2016 Canonical Ltd.
// Copyright 2016 Cloudbase Solutions
// Licensed under the AGPLv3, see LICENCE file for details.

package retrystrategy_test

import (
	"time"

	"github.com/juju/loggo"
	"github.com/juju/names/v4"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/worker/v2"
	"github.com/juju/worker/v2/workertest"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/core/watcher"
	coretesting "github.com/juju/juju/testing"
	"github.com/juju/juju/worker/retrystrategy"
)

type fixture struct {
	testing.Stub
}

func newFixture(c *gc.C, errs ...error) *fixture {
	fix := &fixture{}
	c.Assert(nil, jc.ErrorIsNil)
	fix.SetErrors(errs...)
	return fix
}

func (fix *fixture) Run(c *gc.C, test func(worker.Worker)) {
	stubRetryStrategy := params.RetryStrategy{
		ShouldRetry: true,
	}
	stubTag := stubTag{}
	stubFacade := newStubFacade(c, &fix.Stub, stubRetryStrategy, stubTag)
	stubConfig := retrystrategy.WorkerConfig{
		Facade:        stubFacade,
		AgentTag:      stubTag,
		RetryStrategy: stubRetryStrategy,
		Logger:        loggo.GetLogger("test"),
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
	c               *gc.C
	stub            *testing.Stub
	watcher         *stubWatcher
	count           int
	shouldBounce    bool
	initialStrategy params.RetryStrategy
	stubTag         names.Tag
}

func newStubFacade(c *gc.C, stub *testing.Stub, initialStrategy params.RetryStrategy, stubTag names.Tag) *stubFacade {
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
func (f *stubFacade) WatchRetryStrategy(agentTag names.Tag) (watcher.NotifyWatcher, error) {
	f.c.Assert(agentTag, gc.Equals, f.stubTag)
	f.stub.AddCall("WatchRetryStrategy", agentTag)
	err := f.stub.NextErr()
	if err != nil {
		return nil, err
	}
	return f.watcher, nil
}

// RetryStrategy is part of the retrystrategy Facade
func (f *stubFacade) RetryStrategy(agentTag names.Tag) (params.RetryStrategy, error) {
	f.c.Assert(agentTag, gc.Equals, f.stubTag)
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
