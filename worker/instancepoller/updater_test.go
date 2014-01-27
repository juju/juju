// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

// TODO(wallyworld) - move to instancepoller_test
package instancepoller

import (
	"errors"
	stdtesting "testing"
	"time"

	gc "launchpad.net/gocheck"

	"launchpad.net/juju-core/state"
	coretesting "launchpad.net/juju-core/testing"
	jc "launchpad.net/juju-core/testing/checkers"
	"launchpad.net/juju-core/testing/testbase"
)

func TestPackage(t *stdtesting.T) {
	coretesting.MgoTestPackage(t)
}

var _ = gc.Suite(&updaterSuite{})

type updaterSuite struct {
	testbase.LoggingSuite
}

func (*updaterSuite) TestStopsWatcher(c *gc.C) {
	context := &testUpdaterContext{
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

func (*updaterSuite) TestWatchMachinesWaitsForMachinePollers(c *gc.C) {
	// We can't see that the machine pollers are still alive directly,
	// but we can make the machine's Refresh method block,
	// and test that watchMachinesLoop only terminates
	// when it unblocks.
	waitRefresh := make(chan struct{})
	m := &testMachine{
		id:         "99",
		instanceId: "i1234",
		life:       state.Alive,
		refresh: func() error {
			// Signal that we're in Refresh.
			waitRefresh <- struct{}{}
			// Wait to be unblocked.
			<-waitRefresh
			return nil
		},
	}
	dyingc := make(chan struct{})
	context := &testUpdaterContext{
		dyingc: dyingc,
		newMachineContextFunc: func() machineContext {
			return &testMachineContext{
				getInstanceInfo: instanceInfoGetter(c, "i1234", testAddrs, "running", nil),
				dyingc:          dyingc,
			}
		},
		getMachineFunc: func(id string) (machine, error) {
			c.Check(id, gc.Equals, m.id)
			return m, nil
		},
	}
	watcher := &testMachinesWatcher{
		changes: make(chan []string),
	}
	done := make(chan error)
	go func() {
		done <- watchMachinesLoop(context, watcher)
	}()
	// Send two changes; the first one should start the machineLoop;
	// the second should call Refresh.
	watcher.changes <- []string{"99"}
	watcher.changes <- []string{"99"}
	// Wait for the machineLoop to call Refresh
	select {
	case <-waitRefresh:
		c.Logf("poller called Refresh")
	case <-time.After(coretesting.LongWait):
		c.Fatalf("timed out waiting for machine to be refreshed")
	}
	close(context.dyingc)
	// Wait a little while to be sure that watchMachinesLoop is
	// actually waiting for its machine poller to finish.
	select {
	case err := <-done:
		c.Fatalf("watchMachinesLoop terminated prematurely: %v", err)
	case <-time.After(coretesting.ShortWait):
	}

	waitRefresh <- struct{}{}
	select {
	case err := <-done:
		c.Assert(err, gc.IsNil)
	case <-time.After(coretesting.LongWait):
		c.Fatalf("timed out waiting for watchMachinesLoop to terminate")
	}
	c.Assert(watcher.stopped, jc.IsTrue)
}

type testUpdaterContext struct {
	newMachineContextFunc func() machineContext
	getMachineFunc        func(id string) (machine, error)
	dyingc                chan struct{}
}

func (context *testUpdaterContext) newMachineContext() machineContext {
	return context.newMachineContextFunc()
}

func (context *testUpdaterContext) getMachine(id string) (machine, error) {
	return context.getMachineFunc(id)
}

func (context *testUpdaterContext) dying() <-chan struct{} {
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
