// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.
package changestream

import (
	stdtesting "testing"
	"time"

	"github.com/juju/clock"
	"github.com/juju/errors"
	"github.com/juju/tc"
	"github.com/juju/worker/v4"
	"github.com/juju/worker/v4/workertest"
	"go.uber.org/goleak"

	"github.com/juju/juju/core/changestream"
	coredatabase "github.com/juju/juju/core/database"
	"github.com/juju/juju/core/logger"
	loggertesting "github.com/juju/juju/internal/logger/testing"
	"github.com/juju/juju/internal/testing"
)

type workerSuite struct {
	baseSuite
}

func TestWorkerSuite(t *stdtesting.T) {
	defer goleak.VerifyNone(t)
	tc.Run(t, &workerSuite{})
}

func (s *workerSuite) TestValidateConfig(c *tc.C) {
	defer s.setupMocks(c).Finish()

	cfg := s.getConfig(c)
	c.Check(cfg.Validate(), tc.ErrorIsNil)

	cfg.AgentTag = ""
	c.Check(cfg.Validate(), tc.ErrorIs, errors.NotValid)

	cfg.Clock = nil
	c.Check(cfg.Validate(), tc.ErrorIs, errors.NotValid)

	cfg = s.getConfig(c)
	cfg.Logger = nil
	c.Check(cfg.Validate(), tc.ErrorIs, errors.NotValid)

	cfg = s.getConfig(c)
	cfg.DBGetter = nil
	c.Check(cfg.Validate(), tc.ErrorIs, errors.NotValid)

	cfg = s.getConfig(c)
	cfg.FileNotifyWatcher = nil
	c.Check(cfg.Validate(), tc.ErrorIs, errors.NotValid)

	cfg = s.getConfig(c)
	cfg.Metrics = nil
	c.Check(cfg.Validate(), tc.ErrorIs, errors.NotValid)

	cfg = s.getConfig(c)
	cfg.NewWatchableDB = nil
	c.Check(cfg.Validate(), tc.ErrorIs, errors.NotValid)
}

func (s *workerSuite) getConfig(c *tc.C) WorkerConfig {
	return WorkerConfig{
		AgentTag:          "tag",
		DBGetter:          s.dbGetter,
		FileNotifyWatcher: s.fileNotifyWatcher,
		Clock:             s.clock,
		Logger:            loggertesting.WrapCheckLog(c),
		Metrics:           NewMetricsCollector(),
		NewWatchableDB: func(string, coredatabase.TxnRunner, FileNotifier, clock.Clock, NamespaceMetrics, logger.Logger) (WatchableDBWorker, error) {
			return nil, nil
		},
	}
}

func (s *workerSuite) TestKillGetWatchableDBError(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.expectClock()

	done := make(chan struct{})

	s.dbGetter.EXPECT().GetDB("controller").Return(s.TxnRunner(), nil)
	s.watchableDBWorker.EXPECT().Kill().AnyTimes()
	s.watchableDBWorker.EXPECT().Wait().DoAndReturn(func() error {
		select {
		case <-done:
		case <-time.After(testing.LongWait):
			c.Fatal("timed out waiting for Wait to be called")
		}
		return nil
	})

	w := s.newWorker(c, 1)
	defer workertest.DirtyKill(c, w)
	stream, _ := w.(changestream.WatchableDBGetter)

	_, err := stream.GetWatchableDB("controller")
	c.Assert(err, tc.ErrorIsNil)

	close(done)
	workertest.CleanKill(c, w)

	_, err = stream.GetWatchableDB("controller")
	c.Assert(err, tc.ErrorIs, coredatabase.ErrChangeStreamDying)
}

func (s *workerSuite) TestEventSource(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.expectClock()

	done := make(chan struct{})

	s.dbGetter.EXPECT().GetDB("controller").Return(s.TxnRunner(), nil)
	s.watchableDBWorker.EXPECT().Kill().AnyTimes()
	s.watchableDBWorker.EXPECT().Wait().DoAndReturn(func() error {
		select {
		case <-done:
		case <-time.After(testing.LongWait):
			c.Fatal("timed out waiting for Wait to be called")
		}
		return nil
	})

	w := s.newWorker(c, 1)
	defer workertest.CleanKill(c, w)

	stream, ok := w.(changestream.WatchableDBGetter)
	c.Assert(ok, tc.IsTrue, tc.Commentf("worker does not implement ChangeStream"))

	wdb, err := stream.GetWatchableDB("controller")
	c.Assert(err, tc.ErrorIsNil)
	c.Check(wdb, tc.NotNil)

	close(done)
}

func (s *workerSuite) TestEventSourceCalledTwice(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.expectClock()

	done := make(chan struct{})

	s.dbGetter.EXPECT().GetDB("controller").Return(s.TxnRunner(), nil)
	s.watchableDBWorker.EXPECT().Kill().AnyTimes()
	s.watchableDBWorker.EXPECT().Wait().DoAndReturn(func() error {
		select {
		case <-done:
		case <-time.After(testing.LongWait):
			c.Fatal("timed out waiting for Wait to be called")
		}
		return nil
	})

	w := s.newWorker(c, 1)
	defer workertest.CleanKill(c, w)

	stream, ok := w.(changestream.WatchableDBGetter)
	c.Assert(ok, tc.IsTrue, tc.Commentf("worker does not implement ChangeStream"))

	// Ensure that the event queue is only created once.
	_, err := stream.GetWatchableDB("controller")
	c.Assert(err, tc.ErrorIsNil)

	_, err = stream.GetWatchableDB("controller")
	c.Assert(err, tc.ErrorIsNil)

	close(done)
}

func (s *workerSuite) TestEventSourceCalledWithError(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.expectClock()

	// Test that the worker doesn't restart in the face of a ErrDBNotFound
	// error.

	s.dbGetter.EXPECT().GetDB("controller").Return(s.TxnRunner(), coredatabase.ErrDBNotFound)
	s.watchableDBWorker.EXPECT().Kill().AnyTimes()

	w := s.newWorker(c, 1)
	defer workertest.CleanKill(c, w)

	stream, ok := w.(changestream.WatchableDBGetter)
	c.Assert(ok, tc.IsTrue, tc.Commentf("worker does not implement ChangeStream"))

	// Ensure that the event queue is only created once.
	_, err := stream.GetWatchableDB("controller")
	c.Assert(err, tc.ErrorIs, errors.NotFound)
}

func (s *workerSuite) newWorker(c *tc.C, attempts int) worker.Worker {
	cfg := WorkerConfig{
		AgentTag:          "agent-tag",
		DBGetter:          s.dbGetter,
		FileNotifyWatcher: s.fileNotifyWatcher,
		Clock:             s.clock,
		Logger:            loggertesting.WrapCheckLog(c),
		Metrics:           NewMetricsCollector(),
		NewWatchableDB: func(string, coredatabase.TxnRunner, FileNotifier, clock.Clock, NamespaceMetrics, logger.Logger) (WatchableDBWorker, error) {
			attempts--
			if attempts < 0 {
				c.Fatal("NewWatchableDB called too many times")
			}
			return s.watchableDBWorker, nil
		},
	}

	w, err := newWorker(cfg)
	c.Assert(err, tc.ErrorIsNil)
	return w
}
