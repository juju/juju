// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package restorewatcher_test

import (
	"time"

	"github.com/juju/errors"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/state"
	coretesting "github.com/juju/juju/testing"
	"github.com/juju/juju/watcher/watchertest"
	"github.com/juju/juju/worker/restorewatcher"
	"github.com/juju/juju/worker/workertest"
)

type WorkerSuite struct {
	testing.IsolationSuite
	watcher *mockRestoreInfoWatcher
	stub    testing.Stub
	changed chan state.RestoreStatus
	config  restorewatcher.Config
}

var _ = gc.Suite(&WorkerSuite{})

func (s *WorkerSuite) SetUpTest(c *gc.C) {
	s.IsolationSuite.SetUpTest(c)
	s.watcher = &mockRestoreInfoWatcher{
		changes: make(chan struct{}),
	}
	s.changed = make(chan state.RestoreStatus, 1)
	s.stub.ResetCalls()
	s.config = restorewatcher.Config{
		RestoreInfoWatcher:   s.watcher,
		RestoreStatusChanged: s.restoreStatusChanged,
	}
}

func (s *WorkerSuite) restoreStatusChanged(status state.RestoreStatus) error {
	s.stub.MethodCall(s, "RestoreStatusChanged", status)
	if err := s.stub.NextErr(); err != nil {
		return err
	}
	s.changed <- status
	return nil
}

func (s *WorkerSuite) TestValidateRestoreInfoWatcher(c *gc.C) {
	s.config.RestoreInfoWatcher = nil
	_, err := restorewatcher.NewWorker(s.config)
	c.Assert(err, gc.ErrorMatches, "nil RestoreInfoWatcher not valid")
}

func (s *WorkerSuite) TestValidateRestoreStatusChanged(c *gc.C) {
	s.config.RestoreStatusChanged = nil
	_, err := restorewatcher.NewWorker(s.config)
	c.Assert(err, gc.ErrorMatches, "nil RestoreStatusChanged not valid")
}

func (s *WorkerSuite) TestStartStop(c *gc.C) {
	w, err := restorewatcher.NewWorker(s.config)
	c.Assert(err, jc.ErrorIsNil)
	workertest.CleanKill(c, w)
}

func (s *WorkerSuite) TestWorkerReportsChanges(c *gc.C) {
	w, err := restorewatcher.NewWorker(s.config)
	c.Assert(err, jc.ErrorIsNil)
	defer workertest.CleanKill(c, w)

	select {
	case <-s.changed:
		// We don't send an initial event in the test,
		// so we don't expect a change yet.
		c.Fatal("unexpected change")
	case <-time.After(coretesting.ShortWait):
	}

	s.watcher.status = state.RestoreFailed
	select {
	case s.watcher.changes <- struct{}{}:
	case <-time.After(coretesting.LongWait):
		c.Fatal("timed out sending watcher change")
	}

	select {
	case status, ok := <-s.changed:
		c.Assert(ok, jc.IsTrue)
		c.Assert(status, gc.Equals, state.RestoreFailed)
	case <-time.After(coretesting.LongWait):
		c.Fatal("timed out waiting for status to be reported")
	}
}

func (s *WorkerSuite) TestErrorTerminatesWorker(c *gc.C) {
	s.watcher.SetErrors(errors.New("burp"))

	w, err := restorewatcher.NewWorker(s.config)
	c.Assert(err, jc.ErrorIsNil)
	defer workertest.DirtyKill(c, w)

	s.watcher.status = state.RestoreFailed
	select {
	case s.watcher.changes <- struct{}{}:
	case <-time.After(coretesting.LongWait):
		c.Fatal("timed out sending watcher change")
	}

	err = workertest.CheckKilled(c, w)
	c.Assert(err, gc.ErrorMatches, "burp")
}

type mockRestoreInfoWatcher struct {
	testing.Stub
	changes chan struct{}
	status  state.RestoreStatus
}

func (w *mockRestoreInfoWatcher) WatchRestoreInfoChanges() state.NotifyWatcher {
	w.MethodCall(w, "WatchRestoreInfoChanges")
	return watchertest.NewMockNotifyWatcher(w.changes)
}

func (w *mockRestoreInfoWatcher) RestoreStatus() (state.RestoreStatus, error) {
	w.MethodCall(w, "RestoreStatus")
	return w.status, w.NextErr()
}
