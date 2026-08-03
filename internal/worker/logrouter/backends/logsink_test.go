// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package backends

import (
	"context"
	stderrors "errors"
	"testing"
	"time"

	"github.com/canonical/gomock/gomock"
	"github.com/juju/errors"
	"github.com/juju/loggo/v3"
	"github.com/juju/tc"
	"github.com/juju/worker/v5/workertest"

	"github.com/juju/juju/api"
	apilogsender "github.com/juju/juju/api/logsender"
	internaltesting "github.com/juju/juju/internal/testing"
	"github.com/juju/juju/internal/worker/logsender"
	"github.com/juju/juju/internal/worker/logsender/mocks"
)

type logSinkBackendSuite struct{}

func TestLogSinkBackendSuite(t *testing.T) {
	tc.Run(t, &logSinkBackendSuite{})
}

func (s *logSinkBackendSuite) TestServiceUnavailableRetainsPendingRecords(c *tc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	writer := mocks.NewMockLogWriter(ctrl)
	writer.EXPECT().WriteLog(gomock.Any()).Return(errors.WithType(
		stderrors.New("sending log message: server returned HTTP status 503"),
		api.HTTPStatusServiceUnavailable,
	))
	writer.EXPECT().Close()

	w, err := NewLogSink(stubLogSenderAPI{writer: writer}, 4)
	c.Assert(err, tc.ErrorIsNil)
	defer workertest.CleanKill(c, w)

	backend := w.(*logSinkBackend)
	sendBackendLog(c, backend.LogRecords(), "first")
	for a := internaltesting.LongAttempt.Start(); a.Next(); {
		if backend.Report(c.Context())["cutoverBlocked"] == true {
			break
		}
	}
	sendBackendLog(c, backend.LogRecords(), "second")

	var pending []*logsender.LogRecord
	for a := internaltesting.LongAttempt.Start(); a.Next(); {
		pending = backend.PendingRecords()
		if len(pending) == 2 {
			break
		}
	}

	c.Assert(pending, tc.HasLen, 2)
	c.Check(pending[0].Message, tc.Equals, "first")
	c.Check(pending[1].Message, tc.Equals, "second")
	c.Check(backend.Report(c.Context())["cutoverBlocked"], tc.Equals, true)
}

func (s *logSinkBackendSuite) TestRetryableWriteErrorDoesNotBlockCutover(c *tc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	writer := mocks.NewMockLogWriter(ctrl)
	writer.EXPECT().WriteLog(gomock.Any()).Return(stderrors.New("write failed")).AnyTimes()
	writer.EXPECT().Close().AnyTimes()

	w, err := NewLogSink(stubLogSenderAPI{writer: writer}, 4)
	c.Assert(err, tc.ErrorIsNil)
	defer workertest.CleanKill(c, w)

	backend := w.(*logSinkBackend)
	sendBackendLog(c, backend.LogRecords(), "first")

	workertest.CheckAlive(c, w)
	c.Check(backend.Report(c.Context())["cutoverBlocked"], tc.Equals, false)
}

type stubLogSenderAPI struct {
	writer *mocks.MockLogWriter
	err    error
}

func (s stubLogSenderAPI) LogWriter(context.Context) (apilogsender.LogWriter, error) {
	if s.err != nil {
		return nil, s.err
	}
	return s.writer, nil
}

func sendBackendLog(c *tc.C, logs logsender.LogRecordCh, message string) {
	select {
	case logs <- &logsender.LogRecord{
		Time:    time.Now(),
		Module:  "test",
		Level:   loggo.INFO,
		Message: message,
	}:
	case <-c.Context().Done():
		c.Fatal("timed out sending backend log")
	}
}
