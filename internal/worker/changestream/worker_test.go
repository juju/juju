// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.
package changestream

import (
	"time"

	"github.com/juju/clock"
	"github.com/juju/errors"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/worker/v3"
	"github.com/juju/worker/v3/workertest"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/core/changestream"
	coredatabase "github.com/juju/juju/core/database"
	"github.com/juju/juju/testing"
)

type workerSuite struct {
	baseSuite
}

var _ = gc.Suite(&workerSuite{})

func (s *workerSuite) TestValidateConfig(c *gc.C) {
	defer s.setupMocks(c).Finish()

	cfg := s.getConfig()
	c.Check(cfg.Validate(), jc.ErrorIsNil)

	cfg.AgentTag = ""
	c.Check(cfg.Validate(), jc.ErrorIs, errors.NotValid)

	cfg.Clock = nil
	c.Check(cfg.Validate(), jc.ErrorIs, errors.NotValid)

	cfg = s.getConfig()
	cfg.Logger = nil
	c.Check(cfg.Validate(), jc.ErrorIs, errors.NotValid)

	cfg = s.getConfig()
	cfg.DBGetter = nil
	c.Check(cfg.Validate(), jc.ErrorIs, errors.NotValid)

	cfg = s.getConfig()
	cfg.FileNotifyWatcher = nil
	c.Check(cfg.Validate(), jc.ErrorIs, errors.NotValid)

	cfg = s.getConfig()
	cfg.Metrics = nil
	c.Check(cfg.Validate(), jc.ErrorIs, errors.NotValid)

	cfg = s.getConfig()
	cfg.NewWatchableDB = nil
	c.Check(cfg.Validate(), jc.ErrorIs, errors.NotValid)
}

func (s *workerSuite) getConfig() WorkerConfig {
	return WorkerConfig{
		AgentTag:          "tag",
		DBGetter:          s.dbGetter,
		FileNotifyWatcher: s.fileNotifyWatcher,
		Clock:             s.clock,
		Logger:            s.logger,
		Metrics:           NewMetricsCollector(),
		NewWatchableDB: func(string, coredatabase.TxnRunner, FileNotifier, clock.Clock, NamespaceMetrics, Logger) (WatchableDBWorker, error) {
			return nil, nil
		},
	}
}

func (s *workerSuite) TestKillGetWatchableDBError(c *gc.C) {
	defer s.setupMocks(c).Finish()

	s.expectAnyLogs()
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
	c.Assert(err, jc.ErrorIsNil)

	close(done)
	workertest.CleanKill(c, w)

	_, err = stream.GetWatchableDB("controller")
	c.Assert(err, jc.ErrorIs, coredatabase.ErrChangeStreamDying)
}

func (s *workerSuite) TestEventSource(c *gc.C) {
	defer s.setupMocks(c).Finish()

	s.expectAnyLogs()
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
	c.Assert(ok, jc.IsTrue, gc.Commentf("worker does not implement ChangeStream"))

	wdb, err := stream.GetWatchableDB("controller")
	c.Assert(err, jc.ErrorIsNil)
	c.Check(wdb, gc.NotNil)

	close(done)
}

func (s *workerSuite) TestEventSourceCalledTwice(c *gc.C) {
	defer s.setupMocks(c).Finish()

	s.expectAnyLogs()
	s.expectClock()

	done := make(chan struct{})

	s.dbGetter.EXPECT().GetDB("controller").Return(s.TxnRunner(), nil).Times(1)
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
	c.Assert(ok, jc.IsTrue, gc.Commentf("worker does not implement ChangeStream"))

	// Ensure that the event queue is only created once.
	_, err := stream.GetWatchableDB("controller")
	c.Assert(err, jc.ErrorIsNil)

	_, err = stream.GetWatchableDB("controller")
	c.Assert(err, jc.ErrorIsNil)

	close(done)
}

func (s *workerSuite) newWorker(c *gc.C, attempts int) worker.Worker {
	cfg := WorkerConfig{
		AgentTag:          "agent-tag",
		DBGetter:          s.dbGetter,
		FileNotifyWatcher: s.fileNotifyWatcher,
		Clock:             s.clock,
		Logger:            s.logger,
		Metrics:           NewMetricsCollector(),
		NewWatchableDB: func(string, coredatabase.TxnRunner, FileNotifier, clock.Clock, NamespaceMetrics, Logger) (WatchableDBWorker, error) {
			attempts--
			if attempts < 0 {
				c.Fatal("NewWatchableDB called too many times")
			}
			return s.watchableDBWorker, nil
		},
	}

	w, err := newWorker(cfg)
	c.Assert(err, jc.ErrorIsNil)
	return w
}
