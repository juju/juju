// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package restorewatcher_test

import (
	"time"

	"github.com/juju/errors"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/worker/v2/workertest"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/state"
	statetesting "github.com/juju/juju/state/testing"
	coretesting "github.com/juju/juju/testing"
	"github.com/juju/juju/worker/restorewatcher"
)

type WorkerSuite struct {
	testing.IsolationSuite
	watcher *mockRestoreInfoWatcher
	stub    testing.Stub
	config  restorewatcher.Config
}

var _ = gc.Suite(&WorkerSuite{})

func (s *WorkerSuite) SetUpTest(c *gc.C) {
	s.IsolationSuite.SetUpTest(c)
	s.watcher = &mockRestoreInfoWatcher{
		status:  state.RestorePending,
		changes: make(chan struct{}),
	}
	s.stub.ResetCalls()
	s.config = restorewatcher.Config{
		RestoreInfoWatcher: s.watcher,
	}
}

func (s *WorkerSuite) TestValidateRestoreInfoWatcher(c *gc.C) {
	s.config.RestoreInfoWatcher = nil
	_, err := restorewatcher.NewWorker(s.config)
	c.Assert(err, gc.ErrorMatches, "nil RestoreInfoWatcher not valid")
}

func (s *WorkerSuite) TestStartStop(c *gc.C) {
	w, err := restorewatcher.NewWorker(s.config)
	c.Assert(err, jc.ErrorIsNil)
	workertest.CleanKill(c, w)
}

func (s *WorkerSuite) TestWorkerObservesChanges(c *gc.C) {
	w, err := restorewatcher.NewWorker(s.config)
	c.Assert(err, jc.ErrorIsNil)
	defer workertest.CleanKill(c, w)

	c.Assert(w.RestoreStatus(), gc.Equals, state.RestorePending)

	s.watcher.status = state.RestoreFailed
	// Send two changes, to ensure the first one is processed.
	for i := 0; i < 2; i++ {
		select {
		case s.watcher.changes <- struct{}{}:
		case <-time.After(coretesting.LongWait):
			c.Fatal("timed out sending watcher change")
		}
	}
	c.Assert(w.RestoreStatus(), gc.Equals, state.RestoreFailed)
}

func (s *WorkerSuite) TestErrorTerminatesWorker(c *gc.C) {
	s.watcher.SetErrors(nil, errors.New("burp"))

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
	return statetesting.NewMockNotifyWatcher(w.changes)
}

func (w *mockRestoreInfoWatcher) RestoreStatus() (state.RestoreStatus, error) {
	w.MethodCall(w, "RestoreStatus")
	return w.status, w.NextErr()
}
