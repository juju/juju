// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package workers_test

import (
	"time"

	"github.com/juju/errors"
	"github.com/juju/loggo"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/state/workers"
	jujutesting "github.com/juju/juju/testing"
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

// NextWorker reads a worker from the supplied channel and returns it,
// or times out. The result might be nil.
func NextWorker(c *gc.C, ch <-chan worker.Worker) worker.Worker {
	select {
	case worker := <-ch:
		return worker
	case <-time.After(jujutesting.LongWait):
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

// Run runs a test func inside a fresh Context.
func (fix Fixture) Run(c *gc.C, test func(Context)) {
	ctx := fix.newContext()
	defer ctx.cleanup(c)
	test(ctx)
}

// RunDumb starts a DumbWorkers inside a fresh Context and supplies it
// to a test func.
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

// FailDumb verifies that a DumbWorkers cannot start successfully, and
// checks that the returned error matches.
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

// RunRestart starts a RestartWorkers inside a fresh Context and
// supplies it to a test func.
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

// FailRestart verifies that a RestartWorkers cannot start successfully, and
// checks that the returned error matches.
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

// Next starts and returns the next configured worker, or an error.
// In either case, a value is sent on the worker channel.
func (wl *workerList) Next() (worker.Worker, error) {
	worker := wl.workers[wl.next]
	wl.next++
	wl.reports <- worker
	if worker == nil {
		return nil, errors.New("bad start")
	}
	return worker, nil
}

// cleanup checks that every expected worker has already been stopped by
// the SUT. (i.e.: don't set up more workers than your fixture needs).
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

// IsWorker returns true if `wrapped` is one of the above fake*Worker
// types (as returned by the factory methods) and also wraps the
// `expect` worker.
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

// AssertWorker fails if IsWorker returns false.
func AssertWorker(c *gc.C, wrapped interface{}, expect worker.Worker) {
	c.Assert(IsWorker(wrapped, expect), jc.IsTrue)
}

func LM_getter(w workers.Workers) func() interface{} {
	return func() interface{} { return w.LeadershipManager() }
}

func SM_getter(w workers.Workers) func() interface{} {
	return func() interface{} { return w.SingularManager() }
}

func TLW_getter(w workers.Workers) func() interface{} {
	return func() interface{} { return w.TxnLogWatcher() }
}

func PW_getter(w workers.Workers) func() interface{} {
	return func() interface{} { return w.PresenceWatcher() }
}

// WaitWorker blocks until getter returns something that satifies
// IsWorker, or until it times out.
func WaitWorker(c *gc.C, getter func() interface{}, expect worker.Worker) {
	var delay time.Duration
	timeout := time.After(jujutesting.LongWait)
	for {
		select {
		case <-timeout:
			c.Fatalf("expected worker")
		case <-time.After(delay):
			delay = jujutesting.ShortWait
		}
		if IsWorker(getter(), expect) {
			return
		}
	}
}

// WaitAlarms waits until the supplied clock has sent count values on
// its Alarms channel.
func WaitAlarms(c *gc.C, clock *testing.Clock, count int) {
	timeout := time.After(jujutesting.LongWait)
	for i := 0; i < count; i++ {
		select {
		case <-timeout:
			c.Fatalf("never saw alarm %d", i)
		case <-clock.Alarms():
		}
	}
}
