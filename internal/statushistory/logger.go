// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package statushistory

import (
	"context"
	"encoding/json"

	"github.com/juju/juju/core/logger"
	"github.com/juju/juju/internal/errors"
)

const (
	// DataMarshalError is returned when marshaling data fails.
	DataMarshalError = errors.ConstError("data-marshal-error")
)

const (
	statusHistoryCategory = "status-history"
)

type logRecorder struct {
	logger logger.Logger
}

// NewLogRecorder returns a new logRecorder that logs to the given logger.
func NewLogRecorder(log logger.Logger) Recorder {
	return &logRecorder{logger: log.Child("status-history", logger.STATUS_HISTORY)}
}

// Record implements Recorder.Record.
func (r *logRecorder) Record(ctx context.Context, record Record) error {
	data, err := json.Marshal(record.Data)
	if err != nil {
		return errors.Errorf("failed to marshal data: %v", err).Add(DataMarshalError)
	}

	labels := logger.Labels{
		categoryKey:    statusHistoryCategory,
		kindKey:        record.Kind.String(),
		namespaceIDKey: record.ID,
		statusKey:      record.Status,
		messageKey:     record.Message,
		sinceKey:       record.Time,
		dataKey:        string(data),
	}
	r.logger.Logf(ctx, logger.INFO, labels, "status-history (status: %q, status-message: %q)", record.Status, record.Message)
	return nil
}

const (
	categoryKey    = "category"
	kindKey        = "kind"
	namespaceIDKey = "namespace_id"
	statusKey      = "status"
	messageKey     = "message"
	sinceKey       = "since"
	dataKey        = "data"
)
