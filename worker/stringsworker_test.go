// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package worker_test

import (
	"fmt"
	"sync"
	"time"

	gc "launchpad.net/gocheck"
	"launchpad.net/tomb"

	apiWatcher "launchpad.net/juju-core/state/api/watcher"
	"launchpad.net/juju-core/state/watcher"
	coretesting "launchpad.net/juju-core/testing"
	jc "launchpad.net/juju-core/testing/checkers"
	"launchpad.net/juju-core/testing/testbase"
	"launchpad.net/juju-core/worker"
)

type stringsWorkerSuite struct {
	testbase.LoggingSuite
	worker worker.Worker
	actor  *stringsHandler
}

var _ = gc.Suite(&stringsWorkerSuite{})

func (s *stringsWorkerSuite) SetUpTest(c *gc.C) {
	s.LoggingSuite.SetUpTest(c)
	s.actor = &stringsHandler{
		actions: nil,
		handled: make(chan []string, 1),
		watcher: &testStringsWatcher{
			changes: make(chan []string),
		},
	}
	s.worker = worker.NewStringsWorker(s.actor)
}

func (s *stringsWorkerSuite) TearDownTest(c *gc.C) {
	s.stopWorker(c)
	s.LoggingSuite.TearDownTest(c)
}

type stringsHandler struct {
	actions []string
	mu      sync.Mutex
	// Signal handled when we get a handle() call
	handled       chan []string
	setupError    error
	teardownError error
	handlerError  error
	watcher       *testStringsWatcher
}

var _ worker.StringsWatchHandler = (*stringsHandler)(nil)

func (sh *stringsHandler) SetUp() (apiWatcher.StringsWatcher, error) {
	sh.mu.Lock()
	defer sh.mu.Unlock()
	sh.actions = append(sh.actions, "setup")
	if sh.watcher == nil {
		return nil, sh.setupError
	}
	return sh.watcher, sh.setupError
}

func (sh *stringsHandler) TearDown() error {
	sh.mu.Lock()
	defer sh.mu.Unlock()
	sh.actions = append(sh.actions, "teardown")
	if sh.handled != nil {
		close(sh.handled)
	}
	return sh.teardownError
}

func (sh *stringsHandler) Handle(changes []string) error {
	sh.mu.Lock()
	defer sh.mu.Unlock()
	sh.actions = append(sh.actions, "handler")
	if sh.handled != nil {
		// Unlock while we are waiting for the send
		sh.mu.Unlock()
		sh.handled <- changes
		sh.mu.Lock()
	}
	return sh.handlerError
}

func (sh *stringsHandler) CheckActions(c *gc.C, actions ...string) {
	sh.mu.Lock()
	defer sh.mu.Unlock()
	c.Check(sh.actions, gc.DeepEquals, actions)
}

// During teardown we try to stop the worker, but don't hang the test suite if
// Stop never returns
func (s *stringsWorkerSuite) stopWorker(c *gc.C) {
	if s.worker == nil {
		return
	}
	done := make(chan error)
	go func() {
		done <- worker.Stop(s.worker)
	}()
	err := waitForTimeout(c, done, coretesting.LongWait)
	c.Check(err, gc.IsNil)
	s.actor = nil
	s.worker = nil
}

type testStringsWatcher struct {
	mu        sync.Mutex
	changes   chan []string
	stopped   bool
	stopError error
}

var _ apiWatcher.StringsWatcher = (*testStringsWatcher)(nil)

func (tsw *testStringsWatcher) Changes() <-chan []string {
	return tsw.changes
}

func (tsw *testStringsWatcher) Err() error {
	return tsw.stopError
}

func (tsw *testStringsWatcher) Stop() error {
	tsw.mu.Lock()
	defer tsw.mu.Unlock()
	if !tsw.stopped {
		close(tsw.changes)
	}
	tsw.stopped = true
	return tsw.stopError
}

func (tsw *testStringsWatcher) SetStopError(err error) {
	tsw.mu.Lock()
	tsw.stopError = err
	tsw.mu.Unlock()
}

func (tsw *testStringsWatcher) TriggerChange(c *gc.C, changes []string) {
	select {
	case tsw.changes <- changes:
	case <-time.After(coretesting.LongWait):
		c.Errorf("timed out trying to trigger a change")
	}
}

func waitForHandledStrings(c *gc.C, handled chan []string, expect []string) {
	select {
	case changes := <-handled:
		c.Assert(changes, gc.DeepEquals, expect)
	case <-time.After(coretesting.LongWait):
		c.Errorf("handled failed to signal after %s", coretesting.LongWait)
	}
}

func (s *stringsWorkerSuite) TestKill(c *gc.C) {
	s.worker.Kill()
	err := waitShort(c, s.worker)
	c.Assert(err, gc.IsNil)
}

func (s *stringsWorkerSuite) TestStop(c *gc.C) {
	err := worker.Stop(s.worker)
	c.Assert(err, gc.IsNil)
	// After stop, Wait should return right away
	err = waitShort(c, s.worker)
	c.Assert(err, gc.IsNil)
}

func (s *stringsWorkerSuite) TestWait(c *gc.C) {
	done := make(chan error)
	go func() {
		done <- s.worker.Wait()
	}()
	// Wait should not return until we've killed the worker
	select {
	case err := <-done:
		c.Errorf("Wait() didn't wait until we stopped it: %v", err)
	case <-time.After(coretesting.ShortWait):
	}
	s.worker.Kill()
	err := waitForTimeout(c, done, coretesting.LongWait)
	c.Assert(err, gc.IsNil)
}

func (s *stringsWorkerSuite) TestCallSetUpAndTearDown(c *gc.C) {
	// After calling NewStringsWorker, we should have called setup
	s.actor.CheckActions(c, "setup")
	// If we kill the worker, it should notice, and call teardown
	s.worker.Kill()
	err := waitShort(c, s.worker)
	c.Check(err, gc.IsNil)
	s.actor.CheckActions(c, "setup", "teardown")
	c.Check(s.actor.watcher.stopped, jc.IsTrue)
}

func (s *stringsWorkerSuite) TestChangesTriggerHandler(c *gc.C) {
	s.actor.CheckActions(c, "setup")
	s.actor.watcher.TriggerChange(c, []string{"aa", "bb"})
	waitForHandledStrings(c, s.actor.handled, []string{"aa", "bb"})
	s.actor.CheckActions(c, "setup", "handler")
	s.actor.watcher.TriggerChange(c, []string{"cc", "dd"})
	waitForHandledStrings(c, s.actor.handled, []string{"cc", "dd"})
	s.actor.watcher.TriggerChange(c, []string{"ee", "ff"})
	waitForHandledStrings(c, s.actor.handled, []string{"ee", "ff"})
	s.actor.CheckActions(c, "setup", "handler", "handler", "handler")
	c.Assert(worker.Stop(s.worker), gc.IsNil)
	s.actor.CheckActions(c, "setup", "handler", "handler", "handler", "teardown")
}

func (s *stringsWorkerSuite) TestSetUpFailureStopsWithTearDown(c *gc.C) {
	// Stop the worker and SetUp again, this time with an error
	s.stopWorker(c)
	actor := &stringsHandler{
		actions:    nil,
		handled:    make(chan []string, 1),
		setupError: fmt.Errorf("my special error"),
		watcher: &testStringsWatcher{
			changes: make(chan []string),
		},
	}
	w := worker.NewStringsWorker(actor)
	err := waitShort(c, w)
	c.Check(err, gc.ErrorMatches, "my special error")
	// TearDown is not called on SetUp error.
	actor.CheckActions(c, "setup")
	c.Check(actor.watcher.stopped, jc.IsTrue)
}

func (s *stringsWorkerSuite) TestWatcherStopFailurePropagates(c *gc.C) {
	s.actor.watcher.SetStopError(fmt.Errorf("error while stopping watcher"))
	s.worker.Kill()
	c.Assert(s.worker.Wait(), gc.ErrorMatches, "error while stopping watcher")
	// We've already stopped the worker, don't let teardown notice the
	// worker is in an error state
	s.worker = nil
}

func (s *stringsWorkerSuite) TestCleanRunNoticesTearDownError(c *gc.C) {
	s.actor.teardownError = fmt.Errorf("failed to tear down watcher")
	s.worker.Kill()
	c.Assert(s.worker.Wait(), gc.ErrorMatches, "failed to tear down watcher")
	s.worker = nil
}

func (s *stringsWorkerSuite) TestHandleErrorStopsWorkerAndWatcher(c *gc.C) {
	s.stopWorker(c)
	actor := &stringsHandler{
		actions:      nil,
		handled:      make(chan []string, 1),
		handlerError: fmt.Errorf("my handling error"),
		watcher: &testStringsWatcher{
			changes: make(chan []string),
		},
	}
	w := worker.NewStringsWorker(actor)
	actor.watcher.TriggerChange(c, []string{"aa", "bb"})
	waitForHandledStrings(c, actor.handled, []string{"aa", "bb"})
	err := waitShort(c, w)
	c.Check(err, gc.ErrorMatches, "my handling error")
	actor.CheckActions(c, "setup", "handler", "teardown")
	c.Check(actor.watcher.stopped, jc.IsTrue)
}

func (s *stringsWorkerSuite) TestNoticesStoppedWatcher(c *gc.C) {
	// The default closedHandler doesn't panic if you have a genuine error
	// (because it assumes you want to propagate a real error and then
	// restart
	s.actor.watcher.SetStopError(fmt.Errorf("Stopped Watcher"))
	s.actor.watcher.Stop()
	err := waitShort(c, s.worker)
	c.Check(err, gc.ErrorMatches, "Stopped Watcher")
	s.actor.CheckActions(c, "setup", "teardown")
	// Worker is stopped, don't fail TearDownTest
	s.worker = nil
}

func (s *stringsWorkerSuite) TestErrorsOnStillAliveButClosedChannel(c *gc.C) {
	foundErr := fmt.Errorf("did not get an error")
	triggeredHandler := func(errer watcher.Errer) error {
		foundErr = errer.Err()
		return foundErr
	}
	worker.SetMustErr(triggeredHandler)
	s.actor.watcher.SetStopError(tomb.ErrStillAlive)
	s.actor.watcher.Stop()
	err := waitShort(c, s.worker)
	c.Check(foundErr, gc.Equals, tomb.ErrStillAlive)
	// ErrStillAlive is trapped by the Stop logic and gets turned into a
	// 'nil' when stopping. However TestDefaultClosedHandler can assert
	// that it would have triggered a panic.
	c.Check(err, gc.IsNil)
	s.actor.CheckActions(c, "setup", "teardown")
	// Worker is stopped, don't fail TearDownTest
	s.worker = nil
}

func (s *stringsWorkerSuite) TestErrorsOnClosedChannel(c *gc.C) {
	foundErr := fmt.Errorf("did not get an error")
	triggeredHandler := func(errer watcher.Errer) error {
		foundErr = errer.Err()
		return foundErr
	}
	worker.SetMustErr(triggeredHandler)
	s.actor.watcher.Stop()
	err := waitShort(c, s.worker)
	// If the foundErr is nil, we would have panic-ed (see TestDefaultClosedHandler)
	c.Check(foundErr, gc.IsNil)
	c.Check(err, gc.IsNil)
	s.actor.CheckActions(c, "setup", "teardown")
}
