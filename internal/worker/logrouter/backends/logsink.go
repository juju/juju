// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package backends

import (
	"context"
	"fmt"
	"io"
	"strings"
	"sync"
	"time"

	"github.com/juju/errors"
	"github.com/juju/worker/v5/catacomb"

	"github.com/juju/juju/api"
	apilogsender "github.com/juju/juju/api/logsender"
	corelogger "github.com/juju/juju/core/logger"
	internalerrors "github.com/juju/juju/internal/errors"
	"github.com/juju/juju/internal/worker/logsender"
	"github.com/juju/juju/rpc/params"
)

type logSinkBackend struct {
	catacomb catacomb.Catacomb

	records      logsender.LogRecordCh
	logSenderAPI logsender.LogSenderAPI

	mu             sync.Mutex
	pending        []*logsender.LogRecord
	cutoverBlocked bool
}

// NewLogSink returns a backend that sends log records to the controller log
// sink.
func NewLogSink(logSenderAPI logsender.LogSenderAPI, backendBufferSize int) (Backend, error) {
	w := &logSinkBackend{
		records:      make(logsender.LogRecordCh, backendBufferSize),
		logSenderAPI: logSenderAPI,
	}
	if err := catacomb.Invoke(catacomb.Plan{
		Name: "log-router-logsink",
		Site: &w.catacomb,
		Work: w.loop,
	}); err != nil {
		return nil, internalerrors.Capture(err)
	}
	return w, nil
}

// Kill stops the backend and closes the log record channel.
func (w *logSinkBackend) Kill() {
	w.catacomb.Kill(nil)
}

// Wait waits for the backend to stop.
func (w *logSinkBackend) Wait() error {
	return w.catacomb.Wait()
}

// LogRecords returns the channel on which log records are sent to the backend.
func (w *logSinkBackend) LogRecords() logsender.LogRecordCh {
	return w.records
}

// PendingRecords returns any records that were retained while the backend was
// blocked on logsink cutover, followed by records still buffered in the input
// channel.
func (w *logSinkBackend) PendingRecords() []*logsender.LogRecord {
	w.mu.Lock()
	defer w.mu.Unlock()

	pending := append([]*logsender.LogRecord(nil), w.pending...)
	for {
		select {
		case rec := <-w.records:
			if rec == nil {
				continue
			}
			pending = append(pending, rec)
		default:
			return pending
		}
	}
}

// Log implements corelogger.LogSink by converting records to the internal
// logsender format and submitting them to the backend's record channel.
func (w *logSinkBackend) Log(records []corelogger.LogRecord) error {
	return sendRecords(w.records, records)
}

// WatchRefresh implements corelogger.LogSink. Individual backends never
// change their underlying target; refresh signalling is handled by the log
// router when switching backends.
func (w *logSinkBackend) WatchRefresh() <-chan struct{} {
	return corelogger.NoRefresh()
}

// Report returns a report of the backend state.
func (w *logSinkBackend) Report(_ context.Context) map[string]any {
	w.mu.Lock()
	defer w.mu.Unlock()

	return map[string]any{
		"name":            "log-sink-backend",
		"bufferedRecords": len(w.pending) + len(w.records),
		"cutoverBlocked":  w.cutoverBlocked,
	}
}

func (w *logSinkBackend) loop() error {
	ctx := w.catacomb.Context(context.Background())
	var logWriter apilogsender.LogWriter
	closeLogWriter := func() {
		if logWriter != nil {
			_ = logWriter.Close()
			logWriter = nil
		}
	}
	defer closeLogWriter()

	for {
		if w.isCutoverBlocked() {
			ok := w.bufferWhileBlocked(ctx)
			if !ok {
				return nil
			}
			continue
		}

		rec, ok, err := w.nextRecord(ctx)
		if err != nil {
			return internalerrors.Capture(err)
		}
		if !ok {
			return nil
		}
		if logWriter == nil {
			var openErr error
			logWriter, openErr = openLogWriter(ctx, w.logSenderAPI)
			if openErr != nil {
				closeLogWriter()
				if ctx.Err() != nil {
					return nil
				}
				if isLogSinkUnavailableError(openErr) {
					w.setCutoverBlocked(true)
					continue
				}
				if isRetryableLogSenderError(openErr) {
					if err := waitRetry(ctx); err != nil {
						return nil
					}
					continue
				}
				return errors.Annotate(openErr, "logsender dial failed")
			}
		}

		err = writeLogRecord(logWriter, rec)
		if err == nil && rec.DroppedAfter > 0 {
			err = writeDroppedLogRecord(logWriter, rec)
		}
		if err != nil {
			closeLogWriter()
			if ctx.Err() != nil {
				return nil
			}
			if isLogSinkUnavailableError(err) {
				w.setCutoverBlocked(true)
				continue
			}
			if isRetryableLogSenderError(err) {
				if err := waitRetry(ctx); err != nil {
					return nil
				}
				continue
			}
			return internalerrors.Capture(err)
		}

		w.popFront()
	}
}

func (w *logSinkBackend) bufferWhileBlocked(ctx context.Context) bool {
	select {
	case <-ctx.Done():
		return false
	case rec, ok := <-w.records:
		if !ok {
			return false
		}
		if rec != nil {
			w.appendPending(rec)
		}
		return true
	}
}

func (w *logSinkBackend) nextRecord(ctx context.Context) (*logsender.LogRecord, bool, error) {
	for {
		if rec := w.front(); rec != nil {
			return rec, true, nil
		}

		select {
		case <-ctx.Done():
			return nil, false, nil
		case rec, ok := <-w.records:
			if !ok {
				return nil, false, nil
			}
			if rec == nil {
				continue
			}
			w.appendPending(rec)
		}
	}
}

func (w *logSinkBackend) front() *logsender.LogRecord {
	w.mu.Lock()
	defer w.mu.Unlock()

	if len(w.pending) == 0 {
		return nil
	}
	return w.pending[0]
}

func (w *logSinkBackend) popFront() {
	w.mu.Lock()
	defer w.mu.Unlock()

	if len(w.pending) == 0 {
		return
	}
	w.pending[0] = nil
	w.pending = w.pending[1:]
}

func (w *logSinkBackend) appendPending(rec *logsender.LogRecord) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.pending = append(w.pending, rec)
}

func (w *logSinkBackend) isCutoverBlocked() bool {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.cutoverBlocked
}

func (w *logSinkBackend) setCutoverBlocked(blocked bool) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.cutoverBlocked = blocked
}

func openLogWriter(ctx context.Context, logSenderAPI logsender.LogSenderAPI) (apilogsender.LogWriter, error) {
	sender := make(chan apilogsender.LogWriter)
	errChan := make(chan error)
	go func() {
		logWriter, err := logSenderAPI.LogWriter(ctx)
		if err != nil {
			select {
			case errChan <- err:
			case <-ctx.Done():
			}
			return
		}
		select {
		case sender <- logWriter:
		case <-ctx.Done():
			_ = logWriter.Close()
		}
	}()
	select {
	case logWriter := <-sender:
		return logWriter, nil
	case err := <-errChan:
		return nil, err
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

func writeLogRecord(logWriter apilogsender.LogWriter, rec *logsender.LogRecord) error {
	return logWriter.WriteLog(&params.LogRecord{
		Time:     rec.Time,
		Module:   rec.Module,
		Location: rec.Location,
		Level:    rec.Level.String(),
		Message:  rec.Message,
		Labels:   rec.Labels,
	})
}

func writeDroppedLogRecord(logWriter apilogsender.LogWriter, rec *logsender.LogRecord) error {
	return logWriter.WriteLog(&params.LogRecord{
		Time:    rec.Time,
		Module:  "juju.worker.logsender",
		Level:   corelogger.WARNING.String(),
		Message: fmt.Sprintf("%d log messages dropped due to lack of API connectivity", rec.DroppedAfter),
	})
}

func isRetryableLogSenderError(err error) bool {
	if err == nil {
		return false
	}
	return isLogSinkUnavailableError(err) ||
		errors.Is(err, io.EOF) ||
		strings.Contains(err.Error(), "api caller disconnected") ||
		strings.Contains(err.Error(), "use of closed network connection") ||
		strings.Contains(err.Error(), "write")
}

func isLogSinkUnavailableError(err error) bool {
	if err == nil {
		return false
	}
	return errors.Is(err, api.HTTPStatusServiceUnavailable)
}

func waitRetry(ctx context.Context) error {
	timer := time.NewTimer(time.Second)
	defer timer.Stop()
	select {
	case <-timer.C:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}
