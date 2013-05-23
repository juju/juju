// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package worker_test

import (
	"errors"
	"fmt"
	. "launchpad.net/gocheck"
	"launchpad.net/juju-core/log"
	coretesting "launchpad.net/juju-core/testing"
	"launchpad.net/juju-core/worker"
	"launchpad.net/tomb"
	"sync/atomic"
	"time"
)

type runnerSuite struct {
	coretesting.LoggingSuite
	restartDelay time.Duration
}

var _ = Suite(&runnerSuite{})

func noneFatal(error) bool {
	return false
}

func allFatal(error) bool {
	return true
}

func noImportance(err0, err1 error) bool {
	return false
}

func (s *runnerSuite) SetUpTest(c *C) {
	s.LoggingSuite.SetUpTest(c)
	s.restartDelay = worker.RestartDelay
	worker.RestartDelay = 0
}

func (s *runnerSuite) TearDownTest(c *C) {
	worker.RestartDelay = s.restartDelay
	s.LoggingSuite.TearDownTest(c)
}

func (*runnerSuite) TestOneWorkerStart(c *C) {
	runner := worker.NewRunner(noneFatal, noImportance)
	starter := newTestWorkerStarter()
	err := runner.StartWorker("id", starter.start)
	c.Assert(err, IsNil)
	starter.assertStarted(c, true)

	c.Assert(worker.Stop(runner), IsNil)
	starter.assertStarted(c, false)
}

func (*runnerSuite) TestOneWorkerRestart(c *C) {
	runner := worker.NewRunner(noneFatal, noImportance)
	starter := newTestWorkerStarter()
	err := runner.StartWorker("id", starter.start)
	c.Assert(err, IsNil)
	starter.assertStarted(c, true)

	// Check it restarts a few times time.
	for i := 0; i < 3; i++ {
		starter.die <- fmt.Errorf("an error")
		starter.assertStarted(c, false)
		starter.assertStarted(c, true)
	}

	c.Assert(worker.Stop(runner), IsNil)
	starter.assertStarted(c, false)
}

func (*runnerSuite) TestOneWorkerStartFatalError(c *C) {
	runner := worker.NewRunner(allFatal, noImportance)
	starter := newTestWorkerStarter()
	starter.startErr = errors.New("cannot start test task")
	err := runner.StartWorker("id", starter.start)
	c.Assert(err, IsNil)
	err = runner.Wait()
	c.Assert(err, Equals, starter.startErr)
}

func (*runnerSuite) TestOneWorkerDieFatalError(c *C) {
	runner := worker.NewRunner(allFatal, noImportance)
	starter := newTestWorkerStarter()
	err := runner.StartWorker("id", starter.start)
	c.Assert(err, IsNil)
	starter.assertStarted(c, true)
	dieErr := errors.New("error when running")
	starter.die <- dieErr
	err = runner.Wait()
	c.Assert(err, Equals, dieErr)
	starter.assertStarted(c, false)
}

func (*runnerSuite) TestOneWorkerStartStop(c *C) {
	runner := worker.NewRunner(allFatal, noImportance)
	starter := newTestWorkerStarter()
	err := runner.StartWorker("id", starter.start)
	c.Assert(err, IsNil)
	starter.assertStarted(c, true)
	err = runner.StopWorker("id")
	c.Assert(err, IsNil)
	starter.assertStarted(c, false)
	c.Assert(worker.Stop(runner), IsNil)
}

func (*runnerSuite) TestOneWorkerStopFatalError(c *C) {
	runner := worker.NewRunner(allFatal, noImportance)
	starter := newTestWorkerStarter()
	starter.stopErr = errors.New("stop error")
	err := runner.StartWorker("id", starter.start)
	c.Assert(err, IsNil)
	starter.assertStarted(c, true)
	err = runner.StopWorker("id")
	c.Assert(err, IsNil)
	err = runner.Wait()
	c.Assert(err, Equals, starter.stopErr)
}

func (*runnerSuite) TestOneWorkerStartWhenStopping(c *C) {
	worker.RestartDelay = 3 * time.Second
	runner := worker.NewRunner(allFatal, noImportance)
	starter := newTestWorkerStarter()
	starter.stopWait = make(chan struct{})

	err := runner.StartWorker("id", starter.start)
	c.Assert(err, IsNil)
	starter.assertStarted(c, true)
	err = runner.StopWorker("id")
	c.Assert(err, IsNil)
	err = runner.StartWorker("id", starter.start)
	c.Assert(err, IsNil)

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
	c.Assert(worker.Stop(runner), IsNil)
}

func (*runnerSuite) TestOneWorkerRestartDelay(c *C) {
	worker.RestartDelay = 100 * time.Millisecond
	runner := worker.NewRunner(noneFatal, noImportance)
	starter := newTestWorkerStarter()
	err := runner.StartWorker("id", starter.start)
	c.Assert(err, IsNil)
	starter.assertStarted(c, true)
	starter.die <- fmt.Errorf("non-fatal error")
	starter.assertStarted(c, false)
	t0 := time.Now()
	starter.assertStarted(c, true)
	restartDuration := time.Since(t0)
	if restartDuration < worker.RestartDelay {
		c.Fatalf("restart delay was not respected; got %v want %v", restartDuration, worker.RestartDelay)
	}
}

type errorLevel int

func (e errorLevel) Error() string {
	return fmt.Sprintf("error with importance %d", e)
}

func (*runnerSuite) TestErrorImportance(c *C) {
	moreImportant := func(err0, err1 error) bool {
		return err0.(errorLevel) > err1.(errorLevel)
	}
	id := func(i int) string { return fmt.Sprint(i) }
	runner := worker.NewRunner(allFatal, moreImportant)
	for i := 0; i < 10; i++ {
		starter := newTestWorkerStarter()
		starter.stopErr = errorLevel(i)
		err := runner.StartWorker(id(i), starter.start)
		c.Assert(err, IsNil)
	}
	err := runner.StopWorker(id(4))
	c.Assert(err, IsNil)
	err = runner.Wait()
	c.Assert(err, Equals, errorLevel(9))
}

func (*runnerSuite) TestStartWorkerWhenDead(c *C) {
	runner := worker.NewRunner(allFatal, noImportance)
	c.Assert(worker.Stop(runner), IsNil)
	c.Assert(runner.StartWorker("foo", nil), Equals, worker.ErrDead)
}

func (*runnerSuite) TestStopWorkerWhenDead(c *C) {
	runner := worker.NewRunner(allFatal, noImportance)
	c.Assert(worker.Stop(runner), IsNil)
	c.Assert(runner.StopWorker("foo"), Equals, worker.ErrDead)
}

func (*runnerSuite) TestAllWorkersStoppedWhenOneDiesWithFatalError(c *C) {
	runner := worker.NewRunner(allFatal, noImportance)
	var starters []*testWorkerStarter
	for i := 0; i < 10; i++ {
		starter := newTestWorkerStarter()
		err := runner.StartWorker(fmt.Sprint(i), starter.start)
		c.Assert(err, IsNil)
		starters = append(starters, starter)
	}
	for _, starter := range starters {
		starter.assertStarted(c, true)
	}
	dieErr := errors.New("fatal error")
	starters[4].die <- dieErr
	err := runner.Wait()
	c.Assert(err, Equals, dieErr)
	for _, starter := range starters {
		starter.assertStarted(c, false)
	}
}

type testWorkerStarter struct {
	startCount  int32
	startNotify chan bool
	stopWait    chan struct{}
	die         chan error
	stopErr     error
	startErr    error
}

func newTestWorkerStarter() *testWorkerStarter {
	return &testWorkerStarter{
		die:         make(chan error, 1),
		startNotify: make(chan bool, 100),
	}
}

func (starter *testWorkerStarter) assertStarted(c *C, started bool) {
	select {
	case isStarted := <-starter.startNotify:
		c.Assert(isStarted, Equals, started)
	case <-time.After(1 * time.Second):
		c.Fatalf("timed out waiting for start notification")
	}
}

func (starter *testWorkerStarter) start() (worker.Worker, error) {
	if count := atomic.AddInt32(&starter.startCount, 1); count != 1 {
		panic(fmt.Errorf("unexpected start count %d; expected 1", count))
	}
	if starter.startErr != nil {
		return nil, starter.startErr
	}
	task := &testWorker{
		starter: starter,
	}
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
	t.starter.startNotify <- true
	select {
	case <-t.tomb.Dying():
		t.tomb.Kill(t.starter.stopErr)
	case err := <-t.starter.die:
		t.tomb.Kill(err)
	}
	if t.starter.stopWait != nil {
		log.Infof("waiting for stop")
		<-t.starter.stopWait
		log.Infof("stop request received")
	}
	t.starter.startNotify <- false
	if count := atomic.AddInt32(&t.starter.startCount, -1); count != 0 {
		panic(fmt.Errorf("unexpected start count %d; expected 0", count))
	}
}
