// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

// TODO(wallyworld) - move to instancepoller_test
package instancepoller

import (
	"time"

	"github.com/juju/names"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/apiserver/params"
	coretesting "github.com/juju/juju/testing"
	"github.com/juju/juju/watcher"
)

var _ = gc.Suite(&updaterSuite{})

type updaterSuite struct {
	coretesting.BaseSuite
}

func (*updaterSuite) TestWatchMachinesWaitsForMachinePollers(c *gc.C) {
	// We can't see that the machine pollers are still alive directly,
	// but we can make the machine's Refresh method block,
	// and test that watchMachinesLoop only terminates
	// when it unblocks.
	waitRefresh := make(chan struct{})
	m := &testMachine{
		tag:        names.NewMachineTag("99"),
		instanceId: "i1234",
		life:       params.Alive,
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
		getMachineFunc: func(tag names.MachineTag) (machine, error) {
			c.Check(tag, jc.DeepEquals, m.tag)
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
		c.Assert(err, jc.ErrorIsNil)
	case <-time.After(coretesting.LongWait):
		c.Fatalf("timed out waiting for watchMachinesLoop to terminate")
	}
}

func (s *updaterSuite) TestManualMachinesIgnored(c *gc.C) {
	waitStatus := make(chan struct{})
	s.PatchValue(&MachineStatus, func(m *testMachine) (params.StatusResult, error) {
		// Signal that we're in Status.
		waitStatus <- struct{}{}
		return params.StatusResult{
			Status: params.StatusPending,
			Info:   "",
			Data:   map[string]interface{}{},
			Since:  nil,
		}, nil
	})
	m := &testMachine{
		tag:        names.NewMachineTag("99"),
		instanceId: "manual:1234",
		life:       params.Alive,
	}
	dyingc := make(chan struct{})
	context := &testUpdaterContext{
		dyingc: dyingc,
		newMachineContextFunc: func() machineContext {
			return &testMachineContext{
				getInstanceInfo: instanceInfoGetter(c, "manual:1234", testAddrs, "running", nil),
				dyingc:          dyingc,
			}
		},
		getMachineFunc: func(tag names.MachineTag) (machine, error) {
			c.Check(tag, jc.DeepEquals, m.tag)
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
	// Send a change to start the machineLoop;
	watcher.changes <- []string{"99"}
	select {
	case <-waitStatus:
		c.Fatalf("poller called Status")
	case <-time.After(coretesting.ShortWait):
		c.Logf("status not called")
	}
	close(context.dyingc)
	select {
	case err := <-done:
		c.Assert(err, jc.ErrorIsNil)
	case <-time.After(coretesting.LongWait):
		c.Fatalf("timed out waiting for watchMachinesLoop to terminate")
	}
}

type testUpdaterContext struct {
	updaterContext
	newMachineContextFunc func() machineContext
	getMachineFunc        func(tag names.MachineTag) (machine, error)
	dyingc                chan struct{}
}

func (context *testUpdaterContext) newMachineContext() machineContext {
	return context.newMachineContextFunc()
}

func (context *testUpdaterContext) getMachine(tag names.MachineTag) (machine, error) {
	return context.getMachineFunc(tag)
}

func (context *testUpdaterContext) dying() <-chan struct{} {
	return context.dyingc
}

func (context *testUpdaterContext) errDying() error {
	return nil
}

type testMachinesWatcher struct {
	watcher.StringsWatcher
	changes chan []string
	err     error
}

func (w *testMachinesWatcher) Changes() watcher.StringsChannel {
	return w.changes
}

func (w *testMachinesWatcher) Wait() error {
	return w.err
}
