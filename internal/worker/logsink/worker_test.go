// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package logsink

import (
	context "context"
	"sync/atomic"
	"time"

	"github.com/juju/clock"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/worker/v4"
	"github.com/juju/worker/v4/workertest"
	gomock "go.uber.org/mock/gomock"
	gc "gopkg.in/check.v1"
	"gopkg.in/tomb.v2"

	"github.com/juju/juju/core/logger"
	loggertesting "github.com/juju/juju/internal/logger/testing"
)

type workerSuite struct {
	testing.IsolationSuite

	states    chan string
	called    int64
	logWriter *MockLogWriterCloser
}

var _ = gc.Suite(&workerSuite{})

func (s *workerSuite) TestKilledGetLoggerErrDying(c *gc.C) {
	defer s.setupMocks(c).Finish()

	w := s.newWorker(c)
	defer workertest.DirtyKill(c, w)

	s.ensureStartup(c)

	w.Kill()

	worker := w.(*LogSink)
	_, err := worker.GetLogWriter(context.Background(), "foo")
	c.Assert(err, jc.ErrorIs, logger.ErrLoggerDying)
}

func (s *workerSuite) TestKilledGetLoggerContextErrDying(c *gc.C) {
	defer s.setupMocks(c).Finish()

	w := s.newWorker(c)
	defer workertest.DirtyKill(c, w)

	s.ensureStartup(c)

	w.Kill()

	worker := w.(*LogSink)
	_, err := worker.GetLoggerContext(context.Background(), "foo")
	c.Assert(err, jc.ErrorIs, logger.ErrLoggerDying)
}

func (s *workerSuite) TestClose(c *gc.C) {
	defer s.setupMocks(c).Finish()

	w := s.newWorker(c)
	defer workertest.DirtyKill(c, w)

	s.ensureStartup(c)

	worker := w.(*LogSink)
	err := worker.Close()
	c.Assert(err, jc.ErrorIsNil)

	workertest.CheckKill(c, w)
}

func (s *workerSuite) TestGetLogWriter(c *gc.C) {
	defer s.setupMocks(c).Finish()

	w := s.newWorker(c)
	defer workertest.DirtyKill(c, w)

	s.ensureStartup(c)

	worker := w.(*LogSink)
	logger, err := worker.GetLogWriter(context.Background(), "foo")
	c.Assert(err, jc.ErrorIsNil)
	c.Check(logger, gc.NotNil)

	workertest.CheckKill(c, w)
}

func (s *workerSuite) TestGetLogWriterIsCached(c *gc.C) {
	defer s.setupMocks(c).Finish()

	w := s.newWorker(c)
	defer workertest.DirtyKill(c, w)

	s.ensureStartup(c)

	worker := w.(*LogSink)

	for i := 0; i < 10; i++ {
		logger, err := worker.GetLogWriter(context.Background(), "foo")
		c.Assert(err, jc.ErrorIsNil)
		c.Check(logger, gc.NotNil)
	}

	workertest.CheckKill(c, w)

	c.Assert(atomic.LoadInt64(&s.called), gc.Equals, int64(1))
}

func (s *workerSuite) TestGetLoggerContext(c *gc.C) {
	defer s.setupMocks(c).Finish()

	w := s.newWorker(c)
	defer workertest.DirtyKill(c, w)

	s.ensureStartup(c)

	worker := w.(*LogSink)
	logger, err := worker.GetLoggerContext(context.Background(), "foo")
	c.Assert(err, jc.ErrorIsNil)
	c.Check(logger, gc.NotNil)

	workertest.CheckKill(c, w)
}

func (s *workerSuite) TestGetLoggerContextIsCached(c *gc.C) {
	defer s.setupMocks(c).Finish()

	w := s.newWorker(c)
	defer workertest.DirtyKill(c, w)

	s.ensureStartup(c)

	worker := w.(*LogSink)

	for i := 0; i < 10; i++ {
		logger, err := worker.GetLoggerContext(context.Background(), "foo")
		c.Assert(err, jc.ErrorIsNil)
		c.Check(logger, gc.NotNil)
	}

	workertest.CheckKill(c, w)

	c.Assert(atomic.LoadInt64(&s.called), gc.Equals, int64(1))
}

func (s *workerSuite) TestGetLogWriterAndGetLoggerContextIsCachedTogether(c *gc.C) {
	defer s.setupMocks(c).Finish()

	w := s.newWorker(c)
	defer workertest.DirtyKill(c, w)

	s.ensureStartup(c)

	worker := w.(*LogSink)

	// They both should use the same underlying model logger.

	for i := 0; i < 10; i++ {
		if i%2 == 0 {
			_, err := worker.GetLogWriter(context.Background(), "foo")
			c.Assert(err, jc.ErrorIsNil)
			continue
		}

		_, err := worker.GetLoggerContext(context.Background(), "foo")
		c.Assert(err, jc.ErrorIsNil)
	}

	workertest.CheckKill(c, w)

	c.Assert(atomic.LoadInt64(&s.called), gc.Equals, int64(1))
}

func (s *workerSuite) setupMocks(c *gc.C) *gomock.Controller {
	// Ensure we buffer the channel, this is because we might miss the
	// event if we're too quick at starting up.
	s.states = make(chan string, 1)
	atomic.StoreInt64(&s.called, 0)

	ctrl := gomock.NewController(c)

	return ctrl
}

func (s *workerSuite) newWorker(c *gc.C) worker.Worker {
	w, err := newWorker(Config{
		LogSinkConfig: DefaultLogSinkConfig(),
		NewModelLogger: func(ctx context.Context, modelUUID string, newLogWriter logger.LogWriterForModelFunc, bufferSize int, flushInterval time.Duration, clock clock.Clock) (worker.Worker, error) {
			atomic.AddInt64(&s.called, 1)
			return newLoggerWorker(), nil
		},
		LogWriterForModelFunc: func(ctx context.Context, modelUUID string) (logger.LogWriterCloser, error) {
			return s.logWriter, nil
		},
		Logger: loggertesting.WrapCheckLog(c),
		Clock:  clock.WallClock,
	}, s.states)
	c.Assert(err, jc.ErrorIsNil)
	return w
}

func (s *workerSuite) ensureStartup(c *gc.C) {
	select {
	case state := <-s.states:
		c.Assert(state, gc.Equals, stateStarted)
	case <-time.After(testing.ShortWait * 10):
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
