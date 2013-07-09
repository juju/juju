// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package worker_test

import (
	"fmt"
	"sync"
	"time"

	gc "launchpad.net/gocheck"
	"launchpad.net/tomb"

	"launchpad.net/juju-core/state/api/params"
	"launchpad.net/juju-core/state/watcher"
	coretesting "launchpad.net/juju-core/testing"
	jc "launchpad.net/juju-core/testing/checkers"
	"launchpad.net/juju-core/worker"
)

type notifyWorkerSuite struct {
	coretesting.LoggingSuite
	worker worker.NotifyWorker
	actor  *actionsHandler
}

var _ = gc.Suite(&notifyWorkerSuite{})

func (s *notifyWorkerSuite) SetUpTest(c *gc.C) {
	s.LoggingSuite.SetUpTest(c)
	s.actor = &actionsHandler{
		actions:     nil,
		handled:     make(chan struct{}, 1),
		description: "test action handler",
		watcher: &TestWatcher{
			changes: make(chan struct{}),
		},
	}
	s.worker = worker.NewNotifyWorker(s.actor)
}

func (s *notifyWorkerSuite) TearDownTest(c *gc.C) {
	s.stopWorker(c)
	s.LoggingSuite.TearDownTest(c)
}

type actionsHandler struct {
	actions []string
	mu      sync.Mutex
	// Signal handled when we get a handle() call
	handled       chan struct{}
	setupError    error
	teardownError error
	handlerError  error
	watcher       *TestWatcher
	description   string
}

func (a *actionsHandler) SetUp() (params.NotifyWatcher, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.actions = append(a.actions, "setup")
	if a.watcher == nil {
		return nil, a.setupError
	}
	return a.watcher, a.setupError
}

func (a *actionsHandler) TearDown() error {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.actions = append(a.actions, "teardown")
	if a.handled != nil {
		close(a.handled)
	}
	return a.teardownError
}

func (a *actionsHandler) Handle() error {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.actions = append(a.actions, "handler")
	if a.handled != nil {
		// Unlock while we are waiting for the send
		a.mu.Unlock()
		a.handled <- struct{}{}
		a.mu.Lock()
	}
	return a.handlerError
}

func (a *actionsHandler) String() string {
	return a.description
}

func (a *actionsHandler) CheckActions(c *gc.C, actions ...string) {
	a.mu.Lock()
	defer a.mu.Unlock()
	c.Check(a.actions, gc.DeepEquals, actions)
}

// During teardown we try to stop the worker, but don't hang the test suite if
// Stop never returns
func (s *notifyWorkerSuite) stopWorker(c *gc.C) {
	if s.worker == nil {
		return
	}
	done := make(chan error)
	go func() {
		done <- s.worker.Stop()
	}()
	err := waitForTimeout(c, done, coretesting.LongWait)
	c.Check(err, gc.IsNil)
	s.actor = nil
	s.worker = nil
}

type TestWatcher struct {
	mu        sync.Mutex
	changes   chan struct{}
	action    chan struct{}
	stopped   bool
	stopError error
}

func (tw *TestWatcher) Changes() <-chan struct{} {
	return tw.changes
}

func (tw *TestWatcher) Err() error {
	return tw.stopError
}

func (tw *TestWatcher) Stop() error {
	tw.mu.Lock()
	defer tw.mu.Unlock()
	if !tw.stopped {
		close(tw.changes)
	}
	tw.stopped = true
	return tw.stopError
}

func (tw *TestWatcher) SetStopError(err error) {
	tw.mu.Lock()
	tw.stopError = err
	tw.mu.Unlock()
}

func (tw *TestWatcher) TriggerChange(c *gc.C) {
	select {
	case tw.changes <- struct{}{}:
	case <-time.After(coretesting.LongWait):
		c.Errorf("Timeout changes triggering change after %s", coretesting.LongWait)
	}
}

func waitForTimeout(c *gc.C, ch <-chan error, timeout time.Duration) error {
	select {
	case err := <-ch:
		return err
	case <-time.After(timeout):
		c.Errorf("failed to receive value after %s", timeout)
	}
	return nil
}

func WaitShort(c *gc.C, w worker.NotifyWorker) error {
	done := make(chan error)
	go func() {
		done <- w.Wait()
	}()
	return waitForTimeout(c, done, coretesting.ShortWait)
}

func WaitForHandled(c *gc.C, handled chan struct{}) {
	select {
	case <-handled:
		return
	case <-time.After(coretesting.LongWait):
		c.Errorf("handled failed to signal after %s", coretesting.LongWait)
	}
}

func (s *notifyWorkerSuite) TestKill(c *gc.C) {
	s.worker.Kill()
	err := WaitShort(c, s.worker)
	c.Assert(err, gc.IsNil)
}

func (s *notifyWorkerSuite) TestStop(c *gc.C) {
	err := s.worker.Stop()
	c.Assert(err, gc.IsNil)
	// After stop, Wait should return right away
	err = WaitShort(c, s.worker)
	c.Assert(err, gc.IsNil)
}

func (s *notifyWorkerSuite) TestWait(c *gc.C) {
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

func (s *notifyWorkerSuite) TestStringForwardsHandlerString(c *gc.C) {
	c.Check(fmt.Sprint(s.worker), gc.Equals, "test action handler")
}

func (s *notifyWorkerSuite) TestCallSetUpAndTearDown(c *gc.C) {
	// After calling NewNotifyWorker, we should have called setup
	s.actor.CheckActions(c, "setup")
	// If we kill the worker, it should notice, and call teardown
	s.worker.Kill()
	err := WaitShort(c, s.worker)
	c.Check(err, gc.IsNil)
	s.actor.CheckActions(c, "setup", "teardown")
	c.Check(s.actor.watcher.stopped, jc.IsTrue)
}

func (s *notifyWorkerSuite) TestChangesTriggerHandler(c *gc.C) {
	s.actor.CheckActions(c, "setup")
	s.actor.watcher.TriggerChange(c)
	WaitForHandled(c, s.actor.handled)
	s.actor.CheckActions(c, "setup", "handler")
	s.actor.watcher.TriggerChange(c)
	WaitForHandled(c, s.actor.handled)
	s.actor.watcher.TriggerChange(c)
	WaitForHandled(c, s.actor.handled)
	s.actor.CheckActions(c, "setup", "handler", "handler", "handler")
	c.Assert(s.worker.Stop(), gc.IsNil)
	s.actor.CheckActions(c, "setup", "handler", "handler", "handler", "teardown")
}

func (s *notifyWorkerSuite) TestSetUpFailureStopsWithTearDown(c *gc.C) {
	// Stop the worker and SetUp again, this time with an error
	s.stopWorker(c)
	actor := &actionsHandler{
		actions:    nil,
		handled:    make(chan struct{}, 1),
		setupError: fmt.Errorf("my special error"),
		watcher: &TestWatcher{
			changes: make(chan struct{}),
		},
	}
	w := worker.NewNotifyWorker(actor)
	err := WaitShort(c, w)
	c.Check(err, gc.ErrorMatches, "my special error")
	actor.CheckActions(c, "setup", "teardown")
	c.Check(actor.watcher.stopped, jc.IsTrue)
}

func (s *notifyWorkerSuite) TestWatcherStopFailurePropagates(c *gc.C) {
	s.actor.watcher.SetStopError(fmt.Errorf("error while stopping watcher"))
	s.worker.Kill()
	c.Assert(s.worker.Wait(), gc.ErrorMatches, "error while stopping watcher")
	// We've already stopped the worker, don't let teardown notice the
	// worker is in an error state
	s.worker = nil
}

func (s *notifyWorkerSuite) TestCleanRunNoticesTearDownError(c *gc.C) {
	s.actor.teardownError = fmt.Errorf("failed to tear down watcher")
	s.worker.Kill()
	c.Assert(s.worker.Wait(), gc.ErrorMatches, "failed to tear down watcher")
	s.worker = nil
}

func (s *notifyWorkerSuite) TestHandleErrorStopsWorkerAndWatcher(c *gc.C) {
	s.stopWorker(c)
	actor := &actionsHandler{
		actions:      nil,
		handled:      make(chan struct{}, 1),
		handlerError: fmt.Errorf("my handling error"),
		watcher: &TestWatcher{
			changes: make(chan struct{}),
		},
	}
	w := worker.NewNotifyWorker(actor)
	actor.watcher.TriggerChange(c)
	WaitForHandled(c, actor.handled)
	err := WaitShort(c, w)
	c.Check(err, gc.ErrorMatches, "my handling error")
	actor.CheckActions(c, "setup", "handler", "teardown")
	c.Check(actor.watcher.stopped, jc.IsTrue)
}

func (s *notifyWorkerSuite) TestNoticesStoppedWatcher(c *gc.C) {
	// The default closedHandler doesn't panic if you have a genuine error
	// (because it assumes you want to propagate a real error and then
	// restart
	s.actor.watcher.SetStopError(fmt.Errorf("Stopped Watcher"))
	s.actor.watcher.Stop()
	err := WaitShort(c, s.worker)
	c.Check(err, gc.ErrorMatches, "Stopped Watcher")
	s.actor.CheckActions(c, "setup", "teardown")
	// Worker is stopped, don't fail TearDownTest
	s.worker = nil
}

func noopHandler(watcher.Errer) error {
	return nil
}

type CannedErrer struct {
	err error
}

func (c CannedErrer) Err() error {
	return c.err
}

type closerHandler interface {
	SetClosedHandler(func(watcher.Errer) error) func(watcher.Errer) error
}

func (s *notifyWorkerSuite) TestDefaultClosedHandler(c *gc.C) {
	h, ok := s.worker.(closerHandler)
	c.Assert(ok, jc.IsTrue)
	old := h.SetClosedHandler(noopHandler)
	noErr := CannedErrer{nil}
	stillAlive := CannedErrer{tomb.ErrStillAlive}
	customErr := CannedErrer{fmt.Errorf("my special error")}

	// The default handler should be watcher.MustErr which panics if the
	// Errer doesn't actually have an error
	c.Assert(func() { old(noErr) }, gc.PanicMatches, "watcher was stopped cleanly")
	c.Assert(func() { old(stillAlive) }, gc.PanicMatches, "watcher is still running")
	c.Assert(old(customErr), gc.Equals, customErr.Err())
}

func (s *notifyWorkerSuite) TestErrorsOnStillAliveButClosedChannel(c *gc.C) {
	foundErr := fmt.Errorf("did not get an error")
	triggeredHandler := func(errer watcher.Errer) error {
		foundErr = errer.Err()
		return foundErr
	}
	s.worker.(closerHandler).SetClosedHandler(triggeredHandler)
	s.actor.watcher.SetStopError(tomb.ErrStillAlive)
	s.actor.watcher.Stop()
	err := WaitShort(c, s.worker)
	c.Check(foundErr, gc.Equals, tomb.ErrStillAlive)
	// ErrStillAlive is trapped by the Stop logic and gets turned into a
	// 'nil' when stopping. However TestDefaultClosedHandler can assert
	// that it would have triggered a panic.
	c.Check(err, gc.IsNil)
	s.actor.CheckActions(c, "setup", "teardown")
	// Worker is stopped, don't fail TearDownTest
	s.worker = nil
}

func (s *notifyWorkerSuite) TestErrorsOnClosedChannel(c *gc.C) {
	foundErr := fmt.Errorf("did not get an error")
	triggeredHandler := func(errer watcher.Errer) error {
		foundErr = errer.Err()
		return foundErr
	}
	s.worker.(closerHandler).SetClosedHandler(triggeredHandler)
	s.actor.watcher.Stop()
	err := WaitShort(c, s.worker)
	// If the foundErr is nil, we would have panic-ed (see TestDefaultClosedHandler)
	c.Check(foundErr, gc.IsNil)
	c.Check(err, gc.IsNil)
	s.actor.CheckActions(c, "setup", "teardown")
}
