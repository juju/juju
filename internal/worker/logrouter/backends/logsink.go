// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package backends

import (
	"context"

	"github.com/juju/worker/v5"

	"github.com/juju/juju/internal/worker/logsender"
)

type logSinkBackend struct {
	worker.Worker
	records logsender.LogRecordCh
}

// NewLogSink returns a backend that sends log records to the controller log
// sink.
func NewLogSink(logSenderAPI logsender.LogSenderAPI, backendBufferSize int) (Backend, error) {
	records := make(logsender.LogRecordCh, backendBufferSize)
	return &logSinkBackend{
		Worker:  logsender.New(records, logSenderAPI),
		records: records,
	}, nil
}

// LogRecords returns the channel on which log records are sent to the backend.
func (w *logSinkBackend) LogRecords() logsender.LogRecordCh {
	return w.records
}

// Report returns a report of the underlying logsink worker when available.
func (w *logSinkBackend) Report(ctx context.Context) map[string]any {
	m := map[string]any{
		"name": "log-sink-backend",
	}

	reporter, ok := w.Worker.(worker.Reporter)
	if !ok {
		return m
	}
	m["client"] = reporter.Report(ctx)
	return m
}
