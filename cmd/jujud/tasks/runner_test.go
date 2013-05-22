package tasks_test

import (
	"errors"
	"fmt"
	. "launchpad.net/gocheck"
	"launchpad.net/juju-core/cmd/jujud/tasks"
	"launchpad.net/juju-core/log"
	coretesting "launchpad.net/juju-core/testing"
	"launchpad.net/tomb"
	"sync/atomic"
	"testing"
	"time"
)

func TestPackage(t *testing.T) {
	TestingT(t)
}

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
	s.restartDelay = tasks.RestartDelay
	tasks.RestartDelay = 0
}

func (s *runnerSuite) TearDownTest(c *C) {
	tasks.RestartDelay = s.restartDelay
	s.LoggingSuite.TearDownTest(c)
}

func (*runnerSuite) TestOneTaskStart(c *C) {
	runner := tasks.NewRunner(noneFatal, noImportance)
	starter := newTestTaskStarter()
	err := runner.StartTask("id", starter.start)
	c.Assert(err, IsNil)
	starter.assertStarted(c, true)

	err = runner.Stop()
	c.Assert(err, IsNil)
	starter.assertStarted(c, false)
}

func (*runnerSuite) TestOneTaskRestart(c *C) {
	runner := tasks.NewRunner(noneFatal, noImportance)
	starter := newTestTaskStarter()
	err := runner.StartTask("id", starter.start)
	c.Assert(err, IsNil)
	starter.assertStarted(c, true)

	// Check it restarts a few times time.
	for i := 0; i < 3; i++ {
		starter.die <- fmt.Errorf("an error")
		starter.assertStarted(c, false)
		starter.assertStarted(c, true)
	}

	c.Assert(runner.Stop(), IsNil)
	starter.assertStarted(c, false)
}

func (*runnerSuite) TestOneTaskStartFatalError(c *C) {
	runner := tasks.NewRunner(allFatal, noImportance)
	starter := newTestTaskStarter()
	starter.startErr = errors.New("cannot start test task")
	err := runner.StartTask("id", starter.start)
	c.Assert(err, IsNil)
	err = runner.Wait()
	c.Assert(err, Equals, starter.startErr)
}

func (*runnerSuite) TestOneTaskDieFatalError(c *C) {
	runner := tasks.NewRunner(allFatal, noImportance)
	starter := newTestTaskStarter()
	err := runner.StartTask("id", starter.start)
	c.Assert(err, IsNil)
	starter.assertStarted(c, true)
	dieErr := errors.New("error when running")
	starter.die <- dieErr
	err = runner.Wait()
	c.Assert(err, Equals, dieErr)
	starter.assertStarted(c, false)
}

func (*runnerSuite) TestOneTaskStartStop(c *C) {
	runner := tasks.NewRunner(allFatal, noImportance)
	starter := newTestTaskStarter()
	err := runner.StartTask("id", starter.start)
	c.Assert(err, IsNil)
	starter.assertStarted(c, true)
	err = runner.StopTask("id")
	c.Assert(err, IsNil)
	starter.assertStarted(c, false)
	c.Assert(runner.Stop(), IsNil)
}

func (*runnerSuite) TestOneTaskStopFatalError(c *C) {
	runner := tasks.NewRunner(allFatal, noImportance)
	starter := newTestTaskStarter()
	starter.stopErr = errors.New("stop error")
	err := runner.StartTask("id", starter.start)
	c.Assert(err, IsNil)
	starter.assertStarted(c, true)
	err = runner.StopTask("id")
	c.Assert(err, IsNil)
	err = runner.Wait()
	c.Assert(err, Equals, starter.stopErr)
}

func (*runnerSuite) TestOneTaskStartWhenStopping(c *C) {
	tasks.RestartDelay = 3 * time.Second
	runner := tasks.NewRunner(allFatal, noImportance)
	starter := newTestTaskStarter()
	starter.stopWait = make(chan struct{})

	err := runner.StartTask("id", starter.start)
	c.Assert(err, IsNil)
	starter.assertStarted(c, true)
	err = runner.StopTask("id")
	c.Assert(err, IsNil)
	err = runner.StartTask("id", starter.start)
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
	c.Assert(runner.Stop(), IsNil)
}

func (*runnerSuite) TestOneTaskRestartDelay(c *C) {
	tasks.RestartDelay = 100 * time.Millisecond
	runner := tasks.NewRunner(noneFatal, noImportance)
	starter := newTestTaskStarter()
	err := runner.StartTask("id", starter.start)
	c.Assert(err, IsNil)
	starter.assertStarted(c, true)
	starter.die <- fmt.Errorf("non-fatal error")
	starter.assertStarted(c, false)
	t0 := time.Now()
	starter.assertStarted(c, true)
	restartDuration := time.Since(t0)
	if restartDuration < tasks.RestartDelay {
		c.Fatalf("restart delay was not respected; got %v want %v", restartDuration, tasks.RestartDelay)
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
	runner := tasks.NewRunner(allFatal, moreImportant)
	for i := 0; i < 10; i++ {
		starter := newTestTaskStarter()
		starter.stopErr = errorLevel(i)
		err := runner.StartTask(id(i), starter.start)
		c.Assert(err, IsNil)
	}
	err := runner.StopTask(id(4))
	c.Assert(err, IsNil)
	err = runner.Wait()
	c.Assert(err, Equals, errorLevel(9))
}

func (*runnerSuite) TestStartTaskWhenDead(c *C) {
	runner := tasks.NewRunner(allFatal, noImportance)
	c.Assert(runner.Stop(), IsNil)
	c.Assert(runner.StartTask("foo", nil), Equals, tasks.ErrDead)
}

func (*runnerSuite) TestStopTaskWhenDead(c *C) {
	runner := tasks.NewRunner(allFatal, noImportance)
	c.Assert(runner.Stop(), IsNil)
	c.Assert(runner.StopTask("foo"), Equals, tasks.ErrDead)
}

func (*runnerSuite) TestAllTasksStoppedWhenOneDiesWithFatalError(c *C) {
	runner := tasks.NewRunner(allFatal, noImportance)
	var starters []*testTaskStarter
	for i := 0; i < 10; i++ {
		starter := newTestTaskStarter()
		err := runner.StartTask(fmt.Sprint(i), starter.start)
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

type testTaskStarter struct {
	startCount  int32
	startNotify chan bool
	stopWait    chan struct{}
	die         chan error
	stopErr     error
	startErr    error
}

func newTestTaskStarter() *testTaskStarter {
	return &testTaskStarter{
		die:         make(chan error, 1),
		startNotify: make(chan bool, 100),
	}
}

func (starter *testTaskStarter) assertStarted(c *C, started bool) {
	select {
	case isStarted := <-starter.startNotify:
		c.Assert(isStarted, Equals, started)
	case <-time.After(1 * time.Second):
		c.Fatalf("timed out waiting for start notification")
	}
}

func (starter *testTaskStarter) start() (tasks.Task, error) {
	if count := atomic.AddInt32(&starter.startCount, 1); count != 1 {
		panic(fmt.Errorf("unexpected start count %d; expected 1", count))
	}
	if starter.startErr != nil {
		return nil, starter.startErr
	}
	task := &testTask{
		starter: starter,
	}
	go task.run()
	return task, nil
}

type testTask struct {
	starter *testTaskStarter
	tomb    tomb.Tomb
}

func (t *testTask) Kill() {
	t.tomb.Kill(nil)
}

func (t *testTask) Wait() error {
	return t.tomb.Wait()
}

func (t *testTask) run() {
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
