// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package domain

import (
	"github.com/juju/clock"

	"github.com/juju/juju/core/logger"
	"github.com/juju/juju/core/model"
	"github.com/juju/juju/internal/statushistory"
)

// NewStatusHistory creates a new StatusHistory using the logger as the
// recorder.
func NewStatusHistory(logger logger.Logger, clock clock.Clock) *statushistory.StatusHistory {
	return statushistory.NewStatusHistory(statushistory.NewLogRecorder(logger), clock)
}

// NewStatusHistoryReader creates a new StatusHistoryReader using the given
// path and model UUID. The path should point to a file containing the status
// history records in JSON format. The model UUID is used to identify the
// model for which the status history is being read.
func NewStatusHistoryReader(path string, modelUUID model.UUID) (*statushistory.StatusHistoryReader, error) {
	return statushistory.ModelStatusHistoryReaderFromFile(modelUUID, path)
}
