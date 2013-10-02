package addressupdater

import (
	"errors"
	stdtesting "testing"
	"time"

	gc "launchpad.net/gocheck"

	coretesting "launchpad.net/juju-core/testing"
	jc "launchpad.net/juju-core/testing/checkers"
	"launchpad.net/juju-core/testing/testbase"
)

func TestPackage(t *stdtesting.T) {
	gc.TestingT(t)
}

var _ = gc.Suite(&updaterSuite{})

type updaterSuite struct {
	testbase.LoggingSuite
}

func (*updaterSuite) TestStopsWatcher(c *gc.C) {
	context := &testPublisherContext{
		dyingc: make(chan struct{}),
	}
	expectErr := errors.New("some error")
	watcher := &testMachinesWatcher{
		changes: make(chan []string),
		err:     expectErr,
	}
	done := make(chan error)
	go func() {
		done <- watchMachinesLoop(context, watcher)
	}()
	close(context.dyingc)
	select {
	case err := <-done:
		c.Assert(err, gc.ErrorMatches, ".*"+expectErr.Error())
	case <-time.After(coretesting.LongWait):
		c.Fatalf("timed out waiting for watchMachinesLoop to terminate")
	}
	c.Assert(watcher.stopped, jc.IsTrue)
}

type testPublisherContext struct {
	newMachineContextFunc func() machineContext
	getMachineFunc        func(id string) (machine, error)
	dyingc                chan struct{}
}

func (context *testPublisherContext) newMachineContext() machineContext {
	return context.newMachineContextFunc()
}

func (context *testPublisherContext) getMachine(id string) (machine, error) {
	return context.getMachineFunc(id)
}

func (context *testPublisherContext) dying() <-chan struct{} {
	return context.dyingc
}

type testMachinesWatcher struct {
	stopped bool
	changes chan []string
	err     error
}

func (w *testMachinesWatcher) Changes() <-chan []string {
	return w.changes
}

func (w *testMachinesWatcher) Stop() error {
	w.stopped = true
	return w.err
}

func (w *testMachinesWatcher) Err() error {
	return w.err
}
