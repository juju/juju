// Copyright 2013-2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package watcher_test

import (
	"context"
	"sync"
	stdtesting "testing"
	"time"

	"github.com/juju/tc"
	"github.com/juju/worker/v4"
	"gopkg.in/tomb.v2"

	"github.com/juju/juju/core/watcher"
	"github.com/juju/juju/internal/errors"
	coretesting "github.com/juju/juju/internal/testing"
)

type stringsWorkerSuite struct {
	coretesting.BaseSuite
	worker worker.Worker
	actor  *stringsHandler
}

func TestStringsWorkerSuite(t *stdtesting.T) {
	tc.Run(t, &stringsWorkerSuite{})
}

func newStringsHandlerWorker(c *tc.C, setupError, handlerError, teardownError error) (*stringsHandler, worker.Worker) {
	sh := &stringsHandler{
		actions:       nil,
		handled:       make(chan []string, 1),
		setupError:    setupError,
		teardownError: teardownError,
		handlerError:  handlerError,
		watcher:       newTestStringsWatcher(),
		setupDone:     make(chan struct{}),
	}
	w, err := watcher.NewStringsWorker(watcher.StringsConfig{Handler: sh})
	c.Assert(err, tc.ErrorIsNil)
	select {
	case <-sh.setupDone:
	case <-time.After(coretesting.ShortWait):
		c.Error("Failed waiting for stringsHandler.Setup to be called during SetUpTest")
	}
	return sh, w
}

func (s *stringsWorkerSuite) SetUpTest(c *tc.C) {
	s.BaseSuite.SetUpTest(c)
	s.actor, s.worker = newStringsHandlerWorker(c, nil, nil, nil)
	s.AddCleanup(s.stopWorker)
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
	setupDone     chan struct{}
}

func (sh *stringsHandler) SetUp(ctx context.Context) (watcher.StringsWatcher, error) {
	defer func() { sh.setupDone <- struct{}{} }()
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

func (sh *stringsHandler) Handle(ctx context.Context, changes []string) error {
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

func (sh *stringsHandler) CheckActions(c *tc.C, actions ...string) {
	sh.mu.Lock()
	defer sh.mu.Unlock()
	c.Check(sh.actions, tc.DeepEquals, actions)
}

// During teardown we try to stop the worker, but don't hang the test suite if
// Stop never returns
func (s *stringsWorkerSuite) stopWorker(c *tc.C) {
	if s.worker == nil {
		return
	}
	done := make(chan error)
	go func() {
		done <- worker.Stop(s.worker)
	}()
	err := waitForTimeout(c, done, coretesting.LongWait)
	c.Check(err, tc.ErrorIsNil)
	s.actor = nil
	s.worker = nil
}

func newTestStringsWatcher() *testStringsWatcher {
	w := &testStringsWatcher{
		changes: make(chan []string),
	}
	w.tomb.Go(func() error {
		<-w.tomb.Dying()
		return nil
	})
	return w
}

type testStringsWatcher struct {
	tomb      tomb.Tomb
	changes   chan []string
	mu        sync.Mutex
	stopError error
}

func (tsw *testStringsWatcher) Changes() <-chan []string {
	return tsw.changes
}

func (tsw *testStringsWatcher) Kill() {
	tsw.mu.Lock()
	tsw.tomb.Kill(tsw.stopError)
	tsw.mu.Unlock()
}

func (tsw *testStringsWatcher) Wait() error {
	return tsw.tomb.Wait()
}

func (tsw *testStringsWatcher) Stopped() bool {
	select {
	case <-tsw.tomb.Dead():
		return true
	default:
		return false
	}
}

func (tsw *testStringsWatcher) SetStopError(err error) {
	tsw.mu.Lock()
	tsw.stopError = err
	tsw.mu.Unlock()
}

func (tsw *testStringsWatcher) TriggerChange(c *tc.C, changes []string) {
	select {
	case tsw.changes <- changes:
	case <-time.After(coretesting.LongWait):
		c.Errorf("timed out trying to trigger a change")
	}
}

func waitForHandledStrings(c *tc.C, handled chan []string, expect []string) {
	select {
	case changes := <-handled:
		c.Assert(changes, tc.DeepEquals, expect)
	case <-time.After(coretesting.LongWait):
		c.Errorf("handled failed to signal after %s", coretesting.LongWait)
	}
}

func (s *stringsWorkerSuite) TestKill(c *tc.C) {
	s.worker.Kill()
	err := waitForWorkerStopped(c, s.worker)
	c.Assert(err, tc.ErrorIsNil)
}

func (s *stringsWorkerSuite) TestStop(c *tc.C) {
	err := worker.Stop(s.worker)
	c.Assert(err, tc.ErrorIsNil)
	// After stop, Wait should return right away
	err = waitForWorkerStopped(c, s.worker)
	c.Assert(err, tc.ErrorIsNil)
}

func (s *stringsWorkerSuite) TestWait(c *tc.C) {
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
	c.Assert(err, tc.ErrorIsNil)
}

func (s *stringsWorkerSuite) TestCallSetUpAndTearDown(c *tc.C) {
	// After calling NewStringsWorker, we should have called setup
	s.actor.CheckActions(c, "setup")
	// If we kill the worker, it should notice, and call teardown
	s.worker.Kill()
	err := waitForWorkerStopped(c, s.worker)
	c.Check(err, tc.ErrorIsNil)
	s.actor.CheckActions(c, "setup", "teardown")
	c.Check(s.actor.watcher.Stopped(), tc.IsTrue)
}

func (s *stringsWorkerSuite) TestChangesTriggerHandler(c *tc.C) {
	s.actor.CheckActions(c, "setup")
	s.actor.watcher.TriggerChange(c, []string{"aa", "bb"})
	waitForHandledStrings(c, s.actor.handled, []string{"aa", "bb"})
	s.actor.CheckActions(c, "setup", "handler")
	s.actor.watcher.TriggerChange(c, []string{"cc", "dd"})
	waitForHandledStrings(c, s.actor.handled, []string{"cc", "dd"})
	s.actor.watcher.TriggerChange(c, []string{"ee", "ff"})
	waitForHandledStrings(c, s.actor.handled, []string{"ee", "ff"})
	s.actor.CheckActions(c, "setup", "handler", "handler", "handler")
	c.Assert(worker.Stop(s.worker), tc.IsNil)
	s.actor.CheckActions(c, "setup", "handler", "handler", "handler", "teardown")
}

func (s *stringsWorkerSuite) TestSetUpFailureStopsWithTearDown(c *tc.C) {
	// Stop the worker and SetUp again, this time with an error
	s.stopWorker(c)
	actor, w := newStringsHandlerWorker(c, errors.New("my special error"), nil, nil)
	err := waitForWorkerStopped(c, w)
	c.Check(err, tc.ErrorMatches, "my special error")
	actor.CheckActions(c, "setup", "teardown")
	c.Check(actor.watcher.Stopped(), tc.IsTrue)
}

func (s *stringsWorkerSuite) TestWatcherStopFailurePropagates(c *tc.C) {
	s.actor.watcher.SetStopError(errors.New("error while stopping watcher"))
	s.worker.Kill()
	c.Assert(s.worker.Wait(), tc.ErrorMatches, "error while stopping watcher")
	// We've already stopped the worker, don't let teardown notice the
	// worker is in an error state
	s.worker = nil
}

func (s *stringsWorkerSuite) TestCleanRunNoticesTearDownError(c *tc.C) {
	s.actor.teardownError = errors.New("failed to tear down watcher")
	s.worker.Kill()
	c.Assert(s.worker.Wait(), tc.ErrorMatches, "failed to tear down watcher")
	s.worker = nil
}

func (s *stringsWorkerSuite) TestHandleErrorStopsWorkerAndWatcher(c *tc.C) {
	s.stopWorker(c)
	actor, w := newStringsHandlerWorker(c, nil, errors.New("my handling error"), nil)
	actor.watcher.TriggerChange(c, []string{"aa", "bb"})
	waitForHandledStrings(c, actor.handled, []string{"aa", "bb"})
	err := waitForWorkerStopped(c, w)
	c.Check(err, tc.ErrorMatches, "my handling error")
	actor.CheckActions(c, "setup", "handler", "teardown")
	c.Check(actor.watcher.Stopped(), tc.IsTrue)
}

func (s *stringsWorkerSuite) TestNoticesStoppedWatcher(c *tc.C) {
	// The default closedHandler doesn't panic if you have a genuine error
	// (because it assumes you want to propagate a real error and then
	// restart
	s.actor.watcher.SetStopError(errors.New("Stopped Watcher"))
	s.actor.watcher.Kill()
	err := waitForWorkerStopped(c, s.worker)
	c.Check(err, tc.ErrorMatches, "Stopped Watcher")
	s.actor.CheckActions(c, "setup", "teardown")
	// Worker is stopped, don't fail TearDownTest
	s.worker = nil
}

func (s *stringsWorkerSuite) TestErrorsOnClosedChannel(c *tc.C) {
	close(s.actor.watcher.changes)
	err := waitForWorkerStopped(c, s.worker)
	c.Check(err, tc.ErrorMatches, "change channel closed")
	s.actor.CheckActions(c, "setup", "teardown")
	s.worker = nil
}
