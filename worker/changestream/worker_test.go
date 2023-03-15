// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.
package changestream

import (
	"time"

	"github.com/juju/clock"
	"github.com/juju/errors"
	coredatabase "github.com/juju/juju/core/database"
	"github.com/juju/juju/testing"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/worker/v3"
	"github.com/juju/worker/v3/workertest"
	gc "gopkg.in/check.v1"
)

type workerSuite struct {
	baseSuite
}

var _ = gc.Suite(&workerSuite{})

func (s *workerSuite) TestValidateConfig(c *gc.C) {
	defer s.setupMocks(c).Finish()

	cfg := s.getConfig()
	c.Check(cfg.Validate(), jc.ErrorIsNil)

	cfg.Clock = nil
	c.Check(errors.Is(cfg.Validate(), errors.NotValid), jc.IsTrue)

	cfg = s.getConfig()
	cfg.Logger = nil
	c.Check(errors.Is(cfg.Validate(), errors.NotValid), jc.IsTrue)

	cfg = s.getConfig()
	cfg.DBGetter = nil
	c.Check(errors.Is(cfg.Validate(), errors.NotValid), jc.IsTrue)

	cfg = s.getConfig()
	cfg.FileNotifyWatcher = nil
	c.Check(errors.Is(cfg.Validate(), errors.NotValid), jc.IsTrue)

	cfg = s.getConfig()
	cfg.NewEventQueueWorker = nil
	c.Check(errors.Is(cfg.Validate(), errors.NotValid), jc.IsTrue)
}

func (s *workerSuite) getConfig() WorkerConfig {
	return WorkerConfig{
		DBGetter:          s.dbGetter,
		FileNotifyWatcher: s.fileNotifyWatcher,
		Clock:             s.clock,
		Logger:            s.logger,
		NewEventQueueWorker: func(coredatabase.TrackedDB, FileNotifier, clock.Clock, Logger) (EventQueueWorker, error) {
			return nil, nil
		},
	}
}

func (s *workerSuite) TestEventQueue(c *gc.C) {
	defer s.setupMocks(c).Finish()

	s.expectAnyLogs()
	s.expectClock()

	s.dbGetter.EXPECT().GetDB("controller").Return(s.TrackedDB(), nil)
	s.eventQueueWorker.EXPECT().EventQueue().Return(s.eventQueue)
	s.eventQueueWorker.EXPECT().Kill().AnyTimes()
	s.eventQueueWorker.EXPECT().Wait().MinTimes(1)

	w := s.newWorker(c, 1)
	defer workertest.DirtyKill(c, w)

	stream, ok := w.(ChangeStream)
	c.Assert(ok, jc.IsTrue, gc.Commentf("worker does not implement ChangeStream"))

	_, err := stream.EventQueue("controller")
	c.Assert(err, jc.ErrorIsNil)

	workertest.CleanKill(c, w)
}

func (s *workerSuite) TestEventQueueCalledTwice(c *gc.C) {
	defer s.setupMocks(c).Finish()

	s.expectAnyLogs()
	s.expectClock()

	done := make(chan struct{})

	s.dbGetter.EXPECT().GetDB("controller").Return(s.TrackedDB(), nil)
	s.eventQueueWorker.EXPECT().EventQueue().Return(s.eventQueue).Times(2)
	s.eventQueueWorker.EXPECT().Kill().AnyTimes()
	s.eventQueueWorker.EXPECT().Wait().DoAndReturn(func() error {
		select {
		case <-done:
		case <-time.After(testing.LongWait):
			c.Fatal("timed out waiting for Wait to be called")
		}
		return nil
	})

	w := s.newWorker(c, 1)
	defer workertest.DirtyKill(c, w)

	stream, ok := w.(ChangeStream)
	c.Assert(ok, jc.IsTrue, gc.Commentf("worker does not implement ChangeStream"))

	// Ensure that the event queue is only created once.
	_, err := stream.EventQueue("controller")
	c.Assert(err, jc.ErrorIsNil)

	_, err = stream.EventQueue("controller")
	c.Assert(err, jc.ErrorIsNil)

	close(done)

	workertest.CleanKill(c, w)
}

func (s *workerSuite) newWorker(c *gc.C, attempts int) worker.Worker {
	cfg := WorkerConfig{
		DBGetter:          s.dbGetter,
		FileNotifyWatcher: s.fileNotifyWatcher,
		Clock:             s.clock,
		Logger:            s.logger,
		NewEventQueueWorker: func(coredatabase.TrackedDB, FileNotifier, clock.Clock, Logger) (EventQueueWorker, error) {
			attempts--
			if attempts < 0 {
				c.Fatal("NewEventQueueWorker called too many times")
			}
			return s.eventQueueWorker, nil
		},
	}

	w, err := newWorker(cfg)
	c.Assert(err, jc.ErrorIsNil)
	return w
}
