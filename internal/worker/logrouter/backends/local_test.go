// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package backends

import (
	"sync"
	"testing"
	"time"

	"github.com/juju/errors"
	"github.com/juju/loggo/v3"
	"github.com/juju/tc"
	"github.com/juju/worker/v5/workertest"

	corelogger "github.com/juju/juju/core/logger"
	"github.com/juju/juju/internal/worker/logsender"
)

type localBackendSuite struct{}

func TestLocalBackendSuite(t *testing.T) {
	tc.Run(t, &localBackendSuite{})
}

func (s *localBackendSuite) TestValidate(c *tc.C) {
	cfg := LocalConfig{
		BackendBufferSize: 1,
		LogSink:           &recordingLogSink{},
	}
	c.Check(cfg.Validate(), tc.ErrorIsNil)

	cfg.BackendBufferSize = 0
	c.Check(cfg.Validate(), tc.ErrorMatches,
		"non-positive BackendBufferSize not valid")

	cfg = LocalConfig{
		BackendBufferSize: 1,
		LogSink:           nil,
	}
	c.Check(cfg.Validate(), tc.ErrorMatches, "nil LogSink not valid")
}

func (s *localBackendSuite) TestNewLocalValidatesConfig(c *tc.C) {
	_, err := NewLocal(LocalConfig{})
	c.Check(err, tc.ErrorIs, errors.NotValid)
}

func (s *localBackendSuite) TestForwardsRecordsToLogSink(c *tc.C) {
	sink := newRecordingLogSink()
	w, err := NewLocal(LocalConfig{
		BackendBufferSize: 4,
		LogSink:           sink,
	})
	c.Assert(err, tc.ErrorIsNil)
	defer workertest.CleanKill(c, w)

	backend := w.(*localBackend)
	sendBackendLog(c, backend.LogRecords(), "first")
	sendBackendLog(c, backend.LogRecords(), "second")

	records := sink.wait(c, 2)
	c.Assert(records, tc.HasLen, 2)
	c.Check(records[0].Message, tc.Equals, "first")
	c.Check(records[1].Message, tc.Equals, "second")
}

func (s *localBackendSuite) TestPreservesRecordFields(c *tc.C) {
	sink := newRecordingLogSink()
	w, err := NewLocal(LocalConfig{
		BackendBufferSize: 4,
		LogSink:           sink,
	})
	c.Assert(err, tc.ErrorIsNil)
	defer workertest.CleanKill(c, w)

	backend := w.(*localBackend)
	select {
	case backend.LogRecords() <- &logsender.LogRecord{
		Time:      time.Date(2026, 7, 2, 12, 0, 0, 0, time.UTC),
		Module:    "juju.test",
		Location:  "local.go:42",
		Level:     loggo.WARNING,
		Message:   "preserved",
		Labels:    map[string]string{"tag": "unit"},
		ModelUUID: "model-uuid",
		Entity:    "unit-foo/0",
	}:
	case <-c.Context().Done():
		c.Fatal("timed out sending record")
	}

	records := sink.wait(c, 1)
	c.Assert(records, tc.HasLen, 1)
	rec := records[0]
	c.Check(rec.Message, tc.Equals, "preserved")
	c.Check(rec.Module, tc.Equals, "juju.test")
	c.Check(rec.Location, tc.Equals, "local.go:42")
	c.Check(rec.Level, tc.Equals, corelogger.WARNING)
	c.Check(rec.Labels, tc.DeepEquals, map[string]string{"tag": "unit"})
	c.Check(rec.ModelUUID, tc.Equals, "model-uuid")
	c.Check(rec.Entity, tc.Equals, "unit-foo/0")
}

func (s *localBackendSuite) TestNilRecordIgnored(c *tc.C) {
	sink := newRecordingLogSink()
	w, err := NewLocal(LocalConfig{
		BackendBufferSize: 4,
		LogSink:           sink,
	})
	c.Assert(err, tc.ErrorIsNil)
	defer workertest.CleanKill(c, w)

	backend := w.(*localBackend)
	select {
	case backend.LogRecords() <- nil:
	case <-c.Context().Done():
		c.Fatal("timed out sending nil record")
	}

	sendBackendLog(c, backend.LogRecords(), "after-nil")
	records := sink.wait(c, 1)

	c.Assert(records, tc.HasLen, 1)
	c.Check(records[0].Message, tc.Equals, "after-nil")
}

func (s *localBackendSuite) TestLogSinkErrorKillsBackend(c *tc.C) {
	sentinel := errors.New("sink failure")
	sink := &recordingLogSink{err: sentinel}
	w, err := NewLocal(LocalConfig{
		BackendBufferSize: 4,
		LogSink:           sink,
	})
	c.Assert(err, tc.ErrorIsNil)

	backend := w.(*localBackend)
	sendBackendLog(c, backend.LogRecords(), "boom")

	workertest.CheckKilled(c, w)
	err = w.Wait()
	c.Check(err, tc.ErrorMatches, "sink failure")
}

func (s *localBackendSuite) TestReport(c *tc.C) {
	sink := newRecordingLogSink()
	w, err := NewLocal(LocalConfig{
		BackendBufferSize: 4,
		LogSink:           sink,
	})
	c.Assert(err, tc.ErrorIsNil)
	defer workertest.CleanKill(c, w)

	backend := w.(*localBackend)

	sendBackendLog(c, backend.LogRecords(), "buffered")

	// Wait for the record to arrive in the sink so we know the
	// channel has been drained.
	sink.wait(c, 1)

	report := backend.Report(c.Context())
	c.Check(report["name"], tc.Equals, "local-backend")
	c.Check(report["bufferedRecords"], tc.Equals, 0)
}

func (s *localBackendSuite) TestReportShowsBufferedRecords(c *tc.C) {
	sink := &channelBlockingLogSink{
		block:    make(chan struct{}),
		notified: make(chan struct{}, 1),
	}
	w, err := NewLocal(LocalConfig{
		BackendBufferSize: 4,
		LogSink:           sink,
	})
	c.Assert(err, tc.ErrorIsNil)
	defer func() {
		close(sink.block)
		workertest.CleanKill(c, w)
	}()

	backend := w.(*localBackend)
	// The blocking sink blocks inside Log, so the record is
	// consumed from the channel but the loop stalls. Any
	// subsequent records stay buffered.
	sendBackendLog(c, backend.LogRecords(), "stuck")
	sink.waitBlocked(c)

	report := backend.Report(c.Context())
	c.Check(report["name"], tc.Equals, "local-backend")
	// The stuck record has been pulled from the channel by the
	// loop, so the buffer is empty. This verifies that Report
	// reflects the channel depth, not pending in-flight records.
	c.Check(report["bufferedRecords"], tc.Equals, 0)
}

func (s *localBackendSuite) TestLogRecordsReturnsStableChannel(c *tc.C) {
	sink := newRecordingLogSink()
	w, err := NewLocal(LocalConfig{
		BackendBufferSize: 4,
		LogSink:           sink,
	})
	c.Assert(err, tc.ErrorIsNil)
	defer workertest.CleanKill(c, w)

	backend := w.(*localBackend)
	ch1 := backend.LogRecords()
	ch2 := backend.LogRecords()
	c.Check(ch1, tc.Equals, ch2)
}

// recordingLogSink is a thread-safe LogSink that records all logs it
// receives and notifies a channel when new records arrive.
type recordingLogSink struct {
	mu     sync.Mutex
	logs   []corelogger.LogRecord
	notify chan struct{}
	err    error
}

func newRecordingLogSink() *recordingLogSink {
	return &recordingLogSink{
		notify: make(chan struct{}, 256),
	}
}

func (s *recordingLogSink) Log(records []corelogger.LogRecord) error {
	s.mu.Lock()
	s.logs = append(s.logs, records...)
	s.mu.Unlock()
	select {
	case s.notify <- struct{}{}:
	default:
	}
	return s.err
}

func (s *recordingLogSink) WatchRefresh() <-chan struct{} {
	return corelogger.NoRefresh()
}

func (s *recordingLogSink) records() []corelogger.LogRecord {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]corelogger.LogRecord, len(s.logs))
	copy(out, s.logs)
	return out
}

// wait blocks until at least n records have been received, then
// returns a copy of all received records. It fails the test on
// context timeout.
func (s *recordingLogSink) wait(c *tc.C, n int) []corelogger.LogRecord {
	for {
		s.mu.Lock()
		count := len(s.logs)
		s.mu.Unlock()
		if count >= n {
			return s.records()
		}
		select {
		case <-s.notify:
		case <-c.Context().Done():
			c.Fatalf("timed out waiting for %d records, got %d",
				n, count)
		}
	}
}

// channelBlockingLogSink blocks inside Log until its block channel
// is closed. This allows tests to stall the loop goroutine and then
// release it during cleanup.
type channelBlockingLogSink struct {
	block    chan struct{}
	notified chan struct{}
}

func (s *channelBlockingLogSink) Log([]corelogger.LogRecord) error {
	select {
	case s.notified <- struct{}{}:
	default:
	}
	<-s.block
	return nil
}

func (s *channelBlockingLogSink) WatchRefresh() <-chan struct{} {
	return corelogger.NoRefresh()
}

func (s *channelBlockingLogSink) waitBlocked(c *tc.C) {
	select {
	case <-s.notified:
	case <-c.Context().Done():
		c.Fatal("timed out waiting for Log to be called")
	}
}

// compile-time interface checks.
var (
	_ Backend            = (*localBackend)(nil)
	_ corelogger.LogSink = (*recordingLogSink)(nil)
	_ corelogger.LogSink = (*channelBlockingLogSink)(nil)
)
