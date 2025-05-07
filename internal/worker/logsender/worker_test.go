// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package logsender_test

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/juju/loggo/v2"
	"github.com/juju/tc"
	jtesting "github.com/juju/testing"
	"github.com/juju/worker/v4/workertest"
	"go.uber.org/mock/gomock"

	apilogsender "github.com/juju/juju/api/logsender"
	"github.com/juju/juju/internal/testing"
	"github.com/juju/juju/internal/worker/logsender"
	"github.com/juju/juju/internal/worker/logsender/mocks"
	"github.com/juju/juju/rpc/params"
)

type workerSuite struct {
	jtesting.IsolationSuite
}

var _ = tc.Suite(&workerSuite{})

type logsenderAPI struct {
	writer *mocks.MockLogWriter
}

func (s logsenderAPI) LogWriter(_ context.Context) (apilogsender.LogWriter, error) {
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
	for i := 0; i < logCount; i++ {
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
	worker := logsender.New(logsCh, logsenderAPI{writer})
	defer workertest.CleanKill(c, worker)

	// Send some logs, also building up what should appear in the
	// database.
	for i := 0; i < logCount; i++ {
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

	select {
	case <-done:
	case <-time.After(testing.ShortWait):
		c.Fatal("timed out waiting for all events")
	}
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
	worker := logsender.New(logsCh, logsenderAPI{writer})
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

	select {
	case <-done:
	case <-time.After(testing.ShortWait):
		c.Fatal("timed out waiting for all events")
	}
}
