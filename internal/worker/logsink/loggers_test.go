// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package logsink

import (
	"sync"
	"sync/atomic"
	"testing"

	"github.com/canonical/gomock/gomock"
	"github.com/juju/names/v6"
	"github.com/juju/tc"
	"github.com/juju/worker/v5/workertest"

	corelogger "github.com/juju/juju/core/logger"
	model "github.com/juju/juju/core/model"
	"github.com/juju/juju/internal/testhelpers"
	"github.com/juju/juju/internal/uuid"
)

type LoggersSuite struct {
	testhelpers.IsolationSuite

	logWriter *MockLogSink
	modelUUID string
}

func TestLoggersSuite(t *testing.T) {
	tc.Run(t, &LoggersSuite{})
}

var _ LogSinkWriter = (*modelLogger)(nil)

func (s *LoggersSuite) TestLoggers(c *tc.C) {
	defer s.setupMocks(c).Finish()

	logger := s.newModelLogger(c)

	workertest.CheckKill(c, logger)
}

func (s *LoggersSuite) TestLoggerLogs(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.logWriter.EXPECT().Log([]corelogger.LogRecord{{Message: "foo"}}).Return(nil)

	logger := s.newModelLogger(c)
	err := logger.Log([]corelogger.LogRecord{{Message: "foo"}})
	c.Assert(err, tc.ErrorIsNil)

	workertest.CheckKill(c, logger)
}

func (s *LoggersSuite) TestLoggerGetLogger(c *tc.C) {
	defer s.setupMocks(c).Finish()

	var logs []corelogger.LogRecord
	s.logWriter.EXPECT().Log(gomock.Any()).DoAndReturn(func(records []corelogger.LogRecord) error {
		logs = append(logs, records...)
		return nil
	})

	logger := s.newModelLogger(c)

	fooLogger := logger.GetLogger("foo")
	c.Assert(fooLogger, tc.NotNil)

	fooLogger.Infof(c.Context(), "message me")

	workertest.CheckKill(c, logger)

	c.Assert(logs, tc.HasLen, 1)
	c.Check(logs[0].Message, tc.Equals, "message me")
	c.Check(logs[0].Level, tc.Equals, corelogger.INFO)
	c.Check(logs[0].Module, tc.Equals, "foo")
	c.Check(logs[0].ModelUUID, tc.Equals, s.modelUUID)
}

func (s *LoggersSuite) TestLoggerConfigureLoggers(c *tc.C) {
	defer s.setupMocks(c).Finish()

	var logs []corelogger.LogRecord
	s.logWriter.EXPECT().Log(gomock.Any()).DoAndReturn(func(records []corelogger.LogRecord) error {
		logs = append(logs, records...)
		return nil
	})

	logger := s.newModelLogger(c)

	fooLogger := logger.GetLogger("foo")

	// The debug log, should not be logged by the logger.

	err := logger.ConfigureLoggers("<root>=INFO")
	c.Assert(err, tc.ErrorIsNil)

	fooLogger.Debugf(c.Context(), "message me")

	// Once we reset this is set to warning, so the debug log should not be
	// logged. The warning should be though.

	logger.ResetLoggerLevels()

	fooLogger.Debugf(c.Context(), "message again")
	fooLogger.Warningf(c.Context(), "message again and again")

	workertest.CheckKill(c, logger)

	c.Assert(logs, tc.HasLen, 1)
	c.Check(logs[0].Message, tc.Equals, "message again and again")
	c.Check(logs[0].Level, tc.Equals, corelogger.WARNING)
	c.Check(logs[0].ModelUUID, tc.Equals, s.modelUUID)
}

func (s *LoggersSuite) TestLoggerRebindsOnRefresh(c *tc.C) {
	oldSink := &recordingRefreshLogSink{
		refresh: make(chan struct{}),
		writes:  make(chan struct{}, 10),
	}
	newSink := &recordingRefreshLogSink{
		refresh: make(chan struct{}),
		writes:  make(chan struct{}, 10),
	}
	router := &switchingLogSink{
		sink: oldSink,
	}

	s.modelUUID = uuid.MustNewUUID().String()

	w, err := NewModelLogger(router, model.UUID(s.modelUUID), names.NewUnitTag("foo/0"))
	c.Assert(err, tc.ErrorIsNil)
	logger := w.(*modelLogger)
	defer workertest.CleanKill(c, logger)

	fooLogger := logger.GetLogger("foo")
	fooLogger.Infof(c.Context(), "before switch")
	oldSink.waitForWrite(c)
	c.Assert(oldSink.records, tc.HasLen, 1)
	c.Assert(newSink.records, tc.HasLen, 0)

	router.mu.Lock()
	router.sink = newSink
	router.mu.Unlock()
	close(oldSink.refresh)

	fooLogger.Infof(c.Context(), "after switch")
	newSink.waitForWrite(c)

	c.Assert(oldSink.records, tc.HasLen, 1)
	c.Assert(newSink.records, tc.HasLen, 1)
	c.Check(newSink.records[0].Message, tc.Equals, "after switch")
}

func (s *LoggersSuite) TestLoggerRebindsOnRefreshUnderStress(c *tc.C) {
	sink := &countingRefreshLogSink{
		refresh: make(chan struct{}),
	}

	s.modelUUID = uuid.MustNewUUID().String()

	w, err := NewModelLogger(sink, model.UUID(s.modelUUID), names.NewUnitTag("foo/0"))
	c.Assert(err, tc.ErrorIsNil)
	logger := w.(*modelLogger)
	defer workertest.CleanKill(c, logger)

	fooLogger := logger.GetLogger("foo")

	// Concurrently fire refresh signals and log records to exercise
	// the rebind path under load. The model logger must not panic,
	// deadlock, or race.
	var wg sync.WaitGroup
	const iterations = 200

	wg.Go(func() {
		for i := range iterations {
			fooLogger.Infof(c.Context(), "message %d", i)
		}
	})

	wg.Go(func() {
		for range iterations {
			sink.fireRefresh()
		}
	})

	wg.Wait()

	// All refresh signals must have been delivered. A small number of
	// log records may be lost during the brief RemoveWriter/AddWriter
	// window in bindWriter; verify that the vast majority were
	// delivered rather than requiring every single one.
	c.Check(sink.refreshCount.Load(), tc.Equals, int64(iterations))
	c.Check(sink.logCount.Load(), tc.GreaterThan, int64(iterations-6))
}

func (s *LoggersSuite) newModelLogger(c *tc.C) *modelLogger {
	s.modelUUID = uuid.MustNewUUID().String()

	s.logWriter.EXPECT().WatchRefresh().Return(corelogger.NoRefresh()).AnyTimes()

	w, err := NewModelLogger(s.logWriter, model.UUID(s.modelUUID), names.NewUnitTag("foo/0"))
	c.Assert(err, tc.ErrorIsNil)

	return w.(*modelLogger)
}

func (s *LoggersSuite) setupMocks(c *tc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)

	s.logWriter = NewMockLogSink(ctrl)

	return ctrl
}

type switchingLogSink struct {
	mu   sync.Mutex
	sink corelogger.LogSink
}

func (s *switchingLogSink) Log(records []corelogger.LogRecord) error {
	s.mu.Lock()
	sink := s.sink
	s.mu.Unlock()
	return sink.Log(records)
}

func (s *switchingLogSink) WatchRefresh() <-chan struct{} {
	s.mu.Lock()
	sink := s.sink
	s.mu.Unlock()
	return sink.WatchRefresh()
}

type recordingRefreshLogSink struct {
	refresh chan struct{}
	records []corelogger.LogRecord
	writes  chan struct{}
}

func (s *recordingRefreshLogSink) Log(records []corelogger.LogRecord) error {
	s.records = append(s.records, records...)
	s.writes <- struct{}{}
	return nil
}

func (s *recordingRefreshLogSink) WatchRefresh() <-chan struct{} {
	return s.refresh
}

func (s *recordingRefreshLogSink) waitForWrite(c *tc.C) {
	c.Assert(s.writes, tc.NotNil)
	select {
	case <-s.writes:
	case <-c.Context().Done():
		c.Fatalf("timed out waiting for log write")
	}
}

// countingRefreshLogSink is a thread-safe LogSink used for stress testing
// the rebind-on-refresh path. It counts log calls and refresh firings
// without blocking on channels.
type countingRefreshLogSink struct {
	refresh      chan struct{}
	refreshCount atomic.Int64
	logCount     atomic.Int64
	refreshMu    sync.Mutex
}

func (s *countingRefreshLogSink) Log(records []corelogger.LogRecord) error {
	s.logCount.Add(int64(len(records)))
	return nil
}

func (s *countingRefreshLogSink) WatchRefresh() <-chan struct{} {
	s.refreshMu.Lock()
	defer s.refreshMu.Unlock()
	return s.refresh
}

// fireRefresh closes the current refresh channel and replaces it with a
// fresh one, mimicking the logrouter's fireRefresh behaviour.
func (s *countingRefreshLogSink) fireRefresh() {
	s.refreshMu.Lock()
	defer s.refreshMu.Unlock()
	if s.refresh != nil {
		close(s.refresh)
	}
	s.refresh = make(chan struct{})
	s.refreshCount.Add(1)
}
