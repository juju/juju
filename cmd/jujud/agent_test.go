package main
import (
	"fmt"
	. "launchpad.net/gocheck"
	"launchpad.net/tomb"
)

var _ = Suite(agentSuite{})

type agentSuite struct{}

func assertDead(c *C, tasks []task) {
	for _, t := range tasks {
		c.Assert(t.(*testTask).Dead(), Equals, true)
	}
}

func (agentSuite) TestRunTasksAllSuccess(c *C) {
	tasks := make([]task, 4)
	for i := range tasks {
		t := newTestTask()
		t.done <- nil
		tasks[i] = t
	}
	err := runTasks(make(chan struct{}), tasks...)
	c.Assert(err, IsNil)
	assertDead(c, tasks)
}

func (agentSuite) TestOneTaskError(c *C) {
	tasks := make([]task, 4)
	for i := range tasks {
		t := newTestTask()
		if i == 2 {
			t.done <- fmt.Errorf("an error")
		}
		tasks[i] = t
	}
	err := runTasks(make(chan struct{}), tasks...)
	c.Assert(err, ErrorMatches, "an error")
	assertDead(c, tasks)
}

func (agentSuite) TestTaskStop(c *C) {
	tasks := make([]task, 4)
	for i := range tasks {
		t := newTestTask()
		if i == 2 {
			t.stopErr = fmt.Errorf("a stop error")
		}
		tasks[i] = t
	}
	done := make(chan error)
	stop := make(chan struct{})
	go func() {
		done <- runTasks(stop, tasks...)
	}()
	close(stop)
	c.Assert(<-done, ErrorMatches, "a stop error")
	assertDead(c, tasks)
}

type testTask struct {
	tomb.Tomb
	done chan error
	stopErr error
}

func newTestTask() *testTask {
	t := &testTask{
		done: make(chan error, 1),
	}
	go t.run()
	return t
}

func (t *testTask) Stop() error {
	t.Kill(nil)
	return t.Wait()
}

func (t *testTask) Dead() bool {
	select {
	case <-t.Tomb.Dead():
		return true
	default:
	}
	return false
}

func (t *testTask) run() {
	defer t.Done()
	select {
	case <-t.Dying():
		t.Kill(t.stopErr)
	case err := <-t.done:
		t.Kill(err)
		return
	}
}
