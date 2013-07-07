// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package worker_test

import (
	"fmt"
	"sync"
	"time"

	gc "launchpad.net/gocheck"

	coretesting "launchpad.net/juju-core/testing"
	jc "launchpad.net/juju-core/testing/checkers"
	"launchpad.net/juju-core/worker"
)

var shortWait = 5 * time.Millisecond
var longWait = 500 * time.Millisecond

type notifyWorkerSuite struct {
	coretesting.LoggingSuite
	worker  worker.NotifyWorker
	watcher *TestWatcher
	actor   *ActionsWorker
}

var _ = gc.Suite(&notifyWorkerSuite{})

func (s *notifyWorkerSuite) SetUpTest(c *gc.C) {
	s.LoggingSuite.SetUpTest(c)
	s.actor = &ActionsWorker{
		actions: nil,
		handled: make(chan struct{}),
	}
	s.watcher, s.worker = initializeWorker(c, s.actor)
}

func (s *notifyWorkerSuite) TearDownTest(c *gc.C) {
	s.stopWorker(c)
	s.LoggingSuite.TearDownTest(c)
}

type ActionsWorker struct {
	actions []string
	mu      sync.Mutex
	// Signal handled when we get a handle() call
	handled      chan struct{}
	setupError   error
	handlerError error
}

func (a *ActionsWorker) getSetup() func() error {
	return func() error {
		a.mu.Lock()
		defer a.mu.Unlock()
		a.actions = append(a.actions, "setup")
		return a.setupError
	}
}

func (a *ActionsWorker) getTeardown() func() {
	return func() {
		a.mu.Lock()
		defer a.mu.Unlock()
		a.actions = append(a.actions, "teardown")
	}
}

func (a *ActionsWorker) getHandler() func() error {
	return func() error {
		a.mu.Lock()
		defer a.mu.Unlock()
		a.actions = append(a.actions, "handler")
		if a.handled != nil {
			a.handled <- struct{}{}
		}
		return a.handlerError
	}
}

func (a *ActionsWorker) CheckActions(c *gc.C, actions ...string) {
	a.mu.Lock()
	defer a.mu.Unlock()
	c.Check(a.actions, gc.DeepEquals, actions)
}

func initializeWorker(c *gc.C, actor *ActionsWorker) (*TestWatcher, worker.NotifyWorker) {
	watcher := &TestWatcher{
		c:   c,
		out: make(chan struct{}),
	}
	w := worker.NewNotifyWorker(
		watcher,
		actor.getSetup(),
		actor.getHandler(),
		actor.getTeardown(),
	)
	return watcher, w
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
	select {
	case err := <-done:
		c.Check(err, gc.IsNil)
	case <-time.After(longWait):
		c.Errorf("Failed to stop worker after %.3fs", longWait.Seconds())
	}
	s.watcher = nil
	s.worker = nil
}

type TestWatcher struct {
	mu        sync.Mutex
	out       chan struct{}
	action    chan struct{}
	c         *gc.C
	stopped   bool
	stopError error
}

func (tw *TestWatcher) Changes() <-chan struct{} {
	return tw.out
}

func (tw *TestWatcher) Err() error {
	return nil
}

func (tw *TestWatcher) Stop() error {
	tw.mu.Lock()
	defer tw.mu.Unlock()
	tw.stopped = true
	return tw.stopError
}

func (tw *TestWatcher) SetStopError(err error) {
	tw.mu.Lock()
	tw.stopError = err
	tw.mu.Unlock()
}

func (tw *TestWatcher) TriggerChange() {
	select {
	case tw.out <- struct{}{}:
	case <-time.After(longWait):
		tw.c.Errorf("Timed out triggering change after %.3fs", longWait.Seconds())
	}
}

func WaitShort(c *gc.C, w worker.NotifyWorker) error {
	done := make(chan error)
	go func() {
		done <- w.Wait()
	}()
	select {
	case err := <-done:
		return err
	case <-time.After(shortWait):
		c.Errorf("Wait() failed to return after %.3fs", shortWait.Seconds())
	}
	return nil
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
	select {
	case err := <-done:
		c.Errorf("Wait() didn't wait until we stopped it. err: %v", err)
	case <-time.After(shortWait):
	}
	s.worker.Kill()
	select {
	case err := <-done:
		c.Assert(err, gc.IsNil)
	case <-time.After(longWait):
		c.Errorf("Wait() failed to return after we stopped.")
	}
}

func (s *notifyWorkerSuite) TestCallSetupAndTeardown(c *gc.C) {
	// After calling NewNotifyWorker, we should have called setup
	s.actor.CheckActions(c, "setup")
	// If we kill the worker, it should notice, and call teardown
	s.worker.Kill()
	err := s.worker.Wait()
	s.actor.CheckActions(c, "setup", "teardown")
	c.Check(err, gc.IsNil)
}

func (s *notifyWorkerSuite) TestChangesTriggerHandler(c *gc.C) {
	s.actor.CheckActions(c, "setup")
	s.watcher.TriggerChange()
	<-s.actor.handled
	s.actor.CheckActions(c, "setup", "handler")
	s.watcher.TriggerChange()
	<-s.actor.handled
	s.watcher.TriggerChange()
	<-s.actor.handled
	s.actor.CheckActions(c, "setup", "handler", "handler", "handler")
	c.Assert(s.worker.Stop(), gc.IsNil)
	s.actor.CheckActions(c, "setup", "handler", "handler", "handler", "teardown")
}

func (s *notifyWorkerSuite) TestSetupFailureStopsWithTeardown(c *gc.C) {
	// Setup again, this time with an error
	s.stopWorker(c)
	actor := &ActionsWorker{
		actions:    nil,
		handled:    make(chan struct{}),
		setupError: fmt.Errorf("my special error"),
	}
	watcher, w := initializeWorker(c, actor)
	err := WaitShort(c, w)
	c.Check(err, gc.ErrorMatches, "my special error")
	actor.CheckActions(c, "setup", "teardown")
	c.Check(watcher.stopped, jc.IsTrue)
}

func (s *notifyWorkerSuite) TestWatcherStopFailurePropagates(c *gc.C) {
	s.watcher.SetStopError(fmt.Errorf("error while stopping watcher"))
	s.worker.Kill()
	c.Assert(s.worker.Wait(), gc.ErrorMatches, "error while stopping watcher")
	// We've already stopped the worker, don't let teardown notice the
	// worker is in an error state
	s.worker = nil
}

func (s *notifyWorkerSuite) TestHandleErrorStopsWorkerAndWatcher(c *gc.C) {
	s.stopWorker(c)
	actor := &ActionsWorker{
		actions:      nil,
		handled:      make(chan struct{}),
		handlerError: fmt.Errorf("my handling error"),
	}
	watcher, w := initializeWorker(c, actor)
	watcher.TriggerChange()
	<-actor.handled
	err := WaitShort(c, w)
	c.Check(err, gc.ErrorMatches, "my handling error")
	actor.CheckActions(c, "setup", "handler", "teardown")
	c.Check(watcher.stopped, jc.IsTrue)
}
