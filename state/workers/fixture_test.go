// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package workers_test

import (
	"time"

	"github.com/juju/errors"
	"github.com/juju/loggo"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/state/workers"
	"github.com/juju/juju/testing"
	"github.com/juju/juju/worker"
	"github.com/juju/juju/worker/workertest"
)

const (
	fiveSeconds       = 5 * time.Second
	almostFiveSeconds = fiveSeconds - time.Nanosecond
)

// Context gives a test func access to the harness driving the Workers
// implementation under test.
type Context interface {
	Clock() *testing.Clock
	Factory() workers.Factory
	LWs() <-chan worker.Worker
	SWs() <-chan worker.Worker
	TLWs() <-chan worker.Worker
	PWs() <-chan worker.Worker
}

func NextWorker(c *gc.C, ch <-chan worker.Worker) worker.Worker {
	select {
	case worker := <-ch:
		return worker
	case <-time.After(testing.LongWait):
		c.Fatalf("expected worker never started")
	}
	panic("unreachable") // I hate doing this :-|.
}

// ErrFailStart can be used in any of a Fixture's *_errors fields to
// indicate that we should fail to start a worker.
var ErrFailStart = errors.New("test control value, should not be seen")

// BasicFixture returns a Fixture that expects each worker to be started
// once only, and to stop without error.
func BasicFixture() Fixture {
	return Fixture{
		LW_errors:  []error{nil},
		SW_errors:  []error{nil},
		TLW_errors: []error{nil},
		PW_errors:  []error{nil},
	}
}

// Fixture allows you to run tests against a DumbWorkers or a
// RestartWorkers by specifying a list of errors per worker.
type Fixture struct {
	LW_errors  []error
	SW_errors  []error
	TLW_errors []error
	PW_errors  []error
}

func (fix Fixture) Run(c *gc.C, test func(Context)) {
	ctx := fix.newContext()
	defer ctx.cleanup(c)
	test(ctx)
}

func (fix Fixture) RunDumb(c *gc.C, test func(Context, *workers.DumbWorkers)) {
	fix.Run(c, func(ctx Context) {
		dw, err := workers.NewDumbWorkers(workers.DumbConfig{
			Factory: ctx.Factory(),
			Logger:  loggo.GetLogger("test"),
		})
		c.Assert(err, jc.ErrorIsNil)
		defer workertest.DirtyKill(c, dw)
		test(ctx, dw)
	})
}

func (fix Fixture) FailDumb(c *gc.C, match string) {
	fix.Run(c, func(ctx Context) {
		dw, err := workers.NewDumbWorkers(workers.DumbConfig{
			Factory: ctx.Factory(),
			Logger:  loggo.GetLogger("test"),
		})
		if !c.Check(dw, gc.IsNil) {
			workertest.DirtyKill(c, dw)
		}
		c.Check(err, gc.ErrorMatches, match)
	})
}

func (fix Fixture) RunRestart(c *gc.C, test func(Context, *workers.RestartWorkers)) {
	fix.Run(c, func(ctx Context) {
		rw, err := workers.NewRestartWorkers(workers.RestartConfig{
			Factory: ctx.Factory(),
			Logger:  loggo.GetLogger("test"),
			Clock:   ctx.Clock(),
			Delay:   fiveSeconds,
		})
		c.Assert(err, jc.ErrorIsNil)
		defer workertest.DirtyKill(c, rw)
		test(ctx, rw)
	})
}

func (fix Fixture) FailRestart(c *gc.C, match string) {
	fix.Run(c, func(ctx Context) {
		rw, err := workers.NewRestartWorkers(workers.RestartConfig{
			Factory: ctx.Factory(),
			Logger:  loggo.GetLogger("test"),
			Clock:   ctx.Clock(),
			Delay:   fiveSeconds,
		})
		if !c.Check(rw, gc.IsNil) {
			workertest.DirtyKill(c, rw)
		}
		c.Check(err, gc.ErrorMatches, match)
	})
}

func (fix Fixture) newContext() *context {
	return &context{
		clock:   testing.NewClock(time.Now()),
		lwList:  newWorkerList(fix.LW_errors),
		swList:  newWorkerList(fix.SW_errors),
		tlwList: newWorkerList(fix.TLW_errors),
		pwList:  newWorkerList(fix.PW_errors),
	}
}

// newWorkerList converts the supplied errors into a list of workers to
// be returned in order from the result's Next func (at which point they
// are sent on the reports chan as well).
func newWorkerList(errs []error) *workerList {
	count := len(errs)
	reports := make(chan worker.Worker, count)
	workers := make([]worker.Worker, count)
	for i, err := range errs {
		if err == ErrFailStart {
			workers[i] = nil
		} else {
			workers[i] = workertest.NewErrorWorker(err)
		}
	}
	return &workerList{
		workers: workers,
		reports: reports,
	}
}

type workerList struct {
	next    int
	workers []worker.Worker
	reports chan worker.Worker
}

func (wl *workerList) Next() (worker.Worker, error) {
	worker := wl.workers[wl.next]
	wl.next++
	wl.reports <- worker
	if worker == nil {
		return nil, errors.New("bad start")
	}
	return worker, nil
}

func (wl *workerList) cleanup(c *gc.C) {
	for _, w := range wl.workers {
		if w != nil {
			workertest.CheckKilled(c, w)
		}
	}
}

// context implements Context.
type context struct {
	clock   *testing.Clock
	lwList  *workerList
	swList  *workerList
	tlwList *workerList
	pwList  *workerList
}

func (ctx *context) cleanup(c *gc.C) {
	c.Logf("cleaning up test context")
	for _, list := range []*workerList{
		ctx.lwList,
		ctx.swList,
		ctx.tlwList,
		ctx.pwList,
	} {
		list.cleanup(c)
	}
}

func (ctx *context) LWs() <-chan worker.Worker {
	return ctx.lwList.reports
}

func (ctx *context) SWs() <-chan worker.Worker {
	return ctx.swList.reports
}

func (ctx *context) TLWs() <-chan worker.Worker {
	return ctx.tlwList.reports
}

func (ctx *context) PWs() <-chan worker.Worker {
	return ctx.pwList.reports
}

func (ctx *context) Clock() *testing.Clock {
	return ctx.clock
}

func (ctx *context) Factory() workers.Factory {
	return &factory{ctx}
}

// factory implements workers.Factory for the convenience of the tests.
type factory struct {
	ctx *context
}

func (f *factory) NewLeadershipWorker() (workers.LeaseWorker, error) {
	worker, err := f.ctx.lwList.Next()
	if err != nil {
		return nil, err
	}
	return fakeLeaseWorker{Worker: worker}, nil
}

func (f *factory) NewSingularWorker() (workers.LeaseWorker, error) {
	worker, err := f.ctx.swList.Next()
	if err != nil {
		return nil, err
	}
	return fakeLeaseWorker{Worker: worker}, nil
}

func (f *factory) NewTxnLogWorker() (workers.TxnLogWorker, error) {
	worker, err := f.ctx.tlwList.Next()
	if err != nil {
		return nil, err
	}
	return fakeTxnLogWorker{Worker: worker}, nil
}

func (f *factory) NewPresenceWorker() (workers.PresenceWorker, error) {
	worker, err := f.ctx.pwList.Next()
	if err != nil {
		return nil, err
	}
	return fakePresenceWorker{Worker: worker}, nil
}

type fakeLeaseWorker struct {
	worker.Worker
	workers.LeaseManager
}

type fakeTxnLogWorker struct {
	worker.Worker
	workers.TxnLogWatcher
}

type fakePresenceWorker struct {
	worker.Worker
	workers.PresenceWatcher
}

func IsWorker(wrapped interface{}, expect worker.Worker) bool {
	var actual worker.Worker
	switch wrapped := wrapped.(type) {
	case fakeLeaseWorker:
		actual = wrapped.Worker
	case fakeTxnLogWorker:
		actual = wrapped.Worker
	case fakePresenceWorker:
		actual = wrapped.Worker
	default:
		return false
	}
	return actual == expect
}

func WaitWorker(c *gc.C, wrapped interface{}, expect worker.Worker) {
	var delay time.Duration
	timeout := time.After(testing.LongWait)
	for {
		select {
		case <-timeout:
			c.Fatalf("expected worker")
		case <-time.After(delay):
			delay = testing.ShortWait
		}
		if IsWorker(wrapped, expect) {
			return
		}
	}
}

func WaitAlarms(c *gc.C, clock *testing.Clock, count int) {
	timeout := time.After(testing.LongWait)
	for i := 0; i < count; i++ {
		select {
		case <-timeout:
			c.Fatalf("never saw alarm %d", i)
		case <-clock.Alarms():
		}
	}
}
