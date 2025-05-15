// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package logsink

import (
	"sync/atomic"
	"time"

	"github.com/juju/clock"
	"github.com/juju/names/v6"
	"github.com/juju/tc"
	"github.com/juju/worker/v4"
	"github.com/juju/worker/v4/workertest"
	gomock "go.uber.org/mock/gomock"
	"gopkg.in/tomb.v2"

	"github.com/juju/juju/core/logger"
	model "github.com/juju/juju/core/model"
	modeltesting "github.com/juju/juju/core/model/testing"
	"github.com/juju/juju/internal/testhelpers"
)

type workerSuite struct {
	testhelpers.IsolationSuite

	states chan string
	called int64
}

var _ = tc.Suite(&workerSuite{})

func (s *workerSuite) TestKilledGetLogger(c *tc.C) {
	defer s.setupMocks(c).Finish()

	id := modeltesting.GenModelUUID(c)

	w := s.newWorker(c)
	defer workertest.DirtyKill(c, w)

	s.ensureStartup(c)

	w.Kill()

	worker := w.(*LogSink)
	_, err := worker.GetLogWriter(c.Context(), id)
	c.Assert(err, tc.ErrorIs, logger.ErrLoggerDying)
}

func (s *workerSuite) TestKilledGetLoggerContext(c *tc.C) {
	defer s.setupMocks(c).Finish()

	id := modeltesting.GenModelUUID(c)

	w := s.newWorker(c)
	defer workertest.DirtyKill(c, w)

	s.ensureStartup(c)

	w.Kill()

	worker := w.(*LogSink)
	_, err := worker.GetLoggerContext(c.Context(), id)
	c.Assert(err, tc.ErrorIs, logger.ErrLoggerDying)
}

func (s *workerSuite) TestGetLogWriter(c *tc.C) {
	defer s.setupMocks(c).Finish()

	id := modeltesting.GenModelUUID(c)

	w := s.newWorker(c)
	defer workertest.DirtyKill(c, w)

	s.ensureStartup(c)

	worker := w.(*LogSink)
	logger, err := worker.GetLogWriter(c.Context(), id)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(logger, tc.NotNil)

	workertest.CheckKill(c, w)
}

func (s *workerSuite) TestGetLogWriterIsCached(c *tc.C) {
	defer s.setupMocks(c).Finish()

	id := modeltesting.GenModelUUID(c)

	w := s.newWorker(c)
	defer workertest.DirtyKill(c, w)

	s.ensureStartup(c)

	worker := w.(*LogSink)

	for i := 0; i < 10; i++ {
		logger, err := worker.GetLogWriter(c.Context(), id)
		c.Assert(err, tc.ErrorIsNil)
		c.Check(logger, tc.NotNil)
	}

	workertest.CheckKill(c, w)

	c.Assert(atomic.LoadInt64(&s.called), tc.Equals, int64(1))
}

func (s *workerSuite) TestGetLoggerContext(c *tc.C) {
	defer s.setupMocks(c).Finish()

	id := modeltesting.GenModelUUID(c)

	w := s.newWorker(c)
	defer workertest.DirtyKill(c, w)

	s.ensureStartup(c)

	worker := w.(*LogSink)

	logger, err := worker.GetLoggerContext(c.Context(), id)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(logger, tc.NotNil)

	workertest.CheckKill(c, w)
}

func (s *workerSuite) TestGetLoggerContextIsCached(c *tc.C) {
	defer s.setupMocks(c).Finish()

	id := modeltesting.GenModelUUID(c)

	w := s.newWorker(c)
	defer workertest.DirtyKill(c, w)

	s.ensureStartup(c)

	worker := w.(*LogSink)

	for i := 0; i < 10; i++ {
		logger, err := worker.GetLoggerContext(c.Context(), id)
		c.Assert(err, tc.ErrorIsNil)
		c.Check(logger, tc.NotNil)
	}

	workertest.CheckKill(c, w)

	c.Assert(atomic.LoadInt64(&s.called), tc.Equals, int64(1))
}

func (s *workerSuite) TestGetLogWriterAndGetLoggerContextIsCachedTogether(c *tc.C) {
	defer s.setupMocks(c).Finish()

	id := modeltesting.GenModelUUID(c)

	w := s.newWorker(c)
	defer workertest.DirtyKill(c, w)

	s.ensureStartup(c)

	worker := w.(*LogSink)

	// They both should use the same underlying model logger.

	for i := 0; i < 10; i++ {
		if i%2 == 0 {
			_, err := worker.GetLogWriter(c.Context(), id)
			c.Assert(err, tc.ErrorIsNil)
			continue
		}

		_, err := worker.GetLoggerContext(c.Context(), id)
		c.Assert(err, tc.ErrorIsNil)
	}

	workertest.CheckKill(c, w)

	c.Assert(atomic.LoadInt64(&s.called), tc.Equals, int64(1))
}

func (s *workerSuite) setupMocks(c *tc.C) *gomock.Controller {
	// Ensure we buffer the channel, this is because we might miss the
	// event if we're too quick at starting up.
	s.states = make(chan string, 1)
	atomic.StoreInt64(&s.called, 0)

	ctrl := gomock.NewController(c)

	return ctrl
}

func (s *workerSuite) newWorker(c *tc.C) worker.Worker {
	w, err := newWorker(Config{
		NewModelLogger: func(logger.LogSink, model.UUID, names.Tag) (worker.Worker, error) {
			atomic.AddInt64(&s.called, 1)
			return newLoggerWorker(), nil
		},

		Clock: clock.WallClock,
	}, s.states)
	c.Assert(err, tc.ErrorIsNil)
	return w
}

func (s *workerSuite) ensureStartup(c *tc.C) {
	select {
	case state := <-s.states:
		c.Assert(state, tc.Equals, stateStarted)
	case <-time.After(testhelpers.ShortWait * 10):
		c.Fatalf("timed out waiting for startup")
	}
}

type loggerWorker struct {
	LogSinkWriter
	tomb tomb.Tomb
}

func newLoggerWorker() *loggerWorker {
	w := &loggerWorker{}
	w.tomb.Go(func() error {
		<-w.tomb.Dying()
		return tomb.ErrDying
	})
	return w
}

func (w *loggerWorker) Kill() {
	w.tomb.Kill(nil)
}

func (w *loggerWorker) Wait() error {
	return w.tomb.Wait()
}
