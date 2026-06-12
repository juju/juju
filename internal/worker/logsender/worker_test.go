// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package logsender_test

import (
	"context"
	stderrors "errors"
	"fmt"
	"io"
	"net/url"
	"sync"
	"sync/atomic"
	stdtesting "testing"
	"time"

	"github.com/canonical/gomock/gomock"
	gorillaws "github.com/gorilla/websocket"
	"github.com/juju/errors"
	"github.com/juju/loggo/v3"
	"github.com/juju/tc"
	"github.com/juju/worker/v5/dependency"
	"github.com/juju/worker/v5/workertest"

	"github.com/juju/juju/api"
	"github.com/juju/juju/api/base"
	apilogsender "github.com/juju/juju/api/logsender"
	"github.com/juju/juju/internal/testhelpers"
	"github.com/juju/juju/internal/testing"
	"github.com/juju/juju/internal/worker/logsender"
	"github.com/juju/juju/internal/worker/logsender/logsendertest"
	"github.com/juju/juju/internal/worker/logsender/mocks"
	"github.com/juju/juju/rpc/params"
)

type workerSuite struct {
	testhelpers.IsolationSuite
}

func TestWorkerSuite(t *stdtesting.T) {
	tc.Run(t, &workerSuite{})
}

type logsenderAPI struct {
	writer *mocks.MockLogWriter
	err    error
}

func (s logsenderAPI) LogWriter(_ context.Context) (apilogsender.LogWriter, error) {
	if s.err != nil {
		return nil, s.err
	}
	return s.writer, nil
}

func (s *workerSuite) TestLogSending(c *tc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	const logCount = 5
	logsCh := make(chan *logsender.LogRecord, logCount)

	wg := sync.WaitGroup{}
	wg.Add(logCount)
	writer := mocks.NewMockLogWriter(ctrl)
	ts := time.Now()
	for i := range logCount {
		location := fmt.Sprintf("loc%d", i)
		message := fmt.Sprintf("%d", i)

		writer.EXPECT().WriteLog(&params.LogRecord{
			Time:     ts,
			Module:   "logsender-test",
			Location: location,
			Level:    loggo.INFO.String(),
			Message:  message,
			Labels:   map[string]string{"foo": "bar"},
		}).DoAndReturn(func(_ *params.LogRecord) error {
			wg.Add(-1)
			return nil
		})
	}
	writer.EXPECT().Close()

	// Start the logsender worker.
	worker := logsender.New(logsCh, logsenderAPI{writer: writer})
	defer workertest.CleanKill(c, worker)

	// Send some logs, also building up what should appear in the
	// database.
	for i := range logCount {
		location := fmt.Sprintf("loc%d", i)
		message := fmt.Sprintf("%d", i)

		logsCh <- &logsender.LogRecord{
			Time:     ts,
			Module:   "logsender-test",
			Location: location,
			Level:    loggo.INFO,
			Message:  message,
			Labels:   map[string]string{"foo": "bar"},
		}
	}

	done := make(chan bool)
	go func() {
		wg.Wait()
		close(done)
	}()

	<-done
}

func (s *workerSuite) TestDroppedLogs(c *tc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	logsCh := make(logsender.LogRecordCh)

	wg := sync.WaitGroup{}
	wg.Add(3)
	writer := mocks.NewMockLogWriter(ctrl)
	ts := time.Now()
	writer.EXPECT().WriteLog(&params.LogRecord{
		Time:     ts,
		Module:   "aaa",
		Location: "loc",
		Level:    loggo.INFO.String(),
		Message:  "message0",
	}).DoAndReturn(func(_ *params.LogRecord) error {
		wg.Add(-1)
		return nil
	})
	writer.EXPECT().WriteLog(&params.LogRecord{
		Time:    ts,
		Module:  "juju.worker.logsender",
		Level:   loggo.WARNING.String(),
		Message: "666 log messages dropped due to lack of API connectivity",
	}).DoAndReturn(func(a *params.LogRecord) error {
		wg.Add(-1)
		return nil
	})
	writer.EXPECT().WriteLog(&params.LogRecord{
		Time:     ts,
		Module:   "zzz",
		Location: "loc",
		Level:    loggo.INFO.String(),
		Message:  "message1",
	}).DoAndReturn(func(_ *params.LogRecord) error {
		wg.Add(-1)
		return nil
	})
	writer.EXPECT().Close()

	// Start the logsender worker.
	worker := logsender.New(logsCh, logsenderAPI{writer: writer})
	defer workertest.CleanKill(c, worker)

	// Send a log record which indicates some messages after it were
	// dropped.
	logsCh <- &logsender.LogRecord{
		Time:         ts,
		Module:       "aaa",
		Location:     "loc",
		Level:        loggo.INFO,
		Message:      "message0",
		DroppedAfter: 666,
	}

	// Send another log record with no drops indicated.
	logsCh <- &logsender.LogRecord{
		Time:     ts,
		Module:   "zzz",
		Location: "loc",
		Level:    loggo.INFO,
		Message:  "message1",
	}

	done := make(chan bool)
	go func() {
		wg.Wait()
		close(done)
	}()

	<-done
}

func (s *workerSuite) TestLogSinkUnavailableDrainsRemoteLogs(c *tc.C) {
	logsCh := make(logsender.LogRecordCh)
	logSenderAPI := logsenderAPI{
		err: errors.WithType(
			stderrors.New("cannot connect to /logsink: server returned HTTP status 503"),
			api.HTTPStatusServiceUnavailable,
		),
	}

	w := logsender.New(logsCh, logSenderAPI)
	defer workertest.CleanKill(c, w)

	sendLog(c, logsCh, "one")
	sendLog(c, logsCh, "two")
	workertest.CheckAlive(c, w)
}

func (s *workerSuite) TestDisconnectedAPICallerTerminatesWorker(c *tc.C) {
	logsCh := make(logsender.LogRecordCh)
	logSenderAPI := logsenderAPI{
		err: stderrors.New("api caller disconnected"),
	}

	w := logsender.New(logsCh, logSenderAPI)
	err := w.Wait()
	c.Assert(err, tc.ErrorMatches, "logsender dial failed: api caller disconnected")
}

func (s *workerSuite) TestWriteLogFailureTerminatesWorker(c *tc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	logsCh := make(logsender.LogRecordCh, 1)
	writer := mocks.NewMockLogWriter(ctrl)
	writer.EXPECT().WriteLog(gomock.Any()).Return(stderrors.New("write failed"))
	writer.EXPECT().Close()

	logsCh <- &logsender.LogRecord{
		Time:     time.Now(),
		Module:   "test",
		Location: "test:1",
		Level:    loggo.INFO,
		Message:  "hello",
	}

	w := logsender.New(logsCh, logsenderAPI{writer: writer})
	err := w.Wait()
	c.Assert(err, tc.ErrorMatches, "write failed")
}

func (s *workerSuite) TestWriteLogServiceUnavailableDrainsRemoteLogs(c *tc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	logsCh := make(logsender.LogRecordCh)
	writer := mocks.NewMockLogWriter(ctrl)
	writer.EXPECT().WriteLog(gomock.Any()).Return(errors.WithType(
		stderrors.New("sending log message: server returned HTTP status 503"),
		api.HTTPStatusServiceUnavailable,
	))
	writer.EXPECT().Close()

	w := logsender.New(logsCh, logsenderAPI{writer: writer})
	defer workertest.CleanKill(c, w)

	sendLog(c, logsCh, "one")
	sendLog(c, logsCh, "two")
	workertest.CheckAlive(c, w)
}

func (s *workerSuite) TestLogSinkUnavailableDrainsBufferedLogs(c *tc.C) {
	bufferedLogger := logsender.NewBufferedLogWriter(10)
	defer bufferedLogger.Close()

	logSenderAPI := logsenderAPI{
		err: errors.WithType(
			stderrors.New("cannot connect to /logsink: server returned HTTP status 503"),
			api.HTTPStatusServiceUnavailable,
		),
	}

	w := logsender.New(bufferedLogger.Logs(), logSenderAPI)
	defer workertest.CleanKill(c, w)

	for i := range 5 {
		err := bufferedLogger.Write(c.Context(), loggo.Entry{
			Level:     loggo.INFO,
			Module:    "test",
			Filename:  "test.go",
			Line:      i,
			Timestamp: time.Now(),
			Message:   fmt.Sprintf("message%d", i),
		})
		c.Assert(err, tc.ErrorIsNil)
	}

	logsendertest.ExpectLogStats(c, bufferedLogger, logsender.LogStats{
		Enqueued: 5,
		Sent:     5,
	})
	workertest.CheckAlive(c, w)
}

func sendLog(c *tc.C, logsCh logsender.LogRecordCh, message string) {
	select {
	case logsCh <- &logsender.LogRecord{
		Time:     time.Now(),
		Module:   "test",
		Location: "test:1",
		Level:    loggo.INFO,
		Message:  message,
	}:
	case <-c.Context().Done():
		c.Fatalf("timed out sending log record")
	}
}

type workerBounceSuite struct {
	testing.BaseSuite
}

func TestWorkerBounceSuite(t *stdtesting.T) {
	tc.Run(t, &workerBounceSuite{})
}

type mockConnector struct {
	stream base.Stream
}

func (c *mockConnector) ConnectStream(_ context.Context, _ string, _ url.Values) (base.Stream, error) {
	return c.stream, nil
}

type mockStream struct {
	c              *tc.C
	succeedNWrites int
	writeCount     int32
	writesReady    chan struct{}
	closed         chan struct{}
	closeOnce      sync.Once
}

func (s *mockStream) NextReader() (int, io.Reader, error) {
	if s.writesReady != nil {
		select {
		case <-s.writesReady:
		case <-time.After(testing.LongWait):
			s.c.Fatalf("expected number of writes not received")
		}
	}
	return 0, nil, &gorillaws.CloseError{Code: gorillaws.CloseNormalClosure}
}

func (s *mockStream) WriteJSON(v any) error {
	count := atomic.AddInt32(&s.writeCount, 1)
	if int(count) <= s.succeedNWrites {
		if int(count) == s.succeedNWrites {
			close(s.writesReady)
		}
		return nil
	}
	// Ensure readLoop has processed the close error before we return.
	<-s.closed
	return fmt.Errorf("use of closed network connection")
}

func (s *mockStream) ReadJSON(v any) error {
	s.c.Fatal("ReadJSON called unexpectedly")
	return nil
}

func (s *mockStream) Close() error {
	s.closeOnce.Do(func() { close(s.closed) })
	return nil
}

func (s *workerBounceSuite) TestWriteLogEOFReturnsBounce(c *tc.C) {
	stream := &mockStream{
		c:              c,
		succeedNWrites: 0,
		closed:         make(chan struct{}),
	}
	logSenderAPI := apilogsender.NewAPI(&mockConnector{stream: stream})

	logsCh := make(logsender.LogRecordCh, 1)
	logsCh <- &logsender.LogRecord{
		Time:     time.Now(),
		Module:   "test",
		Location: "test:1",
		Level:    loggo.INFO,
		Message:  "hello",
	}

	w := logsender.New(logsCh, logSenderAPI)
	err := w.Wait()
	c.Assert(err, tc.Equals, dependency.ErrBounce)
}

func (s *workerBounceSuite) TestDroppedLogWriteEOFReturnsBounce(c *tc.C) {
	stream := &mockStream{
		c:              c,
		succeedNWrites: 1,
		writesReady:    make(chan struct{}),
		closed:         make(chan struct{}),
	}
	logSenderAPI := apilogsender.NewAPI(&mockConnector{stream: stream})

	logsCh := make(logsender.LogRecordCh, 1)
	logsCh <- &logsender.LogRecord{
		Time:         time.Now(),
		Module:       "test",
		Location:     "test:1",
		Level:        loggo.INFO,
		Message:      "hello",
		DroppedAfter: 5,
	}

	w := logsender.New(logsCh, logSenderAPI)
	err := w.Wait()
	c.Assert(err, tc.Equals, dependency.ErrBounce)
}
