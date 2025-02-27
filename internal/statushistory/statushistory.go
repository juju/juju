// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package statushistory

import (
	"context"
	"encoding/json"
	"time"

	"github.com/juju/juju/core/logger"
	"github.com/juju/juju/core/status"
)

// StatusHistory records status information into a generalized way.
type StatusHistory struct {
	logger logger.Logger
}

// NewStatusHistory creates a new StatusHistory.
func NewStatusHistory(logger logger.Logger) *StatusHistory {
	return &StatusHistory{
		logger: logger,
	}
}

// RecordStatus records the given status information.
// If the status data cannot be marshalled, it will not be recorded, instead
// the error will be logged under the data_error key.
func (s *StatusHistory) RecordStatus(ctx context.Context, status status.StatusInfo) {
	labels := logger.Labels{
		statusKey:  status.Status.String(),
		messageKey: status.Message,
		sinceKey:   status.Since.Format(time.RFC3339),
	}

	// For structured logging this is less than ideal, as we'll have JSON
	// encoded inside of JSON. However, it's the best we can do without
	// an alternative.
	data, err := json.Marshal(status.Data)
	if err != nil {
		labels[dataErrorKey] = err.Error()
	} else {
		labels[dataKey] = string(data)
	}

	s.logger.Logf(ctx, logger.INFO, labels, "status: %s", status.Message)
}

const (
	statusKey    = "status"
	messageKey   = "message"
	sinceKey     = "since"
	dataKey      = "data"
	dataErrorKey = "data_error"
)
