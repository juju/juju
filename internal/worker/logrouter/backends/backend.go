// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package backends

import (
	"github.com/juju/loggo/v3"
	"github.com/juju/worker/v5"

	corelogger "github.com/juju/juju/core/logger"
	"github.com/juju/juju/internal/worker/logsender"
)

// Backend is a worker that accepts log records.
type Backend interface {
	worker.Worker
	worker.Reporter
	LogRecords() logsender.LogRecordCh
}

// sendRecords converts corelogger.LogRecords to the logsender format and
// submits them to the supplied record channel. It is non-blocking: if the
// channel is full the record is dropped, mirroring the behaviour of the log
// router's own loop during transitions. A nil channel (used when the backend
// is not yet running) silently discards all records.
func sendRecords(ch logsender.LogRecordCh, records []corelogger.LogRecord) error {
	for _, r := range records {
		rec := &logsender.LogRecord{
			Time:      r.Time,
			Module:    r.Module,
			Location:  r.Location,
			Level:     loggo.Level(r.Level),
			Message:   r.Message,
			Labels:    r.Labels,
			ModelUUID: r.ModelUUID,
			Entity:    r.Entity,
		}
		select {
		case ch <- rec:
		default:
		}
	}
	return nil
}
