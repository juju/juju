// Copyright 2012-2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package watcher_test

import (
	"sync"
	"time"

	"github.com/juju/errors"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
	"launchpad.net/tomb"

	coretesting "github.com/juju/juju/testing"
	"github.com/juju/juju/watcher"
	"github.com/juju/juju/worker"
)

type notifyWorkerSuite struct {
	coretesting.BaseSuite
	worker worker.Worker
	actor  *notifyHandler
}

var _ = gc.Suite(&notifyWorkerSuite{})

func newNotifyHandlerWorker(c *gc.C, setupError, handlerError, teardownError error) (*notifyHandler, worker.Worker) {
	nh := &notifyHandler{
		actions:       nil,
		handled:       make(chan struct{}, 1),
		setupError:    setupError,
		teardownError: teardownError,
		handlerError:  handlerError,
		watcher:       newTestNotifyWatcher(),
		setupDone:     make(chan struct{}),
	}
	w, err := watcher.NewNotifyWorker(watcher.NotifyConfig{Handler: nh})
	c.Assert(err, jc.ErrorIsNil)
	select {
	case <-nh.setupDone:
	case <-time.After(coretesting.ShortWait):
		c.Error("Failed waiting for notifyHandler.Setup to be called during SetUpTest")
	}
	return nh, w
}

func (s *notifyWorkerSuite) SetUpTest(c *gc.C) {
	s.BaseSuite.SetUpTest(c)
	s.actor, s.worker = newNotifyHandlerWorker(c, nil, nil, nil)
}

func (s *notifyWorkerSuite) TearDownTest(c *gc.C) {
	s.stopWorker(c)
	s.BaseSuite.TearDownTest(c)
}

type notifyHandler struct {
	actions []string
	mu      sync.Mutex
	// Signal handled when we get a handle() call
	handled       chan struct{}
	setupError    error
	teardownError error
	handlerError  error
	watcher       *testNotifyWatcher
	setupDone     chan struct{}
}

func (nh *notifyHandler) SetUp() (watcher.NotifyWatcher, error) {
	defer func() { nh.setupDone <- struct{}{} }()
	nh.mu.Lock()
	defer nh.mu.Unlock()
	nh.actions = append(nh.actions, "setup")
	if nh.watcher == nil {
		return nil, nh.setupError
	}
	return nh.watcher, nh.setupError
}

func (nh *notifyHandler) TearDown() error {
	nh.mu.Lock()
	defer nh.mu.Unlock()
	nh.actions = append(nh.actions, "teardown")
	if nh.handled != nil {
		close(nh.handled)
	}
	return nh.teardownError
}

func (nh *notifyHandler) Handle(_ <-chan struct{}) error {
	nh.mu.Lock()
	defer nh.mu.Unlock()
	nh.actions = append(nh.actions, "handler")
	if nh.handled != nil {
		// Unlock while we are waiting for the send
		nh.mu.Unlock()
		nh.handled <- struct{}{}
		nh.mu.Lock()
	}
	return nh.handlerError
}

func (nh *notifyHandler) CheckActions(c *gc.C, actions ...string) {
	nh.mu.Lock()
	defer nh.mu.Unlock()
	c.Check(nh.actions, gc.DeepEquals, actions)
}

// During teardown we try to stop the worker, but don't hang the test suite if
// Stop never returns
func (s *notifyWorkerSuite) stopWorker(c *gc.C) {
	if s.worker == nil {
		return
	}
	done := make(chan error)
	go func() {
		done <- worker.Stop(s.worker)
	}()
	err := waitForTimeout(c, done, coretesting.LongWait)
	c.Check(err, jc.ErrorIsNil)
	s.actor = nil
	s.worker = nil
}

func newTestNotifyWatcher() *testNotifyWatcher {
	w := &testNotifyWatcher{
		changes: make(chan struct{}),
	}
	go func() {
		defer w.tomb.Done()
		<-w.tomb.Dying()
	}()
	return w
}

type testNotifyWatcher struct {
	tomb      tomb.Tomb
	changes   chan struct{}
	mu        sync.Mutex
	stopError error
}

func (tnw *testNotifyWatcher) Changes() watcher.NotifyChannel {
	return tnw.changes
}

func (tnw *testNotifyWatcher) Kill() {
	tnw.mu.Lock()
	tnw.tomb.Kill(tnw.stopError)
	tnw.mu.Unlock()
}

func (tnw *testNotifyWatcher) Wait() error {
	return tnw.tomb.Wait()
}

func (tnw *testNotifyWatcher) Stopped() bool {
	select {
	case <-tnw.tomb.Dead():
		return true
	default:
		return false
	}
}

func (tnw *testNotifyWatcher) SetStopError(err error) {
	tnw.mu.Lock()
	tnw.stopError = err
	tnw.mu.Unlock()
}

func (tnw *testNotifyWatcher) TriggerChange(c *gc.C) {
	select {
	case tnw.changes <- struct{}{}:
	case <-time.After(coretesting.LongWait):
		c.Errorf("timed out trying to trigger a change")
	}
}

func waitForTimeout(c *gc.C, ch <-chan error, timeout time.Duration) error {
	select {
	case err := <-ch:
		return err
	case <-time.After(timeout):
		c.Errorf("timed out waiting to receive a change after %s", timeout)
	}
	return nil
}

func waitShort(c *gc.C, w worker.Worker) error {
	done := make(chan error)
	go func() {
		done <- w.Wait()
	}()
	return waitForTimeout(c, done, coretesting.ShortWait)
}

func waitForHandledNotify(c *gc.C, handled chan struct{}) {
	select {
	case <-handled:
	case <-time.After(coretesting.LongWait):
		c.Errorf("handled failed to signal after %s", coretesting.LongWait)
	}
}

func (s *notifyWorkerSuite) TestKill(c *gc.C) {
	s.worker.Kill()
	err := waitShort(c, s.worker)
	c.Assert(err, jc.ErrorIsNil)
}

func (s *notifyWorkerSuite) TestStop(c *gc.C) {
	err := worker.Stop(s.worker)
	c.Assert(err, jc.ErrorIsNil)
	// After stop, Wait should return right away
	err = waitShort(c, s.worker)
	c.Assert(err, jc.ErrorIsNil)
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
	c.Assert(err, jc.ErrorIsNil)
}

func (s *notifyWorkerSuite) TestCallSetUpAndTearDown(c *gc.C) {
	// After calling NewNotifyWorker, we should have called setup
	s.actor.CheckActions(c, "setup")
	// If we kill the worker, it should notice, and call teardown
	s.worker.Kill()
	err := waitShort(c, s.worker)
	c.Check(err, jc.ErrorIsNil)
	s.actor.CheckActions(c, "setup", "teardown")
	c.Check(s.actor.watcher.Stopped(), jc.IsTrue)
}

func (s *notifyWorkerSuite) TestChangesTriggerHandler(c *gc.C) {
	s.actor.CheckActions(c, "setup")
	s.actor.watcher.TriggerChange(c)
	waitForHandledNotify(c, s.actor.handled)
	s.actor.CheckActions(c, "setup", "handler")
	s.actor.watcher.TriggerChange(c)
	waitForHandledNotify(c, s.actor.handled)
	s.actor.watcher.TriggerChange(c)
	waitForHandledNotify(c, s.actor.handled)
	s.actor.CheckActions(c, "setup", "handler", "handler", "handler")
	c.Assert(worker.Stop(s.worker), gc.IsNil)
	s.actor.CheckActions(c, "setup", "handler", "handler", "handler", "teardown")
}

func (s *notifyWorkerSuite) TestSetUpFailureStopsWithTearDown(c *gc.C) {
	// Stop the worker and SetUp again, this time with an error
	s.stopWorker(c)
	actor, w := newNotifyHandlerWorker(c, errors.New("my special error"), nil, errors.New("teardown"))
	err := waitShort(c, w)
	c.Check(err, gc.ErrorMatches, "my special error")
	actor.CheckActions(c, "setup", "teardown")
	c.Check(actor.watcher.Stopped(), jc.IsTrue)
}

func (s *notifyWorkerSuite) TestWatcherStopFailurePropagates(c *gc.C) {
	s.actor.watcher.SetStopError(errors.New("error while stopping watcher"))
	s.worker.Kill()
	c.Assert(s.worker.Wait(), gc.ErrorMatches, "error while stopping watcher")
	// We've already stopped the worker, don't let teardown notice the
	// worker is in an error state
	s.worker = nil
}

func (s *notifyWorkerSuite) TestCleanRunNoticesTearDownError(c *gc.C) {
	s.actor.teardownError = errors.New("failed to tear down watcher")
	s.worker.Kill()
	c.Assert(s.worker.Wait(), gc.ErrorMatches, "failed to tear down watcher")
	s.worker = nil
}

func (s *notifyWorkerSuite) TestHandleErrorStopsWorkerAndWatcher(c *gc.C) {
	s.stopWorker(c)
	actor, w := newNotifyHandlerWorker(c, nil, errors.New("my handling error"), nil)
	actor.watcher.TriggerChange(c)
	waitForHandledNotify(c, actor.handled)
	err := waitShort(c, w)
	c.Check(err, gc.ErrorMatches, "my handling error")
	actor.CheckActions(c, "setup", "handler", "teardown")
	c.Check(actor.watcher.Stopped(), jc.IsTrue)
}

func (s *notifyWorkerSuite) TestNoticesStoppedWatcher(c *gc.C) {
	s.actor.watcher.SetStopError(errors.New("Stopped Watcher"))
	s.actor.watcher.Kill()
	err := waitShort(c, s.worker)
	c.Check(err, gc.ErrorMatches, "Stopped Watcher")
	s.actor.CheckActions(c, "setup", "teardown")
	s.worker = nil
}

func (s *notifyWorkerSuite) TestErrorsOnClosedChannel(c *gc.C) {
	close(s.actor.watcher.changes)
	err := waitShort(c, s.worker)
	c.Check(err, gc.ErrorMatches, "change channel closed")
	s.actor.CheckActions(c, "setup", "teardown")
	s.worker = nil
}
