package main

import (
	"fmt"
	. "launchpad.net/gocheck"
	"launchpad.net/juju-core/state"
	"launchpad.net/juju-core/testing"
	"launchpad.net/juju-core/version"
	"launchpad.net/tomb"
	"time"
)

var _ = Suite(&agentSuite{})

type agentSuite struct {
	testing.LoggingSuite
}

func assertDead(c *C, tasks []*testTask) {
	for _, t := range tasks {
		c.Assert(t.Dead(), Equals, true)
	}
}

func (*agentSuite) TestRunTasksAllSuccess(c *C) {
	tasks := newTestTasks(4)
	for _, t := range tasks {
		t.kill <- nil
	}
	err := runTasks(make(chan struct{}), taskSlice(tasks)...)
	c.Assert(err, IsNil)
	assertDead(c, tasks)
}

func (*agentSuite) TestOneTaskError(c *C) {
	tasks := newTestTasks(4)
	for i, t := range tasks {
		if i == 2 {
			t.kill <- fmt.Errorf("kill")
		}
	}
	err := runTasks(make(chan struct{}), taskSlice(tasks)...)
	c.Assert(err, ErrorMatches, "kill")
	assertDead(c, tasks)
}

func (*agentSuite) TestTaskStop(c *C) {
	tasks := newTestTasks(4)
	tasks[2].stopErr = fmt.Errorf("stop")
	stop := make(chan struct{})
	close(stop)
	err := runTasks(stop, taskSlice(tasks)...)
	c.Assert(err, ErrorMatches, "stop")
	assertDead(c, tasks)
}

func (*agentSuite) TestUpgradeGetsPrecedence(c *C) {
	tasks := newTestTasks(2)
	tasks[1].stopErr = &UpgradedError{}
	go func() {
		time.Sleep(10 * time.Millisecond)
		tasks[0].kill <- fmt.Errorf("stop")
	}()
	err := runTasks(nil, taskSlice(tasks)...)
	c.Assert(err, Equals, tasks[1].stopErr)
	assertDead(c, tasks)
}

func mkTools(s string) *state.Tools {
	return &state.Tools{
		Binary: version.MustParseBinary(s + "-foo-bar"),
	}
}

func (*agentSuite) TestUpgradeErrorLog(c *C) {
	tasks := newTestTasks(7)
	tasks[0].stopErr = fmt.Errorf("zero")
	tasks[1].stopErr = fmt.Errorf("one")
	tasks[2].stopErr = &UpgradedError{mkTools("1.1.1")}
	tasks[3].kill <- fmt.Errorf("three")
	tasks[4].stopErr = fmt.Errorf("four")
	tasks[5].stopErr = &UpgradedError{mkTools("2.2.2")}
	tasks[6].stopErr = fmt.Errorf("six")

	expectLog := `
(.|\n)*task0: zero
.*task1: one
.*task2: must restart.*1\.1\.1.*
.*task3: three
.*task4: four
.*task6: six
(.|\n)*`[1:]

	err := runTasks(nil, taskSlice(tasks)...)
	c.Assert(err, Equals, tasks[5].stopErr)
	c.Assert(c.GetTestLog(), Matches, expectLog)
}

type testTask struct {
	name string
	tomb.Tomb
	kill    chan error
	stopErr error
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
	case err := <-t.kill:
		t.Kill(err)
		return
	}
}

func (t *testTask) String() string {
	return t.name
}

func newTestTasks(n int) []*testTask {
	tasks := make([]*testTask, n)
	for i := range tasks {
		tasks[i] = &testTask{
			kill: make(chan error, 1),
			name: fmt.Sprintf("task%d", i),
		}
		go tasks[i].run()
	}
	return tasks
}

func taskSlice(tasks []*testTask) []task {
	r := make([]task, len(tasks))
	for i, t := range tasks {
		r[i] = t
	}
	return r
}
