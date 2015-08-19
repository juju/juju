// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package worker_test

import (
	"fmt"
	"sync/atomic"
	"time"

	"github.com/juju/errors"
	gitjujutesting "github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
	"launchpad.net/tomb"

	"github.com/juju/juju/testing"
	"github.com/juju/juju/worker"
	workertesting "github.com/juju/juju/worker/testing"
)

var (
	_ = gc.Suite(&runnerSuite{})
	_ = gc.Suite(&workersSuite{})
)

type runnerSuite struct {
	testing.BaseSuite
}

func noneFatal(error) bool {
	return false
}

func allFatal(error) bool {
	return true
}

func noImportance(err0, err1 error) bool {
	return false
}

func (s *runnerSuite) SetUpTest(c *gc.C) {
	s.BaseSuite.SetUpTest(c)
	// Avoid patching RestartDealy to zero, as it changes worker behaviour.
	s.PatchValue(&worker.RestartDelay, time.Duration(time.Millisecond))
}

func (*runnerSuite) TestOneWorkerStart(c *gc.C) {
	runner := worker.NewRunner(noneFatal, noImportance)
	starter := newTestWorkerStarter()
	err := runner.StartWorker("id", testWorkerStart(starter))
	c.Assert(err, jc.ErrorIsNil)
	starter.assertStarted(c, true)

	c.Assert(worker.Stop(runner), gc.IsNil)
	starter.assertStarted(c, false)
}

func (*runnerSuite) TestOneWorkerFinish(c *gc.C) {
	runner := worker.NewRunner(noneFatal, noImportance)
	starter := newTestWorkerStarter()
	err := runner.StartWorker("id", testWorkerStart(starter))
	c.Assert(err, jc.ErrorIsNil)
	starter.assertStarted(c, true)

	starter.die <- nil
	starter.assertStarted(c, false)
	starter.assertNeverStarted(c)

	c.Assert(worker.Stop(runner), gc.IsNil)
}

func (*runnerSuite) TestOneWorkerRestart(c *gc.C) {
	runner := worker.NewRunner(noneFatal, noImportance)
	starter := newTestWorkerStarter()
	err := runner.StartWorker("id", testWorkerStart(starter))
	c.Assert(err, jc.ErrorIsNil)
	starter.assertStarted(c, true)

	// Check it restarts a few times time.
	for i := 0; i < 3; i++ {
		starter.die <- fmt.Errorf("an error")
		starter.assertStarted(c, false)
		starter.assertStarted(c, true)
	}

	c.Assert(worker.Stop(runner), gc.IsNil)
	starter.assertStarted(c, false)
}

func (*runnerSuite) TestOneWorkerStartFatalError(c *gc.C) {
	runner := worker.NewRunner(allFatal, noImportance)
	starter := newTestWorkerStarter()
	starter.startErr = errors.New("cannot start test task")
	err := runner.StartWorker("id", testWorkerStart(starter))
	c.Assert(err, jc.ErrorIsNil)
	err = runner.Wait()
	c.Assert(err, gc.Equals, starter.startErr)
}

func (*runnerSuite) TestOneWorkerDieFatalError(c *gc.C) {
	runner := worker.NewRunner(allFatal, noImportance)
	starter := newTestWorkerStarter()
	err := runner.StartWorker("id", testWorkerStart(starter))
	c.Assert(err, jc.ErrorIsNil)
	starter.assertStarted(c, true)
	dieErr := errors.New("error when running")
	starter.die <- dieErr
	err = runner.Wait()
	c.Assert(err, gc.Equals, dieErr)
	starter.assertStarted(c, false)
}

func (*runnerSuite) TestOneWorkerStartStop(c *gc.C) {
	runner := worker.NewRunner(allFatal, noImportance)
	starter := newTestWorkerStarter()
	err := runner.StartWorker("id", testWorkerStart(starter))
	c.Assert(err, jc.ErrorIsNil)
	starter.assertStarted(c, true)
	err = runner.StopWorker("id")
	c.Assert(err, jc.ErrorIsNil)
	starter.assertStarted(c, false)
	c.Assert(worker.Stop(runner), gc.IsNil)
}

func (*runnerSuite) TestOneWorkerStopFatalError(c *gc.C) {
	runner := worker.NewRunner(allFatal, noImportance)
	starter := newTestWorkerStarter()
	starter.stopErr = errors.New("stop error")
	err := runner.StartWorker("id", testWorkerStart(starter))
	c.Assert(err, jc.ErrorIsNil)
	starter.assertStarted(c, true)
	err = runner.StopWorker("id")
	c.Assert(err, jc.ErrorIsNil)
	err = runner.Wait()
	c.Assert(err, gc.Equals, starter.stopErr)
}

func (*runnerSuite) TestOneWorkerStartWhenStopping(c *gc.C) {
	worker.RestartDelay = 3 * time.Second
	runner := worker.NewRunner(allFatal, noImportance)
	starter := newTestWorkerStarter()
	starter.stopWait = make(chan struct{})

	err := runner.StartWorker("id", testWorkerStart(starter))
	c.Assert(err, jc.ErrorIsNil)
	starter.assertStarted(c, true)
	err = runner.StopWorker("id")
	c.Assert(err, jc.ErrorIsNil)
	err = runner.StartWorker("id", testWorkerStart(starter))
	c.Assert(err, jc.ErrorIsNil)

	close(starter.stopWait)
	starter.assertStarted(c, false)
	// Check that the task is restarted immediately without
	// the usual restart timeout delay.
	t0 := time.Now()
	starter.assertStarted(c, true)
	restartDuration := time.Since(t0)
	if restartDuration > 1*time.Second {
		c.Fatalf("task did not restart immediately")
	}
	c.Assert(worker.Stop(runner), gc.IsNil)
}

func (*runnerSuite) TestOneWorkerRestartDelay(c *gc.C) {
	worker.RestartDelay = 100 * time.Millisecond
	runner := worker.NewRunner(noneFatal, noImportance)
	starter := newTestWorkerStarter()
	err := runner.StartWorker("id", testWorkerStart(starter))
	c.Assert(err, jc.ErrorIsNil)
	starter.assertStarted(c, true)
	starter.die <- fmt.Errorf("non-fatal error")
	starter.assertStarted(c, false)
	t0 := time.Now()
	starter.assertStarted(c, true)
	restartDuration := time.Since(t0)
	if restartDuration < worker.RestartDelay {
		c.Fatalf("restart delay was not respected; got %v want %v", restartDuration, worker.RestartDelay)
	}
	c.Assert(worker.Stop(runner), gc.IsNil)
}

type errorLevel int

func (e errorLevel) Error() string {
	return fmt.Sprintf("error with importance %d", e)
}

func (*runnerSuite) TestErrorImportance(c *gc.C) {
	moreImportant := func(err0, err1 error) bool {
		return err0.(errorLevel) > err1.(errorLevel)
	}
	id := func(i int) string { return fmt.Sprint(i) }
	runner := worker.NewRunner(allFatal, moreImportant)
	for i := 0; i < 10; i++ {
		starter := newTestWorkerStarter()
		starter.stopErr = errorLevel(i)
		err := runner.StartWorker(id(i), testWorkerStart(starter))
		c.Assert(err, jc.ErrorIsNil)
	}
	err := runner.StopWorker(id(4))
	c.Assert(err, jc.ErrorIsNil)
	err = runner.Wait()
	c.Assert(err, gc.Equals, errorLevel(9))
}

func (*runnerSuite) TestStartWorkerWhenDead(c *gc.C) {
	runner := worker.NewRunner(allFatal, noImportance)
	c.Assert(worker.Stop(runner), gc.IsNil)
	c.Assert(runner.StartWorker("foo", nil), gc.Equals, worker.ErrDead)
}

func (*runnerSuite) TestStopWorkerWhenDead(c *gc.C) {
	runner := worker.NewRunner(allFatal, noImportance)
	c.Assert(worker.Stop(runner), gc.IsNil)
	c.Assert(runner.StopWorker("foo"), gc.Equals, worker.ErrDead)
}

func (*runnerSuite) TestAllWorkersStoppedWhenOneDiesWithFatalError(c *gc.C) {
	runner := worker.NewRunner(allFatal, noImportance)
	var starters []*testWorkerStarter
	for i := 0; i < 10; i++ {
		starter := newTestWorkerStarter()
		err := runner.StartWorker(fmt.Sprint(i), testWorkerStart(starter))
		c.Assert(err, jc.ErrorIsNil)
		starters = append(starters, starter)
	}
	for _, starter := range starters {
		starter.assertStarted(c, true)
	}
	dieErr := errors.New("fatal error")
	starters[4].die <- dieErr
	err := runner.Wait()
	c.Assert(err, gc.Equals, dieErr)
	for _, starter := range starters {
		starter.assertStarted(c, false)
	}
}

func (*runnerSuite) TestFatalErrorWhileStarting(c *gc.C) {
	// Original deadlock problem that this tests for:
	// A worker dies with fatal error while another worker
	// is inside start(). runWorker can't send startInfo on startedc.
	runner := worker.NewRunner(allFatal, noImportance)

	slowStarter := newTestWorkerStarter()
	// make the startNotify channel synchronous so
	// we can delay the start indefinitely.
	slowStarter.startNotify = make(chan bool)

	err := runner.StartWorker("slow starter", testWorkerStart(slowStarter))
	c.Assert(err, jc.ErrorIsNil)

	fatalStarter := newTestWorkerStarter()
	fatalStarter.startErr = fmt.Errorf("a fatal error")

	err = runner.StartWorker("fatal worker", testWorkerStart(fatalStarter))
	c.Assert(err, jc.ErrorIsNil)

	// Wait for the runner loop to react to the fatal
	// error and go into final shutdown mode.
	time.Sleep(10 * time.Millisecond)

	// At this point, the loop is in shutdown mode, but the
	// slowStarter's worker is still in its start function.
	// When the start function continues (the first assertStarted
	// allows that to happen) and returns the new Worker,
	// runWorker will try to send it on runner.startedc.
	// This test makes sure that succeeds ok.

	slowStarter.assertStarted(c, true)
	slowStarter.assertStarted(c, false)
	err = runner.Wait()
	c.Assert(err, gc.Equals, fatalStarter.startErr)
}

func (*runnerSuite) TestFatalErrorWhileSelfStartWorker(c *gc.C) {
	// Original deadlock problem that this tests for:
	// A worker tries to call StartWorker in its start function
	// at the same time another worker dies with a fatal error.
	// It might not be able to send on startc.
	runner := worker.NewRunner(allFatal, noImportance)

	selfStarter := newTestWorkerStarter()
	// make the startNotify channel synchronous so
	// we can delay the start indefinitely.
	selfStarter.startNotify = make(chan bool)
	selfStarter.hook = func() {
		runner.StartWorker("another", func() (worker.Worker, error) {
			return nil, fmt.Errorf("no worker started")
		})
	}
	err := runner.StartWorker("self starter", testWorkerStart(selfStarter))
	c.Assert(err, jc.ErrorIsNil)

	fatalStarter := newTestWorkerStarter()
	fatalStarter.startErr = fmt.Errorf("a fatal error")

	err = runner.StartWorker("fatal worker", testWorkerStart(fatalStarter))
	c.Assert(err, jc.ErrorIsNil)

	// Wait for the runner loop to react to the fatal
	// error and go into final shutdown mode.
	time.Sleep(10 * time.Millisecond)

	// At this point, the loop is in shutdown mode, but the
	// selfStarter's worker is still in its start function.
	// When the start function continues (the first assertStarted
	// allows that to happen) it will try to create a new
	// worker. This failed in an earlier version of the code because the
	// loop was not ready to receive start requests.

	selfStarter.assertStarted(c, true)
	selfStarter.assertStarted(c, false)
	err = runner.Wait()
	c.Assert(err, gc.Equals, fatalStarter.startErr)
}

type testWorkerStarter struct {
	startCount int32

	// startNotify receives true when the worker starts
	// and false when it exits. If startErr is non-nil,
	// it sends false only.
	startNotify chan bool

	// If stopWait is non-nil, the worker will
	// wait for a value to be sent on it before
	// exiting.
	stopWait chan struct{}

	// Sending a value on die causes the worker
	// to die with the given error.
	die chan error

	// If startErr is non-nil, the worker will die immediately
	// with this error after starting.
	startErr error

	// If stopErr is non-nil, the worker will die with this
	// error when asked to stop.
	stopErr error

	// The hook function is called after starting the worker.
	hook func()
}

func newTestWorkerStarter() *testWorkerStarter {
	return &testWorkerStarter{
		die:         make(chan error, 1),
		startNotify: make(chan bool, 100),
		hook:        func() {},
	}
}

func (starter *testWorkerStarter) assertStarted(c *gc.C, started bool) {
	select {
	case isStarted := <-starter.startNotify:
		c.Assert(isStarted, gc.Equals, started)
	case <-time.After(1 * time.Second):
		c.Fatalf("timed out waiting for start notification")
	}
}

func (starter *testWorkerStarter) assertNeverStarted(c *gc.C) {
	select {
	case isStarted := <-starter.startNotify:
		c.Fatalf("got unexpected start notification: %v", isStarted)
	case <-time.After(worker.RestartDelay + testing.ShortWait):
	}
}

func testWorkerStart(starter *testWorkerStarter) func() (worker.Worker, error) {
	return func() (worker.Worker, error) {
		return starter.start()
	}
}

func (starter *testWorkerStarter) start() (worker.Worker, error) {
	if count := atomic.AddInt32(&starter.startCount, 1); count != 1 {
		panic(fmt.Errorf("unexpected start count %d; expected 1", count))
	}
	if starter.startErr != nil {
		starter.startNotify <- false
		return nil, starter.startErr
	}
	task := &testWorker{
		starter: starter,
	}
	starter.startNotify <- true
	go task.run()
	return task, nil
}

type testWorker struct {
	starter *testWorkerStarter
	tomb    tomb.Tomb
}

func (t *testWorker) Kill() {
	t.tomb.Kill(nil)
}

func (t *testWorker) Wait() error {
	return t.tomb.Wait()
}

func (t *testWorker) run() {
	defer t.tomb.Done()

	t.starter.hook()
	select {
	case <-t.tomb.Dying():
		t.tomb.Kill(t.starter.stopErr)
	case err := <-t.starter.die:
		t.tomb.Kill(err)
	}
	if t.starter.stopWait != nil {
		<-t.starter.stopWait
	}
	t.starter.startNotify <- false
	if count := atomic.AddInt32(&t.starter.startCount, -1); count != 0 {
		panic(fmt.Errorf("unexpected start count %d; expected 0", count))
	}
}

type workersSuite struct {
	testing.BaseSuite

	calls []string
	stub  *gitjujutesting.Stub
}

func (s *workersSuite) SetUpTest(c *gc.C) {
	s.BaseSuite.SetUpTest(c)

	s.stub = &gitjujutesting.Stub{}
	s.calls = nil
}

func (s *workersSuite) newWorkerFunc(id string) func() (worker.Worker, error) {
	return func() (worker.Worker, error) {
		s.calls = append(s.calls, id)
		return nil, nil
	}
}

func (*workersSuite) TestNewWorkers(c *gc.C) {
	workers := worker.NewWorkers()
	ids, funcs := worker.ExtractWorkers(workers)

	c.Check(ids, gc.HasLen, 0)
	c.Check(funcs, gc.HasLen, 0)
}

func (*workersSuite) TestIDsOkay(c *gc.C) {
	newWorker := func() (worker.Worker, error) { return nil, nil }

	workers := worker.NewWorkers()
	err := workers.Add("spam", newWorker)
	c.Assert(err, jc.ErrorIsNil)
	err = workers.Add("eggs", newWorker)
	c.Assert(err, jc.ErrorIsNil)
	ids := workers.IDs()

	c.Check(ids, jc.DeepEquals, []string{"spam", "eggs"})
}

func (*workersSuite) TestIDsEmpty(c *gc.C) {
	workers := worker.NewWorkers()
	ids := workers.IDs()

	c.Check(ids, gc.HasLen, 0)
}

func (s *workersSuite) TestAddOkay(c *gc.C) {
	workers := worker.NewWorkers()
	expected := []string{"spam", "eggs"}
	for _, id := range expected {
		err := workers.Add(id, s.newWorkerFunc(id))
		c.Assert(err, jc.ErrorIsNil)
	}
	ids, funcs := worker.ExtractWorkers(workers)

	c.Check(ids, jc.DeepEquals, expected)
	// We can't compare functions so we work around it.
	for _, newWorker := range funcs {
		newWorker()
	}
	c.Check(s.calls, jc.DeepEquals, expected)
}

func (*workersSuite) TestAddAlreadyRegistered(c *gc.C) {
	newWorker := func() (worker.Worker, error) { return nil, nil }

	workers := worker.NewWorkers()
	err := workers.Add("spam", newWorker)
	c.Assert(err, jc.ErrorIsNil)
	err = workers.Add("spam", newWorker)

	c.Check(err, gc.ErrorMatches, `.*already registered.*`)
}

func (s *workersSuite) TestStartOkay(c *gc.C) {
	runner := workertesting.NewStubRunner(s.stub)
	runner.CallWhenStarted = true

	workers := worker.NewWorkers()
	expected := []string{"spam", "eggs", "ham"}
	for _, id := range expected {
		err := workers.Add(id, s.newWorkerFunc(id))
		c.Assert(err, jc.ErrorIsNil)
	}
	err := workers.Start(runner)
	c.Assert(err, jc.ErrorIsNil)

	// We would use s.stub.CheckCalls if functions could be compared...
	runner.CheckCallIDs(c, "StartWorker", expected...)
	c.Check(s.calls, jc.DeepEquals, expected)
}

func (s *workersSuite) TestStartError(c *gc.C) {
	runner := workertesting.NewStubRunner(s.stub)
	failure := errors.Errorf("<failed>")
	s.stub.SetErrors(nil, failure)

	workers := worker.NewWorkers()
	expected := []string{"spam", "eggs", "ham"}
	for _, id := range expected {
		err := workers.Add(id, s.newWorkerFunc(id))
		c.Assert(err, jc.ErrorIsNil)
	}
	err := workers.Start(runner)

	s.stub.CheckCallNames(c, "StartWorker", "StartWorker")
	c.Check(errors.Cause(err), gc.Equals, failure)
}
